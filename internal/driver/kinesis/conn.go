package kinesis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("kinesis connection is not open")

// Conn is one live connection to one AWS account's Kinesis in one region.
//
// "One connection" is a signed client rather than a socket: every call is an
// HTTPS request that stands alone, so there is nothing held open between them
// and nothing to reconnect.
type Conn struct {
	client *awskinesis.Client
	config clientConfig

	// arns caches the stream ARN a name resolves to. Every call that names a
	// stream takes either the name or the ARN, but the consumer operations
	// take the ARN only - and it is derived from the account, the region and
	// the name, so it never changes for a name that keeps existing.
	arnsMu sync.RWMutex
	arns   map[string]string

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindKinesis }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping asks for one stream.
//
// There is no health endpoint to call and nothing that answers unsigned, so
// the cheapest question that proves the credential still reaches Kinesis is a
// listing capped at one row. An account with no streams answers it empty,
// which is a healthy connection and not an error.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	_, err := c.client.ListStreams(ctx, &awskinesis.ListStreamsInput{Limit: aws.Int32(1)})
	return err
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.arnsMu.Lock()
		c.arns = nil
		c.arnsMu.Unlock()
	})
	return nil
}

// live reports whether the connection is still usable.
func (c *Conn) live() error {
	if c.client == nil {
		return errConnectionDown
	}
	select {
	case <-c.closed:
		return errConnectionDown
	default:
		return nil
	}
}

// capabilities is the family's best case.
//
// It grows one port at a time: CheckConformance fails a capability with no
// interface behind it, so each one arrives in the commit that implements it
// rather than as a promise the connection cannot keep.
func capabilities() []model.Capability {
	return []model.Capability{
		model.CapDestinationList,
	}
}

// open builds the client and proves the credential reaches Kinesis.
//
// The probe is not optional. Nothing here dials, so an unreachable region, a
// mistyped one, an expired session token and a missing permission all look
// identical until a request is signed and sent - and a connection that opened
// without asking would report every one of them as an empty account.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	client, err := newClient(ctx, config)
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		client: client,
		config: config,
		arns:   make(map[string]string),
		closed: make(chan struct{}),
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("kinesis in %s did not answer: %w", config.region, err)
	}
	conn.capabilities = conn.declare()
	return conn, nil
}

// declare turns what answered into the capability set the pages gate on.
func (c *Conn) declare() model.Capabilities {
	return model.Capabilities{
		Supported: capabilities(),
		Degraded:  map[model.Capability]string{},
		Caveats:   map[model.Capability]string{},
	}
}

// Region is which region this connection signs for. The connection row shows
// it where every other family shows an address, because it is the only thing
// on this form that says where the streams are.
func (c *Conn) Region() string { return c.config.region }

// streamARN resolves a stream name to the ARN the consumer calls take.
//
// Every operation on a stream accepts its name, except the four that manage
// enhanced fan-out consumers: those take the stream's ARN and nothing else.
// DescribeStreamSummary is what turns one into the other, and the answer is
// cached because it is stable for as long as the name exists.
func (c *Conn) streamARN(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("no stream name given")
	}

	c.arnsMu.RLock()
	cached, ok := c.arns[name]
	c.arnsMu.RUnlock()
	if ok {
		return cached, nil
	}

	out, err := c.client.DescribeStreamSummary(ctx, &awskinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(name),
	})
	if err != nil {
		if notFound(err) {
			return "", fmt.Errorf("no stream named %q in %s", name, c.config.region)
		}
		return "", err
	}
	arn := aws.ToString(out.StreamDescriptionSummary.StreamARN)
	c.rememberARN(name, arn)
	return arn, nil
}

func (c *Conn) rememberARN(name, arn string) {
	c.arnsMu.Lock()
	defer c.arnsMu.Unlock()
	if c.arns != nil && arn != "" {
		c.arns[name] = arn
	}
}

// activePoll is how often awaitActive asks again. Every settling operation
// here takes seconds rather than milliseconds, so a tighter loop would only
// spend request quota to learn the same thing.
const activePoll = 500 * time.Millisecond

/*
 * awaitActive blocks until the stream is ACTIVE, or the context is done.
 *
 * Every operation that changes a stream is asynchronous: the call returns and
 * the stream goes to CREATING or UPDATING, and the next call that names it is
 * refused with ResourceInUseException while it is there. So an update that
 * changes two settings has to wait between them, and the wait is here rather
 * than in each caller.
 *
 * Not the SDK's own StreamExistsWaiter, which polls DescribeStream: that
 * returns every shard of the stream on every attempt, and the answer wanted
 * here is one field.
 */
func (c *Conn) awaitActive(ctx context.Context, name string) error {
	for {
		out, err := c.client.DescribeStreamSummary(ctx, &awskinesis.DescribeStreamSummaryInput{
			StreamName: aws.String(name),
		})
		if err != nil {
			return err
		}
		if out.StreamDescriptionSummary.StreamStatus == types.StreamStatusActive {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s was still %s when the request timed out: %w",
				name, out.StreamDescriptionSummary.StreamStatus, ctx.Err())
		case <-c.closed:
			return errConnectionDown
		case <-time.After(activePoll):
		}
	}
}

// forgetARN drops a cached ARN, which a delete has to do: the next call under
// this name must ask again rather than address a stream that is on its way out.
func (c *Conn) forgetARN(name string) {
	c.arnsMu.Lock()
	defer c.arnsMu.Unlock()
	delete(c.arns, name)
}

// streamNameOf reads the name back out of a stream ARN.
//
// An ARN spells it last, after "stream/", and a consumer's ARN appends its own
// name after that - so the stream part is taken rather than the last segment.
func streamNameOf(arn string) string {
	trimmed := strings.TrimSpace(arn)
	const marker = ":stream/"
	index := strings.Index(trimmed, marker)
	if index < 0 {
		return trimmed
	}
	name := trimmed[index+len(marker):]
	if cut := strings.Index(name, "/"); cut >= 0 {
		name = name[:cut]
	}
	return name
}
