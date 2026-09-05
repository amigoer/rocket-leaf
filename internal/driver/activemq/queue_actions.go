package activemq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/amigoer/mq-studio/internal/model"
)

// Emptying a destination and moving what is in it.
//
// Both products can do both, and both do them with a selector rather than with
// a count: JMS selects, it does not page. That shapes what this can honestly
// offer - see DropMessages, which neither broker can do.

// errNoDropBatch is why DropMessages is not offered.
var errNoDropBatch = errors.New(
	"activemq removes messages by selector, not by count from the head")

// errNoRebalance is why RebalanceQueues is not offered.
var errNoRebalance = errors.New(
	"an activemq destination lives on the broker that owns it and is not redistributed")

// PurgeQueue empties a destination without deleting it.
func (c *Conn) PurgeQueue(ctx context.Context, ref model.DestinationRef) error {
	mbean, kind, err := c.destinationMBean(ctx, ref)
	if err != nil {
		return err
	}
	if c.tiers.product == artemis {
		if kind == topicKind {
			// A multicast address holds nothing itself: the messages are in
			// its subscription queues, so emptying the topic means emptying
			// each of them.
			return c.purgeArtemisTopic(ctx, ref.Name)
		}
		_, err := c.jolokia.call(ctx, execOperation(mbean, "removeAllMessages()"))
		return err
	}
	_, err = c.jolokia.call(ctx, execOperation(mbean, "purge()"))
	return err
}

func (c *Conn) purgeArtemisTopic(ctx context.Context, address string) error {
	queues, err := c.artemisQueuesUnder(ctx, address)
	if err != nil {
		return err
	}
	requests := make([]request, 0, len(queues))
	for _, queue := range queues {
		requests = append(requests, execOperation(
			c.names.artemisQueue(address, queue, multicast), "removeAllMessages()"))
	}
	// Tolerant: a subscription unsubscribed between the read and the purge
	// must not leave the other subscriptions full.
	_, _, err = c.jolokia.batchTolerant(ctx, requests)
	return err
}

// MoveMessages drains one destination into another.
//
// The canonical request names an exchange and a routing key, which is
// RabbitMQ's vocabulary; ActiveMQ has neither. What it has is a target
// destination by name, so the routing key is read as that name and the
// exchange is ignored - a JMS move puts the message in the named queue and
// there is no topology in between for it to take.
//
// Limit is ignored for the same kind of reason: both products move by
// selector, and neither takes a count. The count that comes back is the
// broker's own, which is what makes the return value worth having.
func (c *Conn) MoveMessages(ctx context.Context, request model.MoveRequest) (int, error) {
	target := request.ToRoutingKey
	if target == "" {
		target = request.ToExchange
	}
	if target == "" {
		return 0, errors.New("a move needs a destination to move into")
	}

	mbean, _, err := c.destinationMBean(ctx, model.DestinationRef{
		Namespace: request.Namespace,
		Name:      request.From,
	})
	if err != nil {
		return 0, err
	}

	if c.tiers.product == artemis {
		// An empty filter matches everything, which is what an unfiltered
		// move means. rejectDuplicates is off: the target may legitimately
		// already hold a copy, and refusing the whole batch for it would make
		// a retry from a dead-letter queue fail on its second attempt.
		value, err := c.jolokia.call(ctx, execOperation(mbean,
			"moveMessages(java.lang.String,java.lang.String)", "", target))
		if err != nil {
			return 0, err
		}
		return intOr(value, 0), nil
	}

	// Classic's selector is a JMS selector, and an empty one matches
	// everything the same way.
	value, err := c.jolokia.call(ctx, execOperation(mbean,
		"moveMatchingMessagesTo(java.lang.String,java.lang.String)", "", target))
	if err != nil {
		return 0, err
	}
	return intOr(value, 0), nil
}

// DropMessages is not offered. See errNoDropBatch.
func (c *Conn) DropMessages(_ context.Context, _ model.DestinationRef, _ int) (int, error) {
	return 0, errNoDropBatch
}

// RebalanceQueues is not offered. See errNoRebalance.
func (c *Conn) RebalanceQueues(_ context.Context) error { return errNoRebalance }

// destinationMBean resolves a ref to the MBean that owns its messages.
//
// It needs the kind, and the kind is not in the ref: Classic keeps queues and
// topics in different trees, and Artemis puts the routing type in the name.
// Reading the listing is what settles it, which costs a round trip and is the
// price of one ref addressing two products.
func (c *Conn) destinationMBean(ctx context.Context, ref model.DestinationRef) (string, destinationKind, error) {
	detail, err := c.DestinationDetail(ctx, ref)
	if err != nil {
		return "", "", err
	}
	kind := destinationKind(detail.Attributes[AttrKind])
	if kind != topicKind {
		kind = queueKind
	}
	return c.names.destination(ref.Name, kind), kind, nil
}

// artemisQueuesUnder lists the queues bound to an address, which for a
// multicast address is its durable subscriptions.
func (c *Conn) artemisQueuesUnder(ctx context.Context, address string) ([]string, error) {
	value, err := c.jolokia.call(ctx, readAttribute(c.names.artemisAddress(address), "QueueNames"))
	if err != nil {
		if notRegistered(err) {
			return nil, nil
		}
		return nil, err
	}
	var queues []string
	if err := json.Unmarshal(value, &queues); err != nil {
		return nil, fmt.Errorf("the queue list under %q is not a set of names: %w", address, err)
	}
	return queues, nil
}
