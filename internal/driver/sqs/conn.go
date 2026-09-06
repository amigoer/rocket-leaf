package sqs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("sqs connection is not open")

// Caveats, as i18n keys rather than sentences. The renderer turns them into
// the user's language; an English frame around one would put the key itself on
// screen.
//
// A caveat is not a degraded reason: the capability works, and doing it has a
// consequence worth saying out loud.
const (
	receiveHides = "mq.sqs.caveat.receiveHides"
)

// Conn is one live connection to one AWS account's SQS in one region.
//
// "One connection" is a signed client rather than a socket: every call is an
// HTTPS request that stands alone, so there is nothing held open between them
// and nothing to reconnect.
type Conn struct {
	client *awssqs.Client
	config clientConfig

	// urls caches the queue URL a name resolves to. The URL is derived from
	// the account, the region and the name, so it never changes for a name
	// that keeps existing - and a queue deleted and recreated gets the same
	// one back. Caching it saves a GetQueueUrl round trip, which AWS bills
	// for, on every operation a page addresses by name.
	urlsMu sync.RWMutex
	urls   map[string]string

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindSQS }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping asks for one queue.
//
// There is no health endpoint to call and nothing that answers unsigned, so
// the cheapest question that proves the credential still reaches SQS is a
// listing capped at one row. An account with no queues answers it empty, which
// is a healthy connection and not an error.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	_, err := c.client.ListQueues(ctx, &awssqs.ListQueuesInput{MaxResults: aws.Int32(1)})
	return err
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.urlsMu.Lock()
		c.urls = nil
		c.urlsMu.Unlock()
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
		model.CapDestinationCreate,
		model.CapDestinationUpdate,
		model.CapDestinationDelete,
		model.CapDestinationPurge,

		model.CapMessageQuery,
	}
}

// open builds the client and proves the credential reaches SQS.
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
		urls:   make(map[string]string),
		closed: make(chan struct{}),
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("sqs in %s did not answer: %w", config.region, err)
	}
	conn.capabilities = conn.declare()
	return conn, nil
}

/*
 * declare turns what answered into the capability set the pages gate on.
 *
 * Nothing here varies by endpoint: SQS is one service with one feature set,
 * and a credential that cannot do something fails the call rather than
 * narrowing what the connection reports. What there is instead is a caveat,
 * and it is unconditional - browsing goes through ReceiveMessage, which is the
 * same call a consumer makes.
 */
func (c *Conn) declare() model.Capabilities {
	return model.Capabilities{
		Supported: capabilities(),
		Degraded:  map[model.Capability]string{},
		Caveats: map[model.Capability]string{
			model.CapMessageQuery: receiveHides,
		},
	}
}

// Region is which region this connection signs for. The connection row shows
// it where every other family shows an address, because it is the only thing
// on this form that says where the queues are.
func (c *Conn) Region() string { return c.config.region }

// queueURL resolves a queue name to the URL every other call takes.
//
// SQS addresses a queue by URL and nothing else: the name is what a user
// types, and GetQueueUrl is the only thing that turns one into the other.
func (c *Conn) queueURL(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("no queue name given")
	}

	c.urlsMu.RLock()
	cached, ok := c.urls[name]
	c.urlsMu.RUnlock()
	if ok {
		return cached, nil
	}

	out, err := c.client.GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{QueueName: aws.String(name)})
	if err != nil {
		if notFound(err) {
			return "", fmt.Errorf("no queue named %q in %s", name, c.config.region)
		}
		return "", err
	}
	url := aws.ToString(out.QueueUrl)
	c.rememberURL(name, url)
	return url, nil
}

func (c *Conn) rememberURL(name, url string) {
	c.urlsMu.Lock()
	defer c.urlsMu.Unlock()
	if c.urls != nil {
		c.urls[name] = url
	}
}

func (c *Conn) forgetURL(name string) {
	c.urlsMu.Lock()
	defer c.urlsMu.Unlock()
	delete(c.urls, name)
}

// queueNameOf reads the name back out of a queue URL or ARN.
//
// Both spell it last and neither offers a call that reports it, so this is how
// a listing - which returns URLs - and a redrive policy - which carries an ARN
// - are turned into the names the pages show.
func queueNameOf(identifier string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(identifier), "/")
	if trimmed == "" {
		return ""
	}
	if index := strings.LastIndexAny(trimmed, "/:"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

// isFIFO reports whether a queue is a FIFO one, which is spelled in its name.
//
// The suffix is not a convention: SQS requires it on a FIFO queue and refuses
// it on a standard one, so the name is a reliable answer and one that needs no
// request to get.
func isFIFO(name string) bool { return strings.HasSuffix(name, ".fifo") }
