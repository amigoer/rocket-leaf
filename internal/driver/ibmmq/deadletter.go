package ibmmq

import (
	"context"
	"sort"
	"strconv"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Dead letters, found by walking the configuration backwards.
 *
 * This is DeadLetterTopology's shape rather than DeadLetterReader's, and the
 * choice is not a style. CapDLQ is a per-entity store the broker names and
 * fills: RocketMQ gives every consumer group a %DLQ% topic named after it, and
 * Service Bus gives every queue and subscription a $DeadLetterQueue. IBM MQ
 * has nothing of the sort. What it has is two pointers, and the queues they
 * point at are ordinary queues that nothing marks:
 *
 *   - the queue manager's DEADQ, which is where it puts anything it cannot
 *     deliver - a message for a queue that is full, put-inhibited or gone, or
 *     one a channel could not hand over;
 *   - a queue's BOQNAME, which is where an application is expected to move a
 *     message it has backed out more than BOTHRESH times.
 *
 * So a dead-letter queue here is found by reading every queue's configuration
 * and inverting it, exactly as RabbitMQ's dead-letter exchange and SQS's
 * redrive policy are. Reading one afterwards is an ordinary browse, which is
 * the other half of why this is the right port: nothing about the queue is
 * special once it has been found.
 *
 * # The one thing this page cannot show
 *
 * A dead letter carries a dead-letter header in front of its payload, so the
 * queue manager stores it as MQDEAD and the messaging interface will not
 * decode it. The messages page lists them with their identifiers and says why
 * their bodies are unavailable; this page reports the queues, their depths and
 * what points at them, which is what the page is for.
 */

// DeadLetterQueues finds the queues something else dead-letters into.
func (c *Conn) DeadLetterQueues(ctx context.Context, _ string) ([]*model.DeadLetterQueue, error) {
	// The namespace argument is ignored: a queue name is flat and unique
	// within its queue manager, and MQ has nothing inside one for a name to
	// belong to.
	if err := c.live(); err != nil {
		return nil, err
	}

	deadLetter, err := c.deadLetterQueueName(ctx)
	if err != nil {
		return nil, err
	}

	// Internal objects included, because SYSTEM.DEAD.LETTER.QUEUE is the
	// default DEADQ on an installation nobody has changed - hiding it would
	// leave this page empty on exactly the deployment that has not been
	// configured, which is the one worth looking at.
	queues, err := c.listQueues(ctx, model.DestinationFilter{IncludeInternal: true}, deadLetter)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*model.Destination, len(queues))
	sources := make(map[string][]*model.DeadLetterSource, 4)
	for _, queue := range queues {
		byName[queue.Ref.Name] = queue
	}

	// The queue manager itself is a source, and it is the one that matters: it
	// is what fills the DEADQ, and there is no queue to name in its place.
	if deadLetter != "" {
		sources[deadLetter] = append(sources[deadLetter], &model.DeadLetterSource{
			Queue: c.qmgr,
			// Exchange carries the attribute that points here rather than a
			// RabbitMQ exchange, which this family has none of. Without it a
			// reader could not tell the queue manager's own dead-letter queue
			// from a backout queue some application named.
			Exchange: attributeDEADQ,
		})
	}

	for _, queue := range queues {
		target := queue.Attribute(AttrBackoutQueue)
		if target == "" || target == queue.Ref.Name {
			continue
		}
		sources[target] = append(sources[target], &model.DeadLetterSource{
			Queue:    queue.Ref.Name,
			Exchange: attributeBOQNAME,
			// RoutingKey carries the threshold, because that is what decides
			// when a message travels this pointer at all. A backout queue with
			// a threshold of zero receives nothing: the queue manager counts
			// the backouts and the application does the moving, and with no
			// threshold there is nothing for it to act on.
			RoutingKey: queue.Attribute(AttrBackoutThreshold),
		})
	}

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	found := make([]*model.DeadLetterQueue, 0, len(names))
	for _, name := range names {
		queue := byName[name]
		entry := &model.DeadLetterQueue{
			Name:    name,
			Depth:   model.UnknownMetric,
			Sources: sources[name],
		}
		if queue != nil {
			entry.Depth = queue.Depth
			// Applications holding it open for input, which is the closest
			// thing to a consumer here: a dead-letter queue with one is a
			// handler somebody is running, and one without is a backlog
			// nobody is looking at.
			if open := queue.Attribute(AttrOpenInput); open != "" {
				if count, err := strconv.Atoi(open); err == nil {
					entry.Consumers = count
				}
			}
		}
		found = append(found, entry)
	}
	return found, nil
}

// The attribute names that point at a dead-letter queue. They are carried
// through to the page because the two are not interchangeable: the queue
// manager fills its DEADQ itself, and a backout queue is filled by whichever
// application decided to give up.
const (
	attributeDEADQ   = "DEADQ"
	attributeBOQNAME = "BOQNAME"
)
