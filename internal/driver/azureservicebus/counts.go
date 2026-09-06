package azureservicebus

import (
	"context"
	"errors"
)

// errGone is what a nil response with a nil error means: the entity is not
// there any more, so there is no count rather than a count of zero.
var errGone = errors.New("the entity is gone")

/*
 * How many messages an entity is holding, and why that is a question with a
 * "no answer" case.
 *
 * Service Bus reports the figures in the CountDetails element of an entity's
 * Atom description, and the SDK reads them through its *RuntimeProperties
 * calls. Against a real namespace they always arrive. Against the emulator
 * they never do, in two different ways:
 *
 *   - a queue's and a topic's description carry no CountDetails element at
 *     all, and the SDK returns "invalid queue runtime properties: no
 *     CountDetails element";
 *   - a subscription's carries one whose five children are renamed to
 *     obfuscated tokens, so the element unmarshals with every field nil and
 *     the SDK dereferences one.
 *
 * That second case is why the guard below exists rather than only an error
 * check. A driver has to be unable to bring the process down whatever a
 * service sends it, and the fields the SDK dereferences are ones the wire
 * format is free to omit.
 *
 * When there is no answer the boards show a dash, which conn.go declares as a
 * degraded capability with a reason - not a zero, which would say the queue is
 * empty.
 */

// counts is what one entity is holding.
//
// int64 because model.Destination's depth is, and the SDK's int32 would have
// to be widened at every use anyway.
type counts struct {
	active     int64
	deadLetter int64
	scheduled  int64
	transfer   int64
	total      int64
}

// queueCounts reads one queue's message counts.
func (c *Conn) queueCounts(ctx context.Context, name string) (counts, bool) {
	if c.config.emulator() {
		return counts{}, false
	}
	return guardedCounts(func() (counts, error) {
		properties, err := c.management.GetQueueRuntimeProperties(ctx, name, nil)
		if err != nil {
			return counts{}, err
		}
		if properties == nil {
			// The entity has gone since the listing. Nil with a nil error is
			// the admin client's way of saying so.
			return counts{}, errGone
		}
		return counts{
			active:     int64(properties.ActiveMessageCount),
			deadLetter: int64(properties.DeadLetterMessageCount),
			scheduled:  int64(properties.ScheduledMessageCount),
			transfer:   int64(properties.TransferMessageCount),
			total:      properties.TotalMessageCount,
		}, nil
	})
}

// topicCounts reads one topic's counts, which are almost none.
//
// A topic holds no messages: a send is copied into every subscription whose
// rules let it through and discarded if none do. The one thing it does hold is
// what has been scheduled and not yet enqueued, because that has not been
// copied anywhere yet.
func (c *Conn) topicCounts(ctx context.Context, name string) (counts, bool) {
	if c.config.emulator() {
		return counts{}, false
	}
	return guardedCounts(func() (counts, error) {
		properties, err := c.management.GetTopicRuntimeProperties(ctx, name, nil)
		if err != nil {
			return counts{}, err
		}
		if properties == nil {
			// The entity has gone since the listing. Nil with a nil error is
			// the admin client's way of saying so.
			return counts{}, errGone
		}
		return counts{scheduled: int64(properties.ScheduledMessageCount)}, nil
	})
}

// subscriptionCounts reads one subscription's backlog.
func (c *Conn) subscriptionCounts(ctx context.Context, topic, name string) (counts, bool) {
	if c.config.emulator() {
		return counts{}, false
	}
	return guardedCounts(func() (counts, error) {
		properties, err := c.management.GetSubscriptionRuntimeProperties(ctx, topic, name, nil)
		if err != nil {
			return counts{}, err
		}
		if properties == nil {
			// The entity has gone since the listing. Nil with a nil error is
			// the admin client's way of saying so.
			return counts{}, errGone
		}
		return counts{
			active:     int64(properties.ActiveMessageCount),
			deadLetter: int64(properties.DeadLetterMessageCount),
			transfer:   int64(properties.TransferMessageCount),
			total:      properties.TotalMessageCount,
		}, nil
	})
}

// guardedCounts turns any failure, including a panic, into "no answer".
//
// The recover is not defensive habit. The SDK dereferences the children of
// CountDetails without checking them, and an entity description is free to
// omit any of them - the emulator's subscriptions do. A count is a figure on a
// board; losing it should cost a dash, never the window.
func guardedCounts(read func() (counts, error)) (held counts, known bool) {
	defer func() {
		if recover() != nil {
			held, known = counts{}, false
		}
	}()
	held, err := read()
	if err != nil {
		return counts{}, false
	}
	return held, true
}
