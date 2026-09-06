package solace

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Dead messages, found by walking the configuration backwards.
 *
 * This is DeadLetterTopology's shape rather than DeadLetterReader's, and the
 * choice is not a style. CapDLQ is a per-entity store the broker names and
 * fills: RocketMQ gives every consumer group a %DLQ% topic named after it, and
 * Service Bus gives every queue and subscription a $DeadLetterQueue. Solace has
 * nothing of the sort. What it has is one pointer - deadMsgQueue, on every
 * queue and every topic endpoint - and the queue it names is an ordinary queue
 * that nothing marks. Reading one afterwards is an ordinary browse, which is
 * the other half of why this is the right port.
 *
 * # The pointer that points nowhere
 *
 * Every endpoint ships with deadMsgQueue set to "#DEAD_MSG_QUEUE", and no
 * broker ships with a queue by that name. So the ordinary state of a fresh
 * Message VPN is that every endpoint is configured to dead-letter into a queue
 * that does not exist, and what actually happens to a message given up on is
 * that it is discarded. That is the single most useful thing this page can
 * say, so a target the Message VPN does not hold is reported with an unknown
 * depth rather than dropped - and because the depths of the targets that do
 * exist are all read successfully or the whole call fails, an unknown depth
 * here means exactly one thing.
 *
 * # And the flag that stops it anyway
 *
 * A queue with respectDmqEligibleEnabled on - the default - moves only a
 * message whose publisher marked it eligible, and most clients mark nothing.
 * So a pointer at a queue that does exist is still not a guarantee. The flag
 * travels to the page on each source, because a reader looking at an empty
 * dead message queue needs to know which of the two reasons they are seeing.
 */

// Source attribute values this driver writes into model.DeadLetterSource.
//
// A contract between this package and frontend/src/mq/solace/deadletters.ts.
// Exchange carries which kind of endpoint points here, because a queue and a
// topic endpoint are configured in different places and a reader fixing one
// needs to know which.
const (
	SourceQueue         = "queue"
	SourceTopicEndpoint = "topicEndpoint"

	// The eligibility rule, carried on the source because a pointer at a real
	// queue still moves nothing when the endpoint respects a mark no publisher
	// sets. A reader looking at an empty dead message queue needs to know
	// which of the two reasons they are seeing.
	SourceMovesEverything = "moves-everything"
	SourceMovesMarkedOnly = "moves-marked-only"
)

// topicEndpointRow is the shape the topic endpoint collection answers with,
// reduced to what this page reads.
type topicEndpointRow struct {
	TopicEndpointName  string `json:"topicEndpointName"`
	DeadMsgQueue       string `json:"deadMsgQueue"`
	MaxRedeliveryCount int    `json:"maxRedeliveryCount"`
	RespectDmqEligible bool   `json:"respectDmqEligibleEnabled"`
}

// DeadLetterQueues finds the queues something else dead-letters into.
func (c *Conn) DeadLetterQueues(ctx context.Context, _ string) ([]*model.DeadLetterQueue, error) {
	// The namespace argument is ignored: a connection is one Message VPN, and
	// a listing that took another one would be reading outside its own scope.
	if err := c.live(); err != nil {
		return nil, err
	}

	queues, err := c.ListDestinations(ctx, model.DestinationFilter{})
	if err != nil {
		return nil, err
	}
	endpoints, err := listMonitor[topicEndpointRow](ctx, c.semp,
		"/msgVpns/"+segment(c.vpn)+"/topicEndpoints?select=topicEndpointName,deadMsgQueue,"+
			"maxRedeliveryCount,respectDmqEligibleEnabled")
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*model.Destination, len(queues))
	for _, queue := range queues {
		byName[queue.Ref.Name] = queue
	}

	sources := make(map[string][]*model.DeadLetterSource, 4)
	add := func(target, from, kind string, redelivery int, respects bool) {
		target = strings.TrimSpace(target)
		if target == "" || target == from {
			return
		}
		sources[target] = append(sources[target], &model.DeadLetterSource{
			Queue:    from,
			Exchange: kind,
			// The threshold, because it is what decides when a message travels
			// this pointer at all, and the eligibility rule beside it, because
			// a pointer at a real queue still moves nothing when the endpoint
			// respects a mark no publisher sets.
			RoutingKey: strconv.Itoa(redelivery),
			Subscription: map[bool]string{
				true:  SourceMovesMarkedOnly,
				false: SourceMovesEverything,
			}[respects],
		})
	}

	for _, queue := range queues {
		add(queue.Attribute(AttrDeadMsgQueue), queue.Ref.Name, SourceQueue,
			atoiOr(queue.Attribute(AttrMaxRedelivery)), queue.Attribute(AttrRespectDmq) == "true")
	}
	for _, endpoint := range endpoints {
		add(endpoint.DeadMsgQueue, endpoint.TopicEndpointName, SourceTopicEndpoint,
			endpoint.MaxRedeliveryCount, endpoint.RespectDmqEligible)
	}

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	found := make([]*model.DeadLetterQueue, 0, len(names))
	for _, name := range names {
		entry := &model.DeadLetterQueue{
			Namespace: c.vpn,
			Name:      name,
			// Unknown until a queue of that name is found, and it stays
			// unknown for the one case this page exists to show: the pointer
			// every endpoint ships with names a queue no broker creates.
			Depth:   model.UnknownMetric,
			Sources: sources[name],
		}
		if queue := byName[name]; queue != nil {
			entry.Depth = queue.Depth
			if queue.Subscribers >= 0 {
				entry.Consumers = queue.Subscribers
			}
		}
		found = append(found, entry)
	}
	return found, nil
}

func atoiOr(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}
