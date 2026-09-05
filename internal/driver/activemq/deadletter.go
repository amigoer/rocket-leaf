package activemq

import (
	"context"
	"errors"
	"sort"

	"github.com/amigoer/mq-studio/internal/model"
)

// Dead letters, which this family has properly and most of the others do not.
//
// Kafka has no broker-side dead-letter queue at all; NATS moves nothing and
// publishes an advisory instead; Redis keeps a pending list because it gives
// up on nothing. ActiveMQ moves the message, to a destination the operator
// named, and can put it back where it came from. That makes it the first
// family since RabbitMQ to exercise this page fully, and the first ever to
// exercise the retry.
//
// Finding the queues is a topology walk rather than a lookup, which is why
// this implements DeadLetterTopology: neither product keeps a list of its
// dead-letter destinations. Artemis records a DeadLetterAddress on each queue,
// so the set is what those point at. Classic decides by broker policy and
// marks the result with a DLQ flag on the destination itself.

// DeadLetterQueues walks the topology backwards to find what dead-letters where.
func (c *Conn) DeadLetterQueues(ctx context.Context, _ string) ([]*model.DeadLetterQueue, error) {
	destinations, err := c.ListDestinations(ctx, model.DestinationFilter{IncludeInternal: true})
	if err != nil {
		return nil, err
	}
	if c.tiers.product == artemis {
		return c.artemisDeadLetterQueues(ctx, destinations)
	}
	return c.classicDeadLetterQueues(destinations), nil
}

// classicDeadLetterQueues reads the broker's own flag.
//
// Classic decides where a message goes by a dead-letter strategy configured on
// the destination policy, and the strategy is not readable per destination -
// but the destination it lands in reports DLQ=true, which is the same fact
// from the other end. What cannot be recovered that way is which destination
// fed it: a shared dead-letter queue holds messages from everywhere, and the
// broker keeps no record of where each came from.
func (c *Conn) classicDeadLetterQueues(destinations []*model.Destination) []*model.DeadLetterQueue {
	queues := make([]*model.DeadLetterQueue, 0, 2)
	for _, destination := range destinations {
		if destination.Attributes[AttrIsDeadLetter] != "true" {
			continue
		}
		queues = append(queues, &model.DeadLetterQueue{
			Name:      destination.Ref.Name,
			Depth:     destination.Depth,
			Consumers: intOr(nil, destination.Subscribers),
			// Left empty deliberately. Classic's dead-letter strategy is
			// broker configuration rather than a per-destination declaration,
			// so there is nothing to walk backwards from - and inventing the
			// list from destination names would be a guess presented as
			// topology.
			Sources: nil,
		})
	}
	sortDeadLetters(queues)
	return queues
}

// artemisDeadLetterQueues walks the declarations.
//
// Every queue names the address its undeliverable messages go to, so the set
// of dead-letter destinations is the set of things pointed at - and the
// sources come out of the same read, which is what lets the page say a
// dead-letter queue has no sources left and will never receive anything again.
func (c *Conn) artemisDeadLetterQueues(ctx context.Context, destinations []*model.Destination) ([]*model.DeadLetterQueue, error) {
	byName := make(map[string]*model.Destination, len(destinations))
	for _, destination := range destinations {
		byName[destination.Ref.Name] = destination
	}

	sources := make(map[string][]*model.DeadLetterSource)
	for _, destination := range destinations {
		target := destination.Attributes[AttrDeadLetter]
		if target == "" || target == destination.Ref.Name {
			continue
		}
		sources[target] = append(sources[target], &model.DeadLetterSource{
			Queue: destination.Ref.Name,
		})
	}

	// A dead-letter address that nothing points at any more still matters: it
	// is where a backlog is sitting, and its emptiness of sources is the fact
	// worth showing. So the default address is included whether or not a queue
	// currently names it.
	for _, destination := range destinations {
		if destination.Attributes[AttrIsDeadLetter] == "true" {
			if _, known := sources[destination.Ref.Name]; !known {
				sources[destination.Ref.Name] = nil
			}
		}
	}

	queues := make([]*model.DeadLetterQueue, 0, len(sources))
	for name, from := range sources {
		destination := byName[name]
		if destination == nil {
			// Declared as a target and not created yet, which Artemis allows:
			// the address appears the first time something is dead-lettered.
			queues = append(queues, &model.DeadLetterQueue{Name: name, Sources: from})
			continue
		}
		queues = append(queues, &model.DeadLetterQueue{
			Name:      name,
			Depth:     destination.Depth,
			Consumers: destination.Subscribers,
			Sources:   from,
		})
	}
	sortDeadLetters(queues)
	_ = ctx
	return queues, nil
}

// RetryDeadLetters sends a dead-lettered destination's contents back to where
// each message came from.
//
// Not one of the canonical ports, and it should not be. DeadLetterReader's
// ResendMessage is RocketMQ's shape - a consumer group, a client id and one
// message id - and this is none of those: both products retry the whole
// destination, and each message goes back to the one it originally failed on
// rather than onto a retry path. There is no consumer group in it at all.
//
// One operation on both, which is unusual for this family: retryMessages()
// takes no arguments on Classic and on Artemis alike. The selector-taking form
// the documentation describes exists on neither - confirmed against 6.2.0 and
// 2.44.0 by listing the MBeans' operations.
//
// The count is the broker's own, which is what separates a retry that matched
// nothing from one that emptied the queue.
func (c *Conn) RetryDeadLetters(ctx context.Context, ref model.DestinationRef) (int, error) {
	mbean, kind, err := c.destinationMBean(ctx, ref)
	if err != nil {
		return 0, err
	}
	if kind == topicKind {
		return 0, errors.New("a topic holds no dead letters to retry")
	}

	value, err := c.jolokia.call(ctx, execOperation(mbean, "retryMessages()"))
	if err != nil {
		return 0, err
	}
	return intOr(value, 0), nil
}

func sortDeadLetters(queues []*model.DeadLetterQueue) {
	sort.SliceStable(queues, func(i, j int) bool { return queues[i].Name < queues[j].Name })
}
