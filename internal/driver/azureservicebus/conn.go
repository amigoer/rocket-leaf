package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"

	"github.com/amigoer/mq-studio/internal/model"
)

var errConnectionDown = errors.New("azure service bus connection is not open")

// Degraded reasons, as i18n keys rather than sentences. The renderer turns
// them into the user's language; an English frame around one would put the key
// itself on screen.
const (
	// countsNotInEmulator is why a depth and a backlog are missing against the
	// emulator and only against it.
	//
	// Service Bus reports both, in the CountDetails element of an entity's
	// Atom description. The emulator serves that element for a queue and a
	// topic not at all, and for a subscription with its children renamed to
	// five obfuscated tokens the SDK cannot read - so there is no number to
	// report and nothing a user could configure to produce one.
	countsNotInEmulator = "mq.azure-servicebus.degraded.countsNotInEmulator"
)

// Conn is one live connection to one Service Bus namespace.
//
// Two clients rather than one, because Service Bus is two protocols. data
// speaks AMQP and holds a real connection open; management speaks Atom over
// HTTPS and holds nothing between calls. Both are built from one credential,
// which is why they belong to one Conn rather than to two.
type Conn struct {
	data       *azservicebus.Client
	management *admin.Client
	config     clientConfig

	capabilities model.Capabilities
	closeOnce    sync.Once
	closed       chan struct{}
}

// Kind identifies the family.
func (c *Conn) Kind() model.MQKind { return model.KindAzureServiceBus }

// Capabilities is what this endpoint can do.
func (c *Conn) Capabilities() model.Capabilities { return c.capabilities }

// Ping asks the namespace to describe itself.
//
// GetNamespaceProperties is the cheapest question that proves the credential
// still signs: it takes no entity, so a namespace with nothing in it answers
// it, and a key that has been rotated fails it. A listing would have done too
// and would have been a lie about what was checked - an empty namespace lists
// empty whether or not the credential was any good.
func (c *Conn) Ping(ctx context.Context) error {
	if err := c.live(); err != nil {
		return err
	}
	_, err := c.management.GetNamespaceProperties(ctx, nil)
	return err
}

// Close releases what the connection holds. The registry closes on disconnect
// and on shutdown, so the second call has to be the one that does nothing.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.data != nil {
			// The AMQP connection is the only thing actually held open; the
			// management client is stateless HTTPS.
			err = c.data.Close(context.WithoutCancel(context.Background()))
		}
	})
	return err
}

// live reports whether the connection is still usable.
func (c *Conn) live() error {
	if c.data == nil || c.management == nil {
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

		model.CapSubscriptionList,
		model.CapSubscriptionCreate,
		model.CapSubscriptionDelete,
		model.CapSubscriptionLag,

		model.CapMessageQuery,
	}
}

// open builds both clients and proves the credential reaches the namespace.
//
// The probe is not optional. Neither client dials on construction, so a
// namespace that does not exist, a key for the wrong one, a rotated key and a
// rule with no Manage claim all look identical until a request is made - and a
// connection that opened without asking would report every one of them as an
// empty namespace.
func open(ctx context.Context, profile model.ConnectionProfile) (*Conn, error) {
	config, err := configOf(profile)
	if err != nil {
		return nil, err
	}

	data, management, err := newClients(config)
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		data:       data,
		management: management,
		config:     config,
		closed:     make(chan struct{}),
	}
	// The probe takes the profile's own timeout rather than the caller's. The
	// SDK retries a refused connection with backoff until its deadline, so a
	// host with nothing behind it would otherwise hold the connection dialog
	// shut for however long the caller was willing to wait.
	probeCtx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	if err := conn.Ping(probeCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%s did not answer: %w", config.namespace, err)
	}
	conn.capabilities = conn.declare()
	return conn, nil
}

/*
 * declare turns what answered into the capability set the pages gate on.
 *
 * One entry varies by endpoint, and it is the reason declare() exists rather
 * than capabilities() being handed straight through: a subscription's backlog
 * is a real figure on a real namespace and unavailable against the emulator.
 * Service Bus reports it in the CountDetails element of the subscription's
 * description; the emulator sends that element with its five children renamed
 * to tokens the SDK cannot read, so there is no number to report and nothing a
 * user could change to produce one.
 *
 * Degraded rather than absent, because the family does have the concept - and
 * degraded only there, because a real namespace answers it.
 */
func (c *Conn) declare() model.Capabilities {
	degraded := map[model.Capability]string{}
	if c.config.emulator() {
		degraded[model.CapSubscriptionLag] = countsNotInEmulator
	}
	supported := make([]model.Capability, 0, len(capabilities()))
	for _, capability := range capabilities() {
		if _, hidden := degraded[capability]; !hidden {
			supported = append(supported, capability)
		}
	}
	return model.Capabilities{
		Supported: supported,
		Degraded:  degraded,
		Caveats:   map[model.Capability]string{},
	}
}

// Namespace is which namespace this connection reads.
func (c *Conn) Namespace() string { return c.config.namespace }

// Emulator is the management host this connection was pointed at, empty for
// the real service. It is what declare() narrows on, so the live tests can
// assert the narrowing rather than infer it.
func (c *Conn) Emulator() string { return c.config.emulatorManagement }

// requiredName trims an entity name and refuses one the API could not take.
//
// A Service Bus entity name may contain a slash - a subscription's path is
// <topic>/Subscriptions/<name> - so the check is not the one Pub/Sub needs.
// What it refuses is a name that would address a sub-entity by accident: the
// dead-letter queue and the transfer dead-letter queue are reached by suffix,
// and a browse of "orders/$DeadLetterQueue" typed into a name field would
// silently read a different entity from the one the page said.
func requiredName(kind, name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("no %s name given", kind)
	}
	if strings.HasPrefix(trimmed, "$") || strings.Contains(trimmed, "/$") {
		return "", fmt.Errorf(
			"%q names a sub-entity rather than a %s; the dead letters have a page of their own",
			trimmed, kind)
	}
	return trimmed, nil
}

// matchesPrefix reports whether a name passes the connection's filter.
// An empty prefix keeps everything, which is the ordinary case.
func (c *Conn) matchesPrefix(name string) bool {
	return c.config.prefix == "" || strings.HasPrefix(name, c.config.prefix)
}

// endpointName is what an error calls whatever answered, for the messages that
// have to distinguish an emulator from a real namespace.
func (c *Conn) endpointName() string {
	if c.config.emulator() {
		return "the service bus emulator at " + c.config.namespace
	}
	return c.config.namespace
}
