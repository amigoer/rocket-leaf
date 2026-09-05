package activemq

import (
	"context"
	"errors"

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating and removing destinations.
//
// Both products can do both, through the broker MBean rather than through the
// destination's own - which does not exist yet when it is being created. What
// they disagree about is what a topic is: Classic has an addTopic beside its
// addQueue, and Artemis has one operation that takes a routing type.

// errNoUpdate is what UpdateDestination reports.
//
// The method exists because DestinationAdmin is one interface; the capability
// is not declared, so nothing in the UI reaches this. See the reason in
// conformance_test.go.
var errNoUpdate = errors.New("activemq destinations are not reconfigured after they are created")

// CreateDestination declares a queue or a topic.
//
// The kind comes off the spec's attributes rather than being inferred from the
// name. A JMS queue and a JMS topic can share a name - they are different
// objects in different trees on Classic, and one address with both routing
// types on Artemis - so guessing would sometimes make the wrong one.
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	kind := kindOf(spec.Attributes)
	if spec.Ref.Name == "" {
		return errors.New("a destination needs a name")
	}

	if c.tiers.product == artemis {
		routing := "ANYCAST"
		if kind == topicKind {
			routing = "MULTICAST"
		}
		// createQueue rather than createAddress, because an address with no
		// queue under it holds nothing: an anycast address without its queue
		// accepts a send and drops it. The three-argument form makes both.
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"createQueue(java.lang.String,java.lang.String,java.lang.String)",
			spec.Ref.Name, spec.Ref.Name, routing))
		return err
	}

	operation := "addQueue(java.lang.String)"
	if kind == topicKind {
		operation = "addTopic(java.lang.String)"
	}
	_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(), operation, spec.Ref.Name))
	return err
}

// UpdateDestination is not offered. See errNoUpdate.
func (c *Conn) UpdateDestination(_ context.Context, _ model.DestinationSpec) error {
	return errNoUpdate
}

// RemoveDestination deletes a queue or a topic and everything it holds.
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if c.tiers.product == artemis {
		// force, because deleteAddress refuses an address that still has
		// queues under it - which is every address this driver created, since
		// createQueue made one. Refusing here would mean a delete that works
		// on an empty topic and fails on a real one.
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"deleteAddress(java.lang.String,boolean)", ref.Name, true))
		return err
	}

	// Classic keeps queues and topics in separate trees with an operation
	// each, and the kind has to be looked up rather than guessed at by trying
	// one and falling back.
	//
	// The obvious shape - call removeQueue, and let a name that is not a queue
	// fail into removeTopic - does not work: removeQueue answers 200 for a
	// topic's name, and for a name that exists nowhere at all. A delete built
	// on it removes nothing and reports success, which is what shipped until
	// somebody deleted a topic in the app and watched the row stay.
	detail, err := c.DestinationDetail(ctx, ref)
	if err != nil {
		return err
	}
	operation := "removeQueue(java.lang.String)"
	if detail.Attributes[AttrKind] == string(topicKind) {
		operation = "removeTopic(java.lang.String)"
	}
	_, err = c.jolokia.call(ctx, execOperation(c.names.brokerMBean(), operation, ref.Name))
	return err
}

// kindOf reads the destination kind a caller asked for, defaulting to a queue.
//
// A queue is the default because it is the one a user who did not think about
// it meant: a topic with no subscriber discards what is sent to it, and a
// queue keeps it.
func kindOf(attributes map[string]string) destinationKind {
	if attributes[AttrKind] == string(topicKind) {
		return topicKind
	}
	return queueKind
}
