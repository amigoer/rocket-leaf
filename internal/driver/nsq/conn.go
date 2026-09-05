package nsq

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("nsq connection is not open")

// node is one nsqd this connection was pointed at, and what it said about
// itself when the connection opened.
type node struct {
	// address is what the profile carried, scheme included. It is the address
	// every call goes to; the one nsqd broadcasts about itself may be a name
	// only the cluster can resolve.
	address string
	info    nsqdInfo
}

// clientConfig is the profile reduced to what this driver dials.
type clientConfig struct {
	nsqd       []string
	lookupd    []string
	timeout    time.Duration
	skipVerify bool
}

// Conn is one live connection to one NSQ cluster.
//
// "One connection" is a set of addresses rather than a socket: NSQ's
// management plane is stateless HTTP, so there is nothing held open between
// calls and nothing to reconnect.
type Conn struct {
	client *httpClient
	config clientConfig

	// nodes is fixed for the life of the connection. An nsqd that joins the
	// cluster later is not picked up, deliberately: the profile names the
	// daemons this connection speaks for, and silently growing that set would
	// change what a depth on the topics board is the sum of.
	nodes []node

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindNSQ }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping asks every nsqd, and fails if any of them does not answer.
//
// Not "any one of them": every read this driver does is a sum across the
// whole set, so a cluster with one daemon missing reports depths that are
// wrong rather than depths that are late. A connection that called that
// healthy would make the topics board quietly under-report.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	_, err := eachNode(ctx, c.nodes, func(ctx context.Context, n node) (struct{}, error) {
		return struct{}{}, c.client.get(ctx, n.address, "/ping", nil, nil)
	})
	return err
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.client != nil {
			c.client.http.CloseIdleConnections()
		}
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
		model.CapDestinationDelete,
		model.CapDestinationPurge,

		model.CapSubscriptionList,
		model.CapSubscriptionCreate,
		model.CapSubscriptionDelete,
		model.CapSubscriptionLag,

		model.CapPublish,
		model.CapDelayedDelivery,
	}
}

// open dials every address and checks each is the daemon its field names.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	client := newHTTPClient(config.timeout, config.skipVerify)
	nodes, err := probeNSQD(ctx, client, config.nsqd)
	if err != nil {
		return nil, err
	}
	if err := probeLookupd(ctx, client, config.lookupd); err != nil {
		return nil, err
	}

	conn := &Conn{client: client, config: config, nodes: nodes, closed: make(chan struct{})}
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

// probeNSQD reads /info off every configured address.
//
// The check is not "did something answer HTTP". nsqlookupd answers /info too,
// with a body carrying nothing but a version, and pointing the nsqd field at
// one is the single likeliest way to fill this form in wrong - the ports are
// adjacent and both daemons are called nsq-something. Caught here it is one
// sentence; caught later it is a connection that opens and then reports a
// cluster with no topics in it.
func probeNSQD(ctx context.Context, client *httpClient, addresses []string) ([]node, error) {
	nodes := make([]node, len(addresses))
	group, groupCtx := errgroup.WithContext(ctx)
	for index, address := range addresses {
		group.Go(func() error {
			var info nsqdInfo
			if err := client.get(groupCtx, address, "/info", nil, &info); err != nil {
				return fmt.Errorf("no nsqd answered at %s: %w", address, err)
			}
			if info.TCPPort == 0 {
				return fmt.Errorf(
					"%s is an nsqlookupd rather than an nsqd; its address belongs in the lookupd field",
					address)
			}
			nodes[index] = node{address: address, info: info}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// probeLookupd checks the optional tier the same way round: an nsqd answers
// /info with a tcp_port, and one typed in here would be listed on the
// directory board as a discovery node it is not.
func probeLookupd(ctx context.Context, client *httpClient, addresses []string) error {
	group, groupCtx := errgroup.WithContext(ctx)
	for _, address := range addresses {
		group.Go(func() error {
			var info lookupInfo
			if err := client.get(groupCtx, address, "/info", nil, &info); err != nil {
				return fmt.Errorf("no nsqlookupd answered at %s: %w", address, err)
			}
			var full nsqdInfo
			if err := client.get(groupCtx, address, "/info", nil, &full); err == nil && full.TCPPort != 0 {
				return fmt.Errorf(
					"%s is an nsqd rather than an nsqlookupd; its address belongs in the nsqd field",
					address)
			}
			return nil
		})
	}
	return group.Wait()
}

// eachNode runs fn against every nsqd at once and returns the results in the
// order the profile lists them, so a board's rows do not reshuffle between
// refreshes.
func eachNode[T any](ctx context.Context, nodes []node, fn func(context.Context, node) (T, error)) ([]T, error) {
	results := make([]T, len(nodes))
	group, groupCtx := errgroup.WithContext(ctx)
	for index, n := range nodes {
		group.Go(func() error {
			result, err := fn(groupCtx, n)
			if err != nil {
				return fmt.Errorf("%s: %w", hostPort(n.address), err)
			}
			results[index] = result
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// configOf reduces a profile to what this driver dials.
func configOf(profile model.ConnectionProfile) (clientConfig, error) {
	nsqd := splitAddresses(profile.Endpoints)
	if len(nsqd) == 0 {
		return clientConfig{}, errors.New("no nsqd address configured")
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if profile.TimeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	return clientConfig{
		nsqd:       nsqd,
		lookupd:    splitAddresses(profile.Option(OptionLookupd)),
		timeout:    timeout,
		skipVerify: isTrue(profile.Option(OptionTLSSkipVerify)),
	}, nil
}

func isTrue(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
