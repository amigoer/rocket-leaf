package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	pubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/iterator"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("google pub/sub connection is not open")

// Conn is one live connection to one Google Cloud project's Pub/Sub.
//
// "One connection" is an authenticated API client rather than a socket:
// the admin calls are unary RPCs that stand alone, so nothing is held open
// between them beyond the gRPC channel the client manages itself.
type Conn struct {
	client *pubsub.Client
	config clientConfig

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindGooglePubSub }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping asks for one topic.
//
// There is no health endpoint to call and nothing that answers unauthenticated,
// so the cheapest question that proves the credential still reaches the project
// is a listing stopped at the first row. A project with no topics answers it
// empty, which is a healthy connection and not an error.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	listing := c.client.TopicAdminClient.ListTopics(ctx, &pubsubpb.ListTopicsRequest{
		Project: c.projectPath(),
	})
	listing.PageInfo().MaxSize = 1
	if _, err := listing.Next(); err != nil && !errors.Is(err, iterator.Done) {
		return err
	}
	return nil
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.client != nil {
			err = c.client.Close()
		}
	})
	return err
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
	return nil
}

// open builds the client and proves the credential reaches the project.
//
// The probe is not optional. Nothing here dials, so a project that does not
// exist, a key for the wrong one, an expired credential and a missing IAM
// role all look identical until a request is made - and a connection that
// opened without asking would report every one of them as an empty project.
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
		closed: make(chan struct{}),
	}
	// The probe takes the profile's own timeout rather than the caller's. The
	// generated client retries an unavailable endpoint with backoff until its
	// deadline, so a host with nothing behind it would otherwise hold the
	// connection dialog shut for however long the caller was willing to wait.
	probeCtx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	if err := conn.Ping(probeCtx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("google pub/sub in %s did not answer: %w", config.project, err)
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

// Project is which project this connection reads. The connection row shows it
// where every other family shows an address, because it is the only thing on
// this form that says where the topics are.
func (c *Conn) Project() string { return c.config.project }

// Emulator is the host this connection was pointed at, empty for the real
// service. It is what declare() narrows on, so the live tests can assert the
// narrowing rather than infer it.
func (c *Conn) Emulator() string { return c.config.emulator }

// Resource names.
//
// Every call in this API takes a full path - projects/p/topics/t - and every
// page shows the last segment. Nothing in the library converts between them,
// so these do, and they are the only place the shape is written down.

func (c *Conn) projectPath() string { return "projects/" + c.config.project }

// shortName is the last segment of a resource path.
//
// A Pub/Sub name may not contain a slash, so the last segment is unambiguous;
// a name that is already short is returned unchanged, which is what lets a
// caller pass either.
func shortName(path string) string {
	trimmed := strings.TrimSpace(path)
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}
