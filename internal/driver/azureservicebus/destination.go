package azureservicebus

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Attribute keys this driver puts on a destination.
//
// A contract between this package and frontend/src/mq/azureservicebus/topics.ts,
// not part of the shared vocabulary.
const (
	// AttrEntityType is queue or topic. It is the first thing a row shows,
	// because the two share this board and nothing else about them is the
	// same: a queue holds messages and a topic holds none.
	AttrEntityType = "entityType"

	AttrStatus                  = "status"
	AttrLockDurationSec         = "lockDurationSec"
	AttrMaxDeliveryCount        = "maxDeliveryCount"
	AttrTTLSec                  = "ttlSec"
	AttrAutoDeleteOnIdleSec     = "autoDeleteOnIdleSec"
	AttrMaxSizeMB               = "maxSizeMb"
	AttrRequiresSession         = "requiresSession"
	AttrRequiresDuplicateDetect = "requiresDuplicateDetection"
	AttrDeadLetterOnExpiry      = "deadLetterOnExpiry"
	AttrPartitioned             = "partitioned"
	AttrForwardTo               = "forwardTo"
	AttrForwardDeadLettersTo    = "forwardDeadLettersTo"
	AttrDeadLetterCount         = "deadLetterCount"
	AttrScheduledCount          = "scheduledCount"
	AttrSubscriptionNames       = "subscriptionNames"
)

// Entity types, as the board spells them.
const (
	EntityQueue = "queue"
	EntityTopic = "topic"
)

// subscriptionFanOut caps how many subscription listings run at once.
//
// A topic reports how many subscriptions it has only in its runtime
// properties, which the emulator cannot serve, so the count is a listing per
// topic. Unbounded that is a request per topic fired together at a management
// API that throttles, and being throttled would arrive as a failed board
// rather than a slow one.
const subscriptionFanOut = 8

// subscriptionNamesShown caps how many subscription names a topic row carries.
//
// The count beside it is the true one. A topic may have two thousand
// subscriptions and the row exists to say which few read it, not to reproduce
// the subscriptions board inside a cell.
const subscriptionNamesShown = 25

/*
 * ListDestinations enumerates the namespace's queues and topics.
 *
 * Both, on one board, because they are the same kind of thing to create,
 * configure and delete - same properties, same operations, same page. What
 * separates them is where the messages end up: a queue holds them itself and a
 * topic holds none at all, copying each into every subscription whose rules
 * let it through. The row says which it is, and the depth column says the
 * rest: a topic's is never a number, because there is nothing in it to count.
 */
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	// The filter's IncludeInternal has nothing to hide. Service Bus keeps no
	// entities of its own: every name in the listing is one somebody created,
	// and the $DeadLetterQueue sub-entities are not listed at all.
	if err := c.live(); err != nil {
		return nil, err
	}

	queues, err := c.listQueues(ctx)
	if err != nil {
		return nil, err
	}
	topics, err := c.listTopics(ctx)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(queues)+len(topics))
	for _, queue := range queues {
		destinations = append(destinations, c.queueDestination(ctx, queue.QueueName, queue.QueueProperties))
	}

	// One subscription listing per topic, bounded. It is the figure the row
	// exists for: a topic with no subscription discards everything sent to it.
	names := make([][]string, len(topics))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, topic := range topics {
		group.Go(func() error {
			attached, err := c.subscriptionNames(groupCtx, topic.TopicName)
			if err != nil {
				// A topic that has gone since the listing is not an error the
				// board should show. Anything else is, because a throttled or
				// forbidden read would otherwise be a row silently missing.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", topic.TopicName, err)
			}
			names[index] = attached
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	for index, topic := range topics {
		destinations = append(destinations,
			c.topicDestination(ctx, topic.TopicName, topic.TopicProperties, names[index]))
	}

	sort.Slice(destinations, func(a, b int) bool {
		return destinations[a].Ref.Name < destinations[b].Ref.Name
	})
	return destinations, nil
}

/*
 * DestinationDetail reads one entity.
 *
 * Nothing in the ref says whether it is a queue or a topic and nothing needs
 * to: a namespace's queues and topics share one name space, so a name is
 * unambiguous. describeEntity is what works out which.
 */
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	name, err := requiredName("entity", ref.Name)
	if err != nil {
		return nil, err
	}

	described, err := c.describeEntity(ctx, name)
	if err != nil {
		return nil, err
	}
	switch {
	case described.queue != nil:
		return c.queueDestination(ctx, name, *described.queue), nil
	case described.topic != nil:
		attached, err := c.subscriptionNames(ctx, name)
		if err != nil {
			return nil, err
		}
		return c.topicDestination(ctx, name, *described.topic, attached), nil
	}
	return nil, fmt.Errorf("no queue or topic named %q in %s", name, c.config.namespace)
}

// described is one entity resolved to its kind and its settings. Both pointers
// nil means there is no such entity.
type described struct {
	queue *admin.QueueProperties
	topic *admin.TopicProperties
}

func (d described) kind() string {
	switch {
	case d.queue != nil:
		return EntityQueue
	case d.topic != nil:
		return EntityTopic
	default:
		return ""
	}
}

/*
 * describeEntity works out whether a name is a queue or a topic.
 *
 * It has to, and the reason is a sharp edge in the API rather than a
 * convenience here. Both kinds are addressed at the same Atom path - GET
 * /<name> - and the SDK's GetQueue and GetTopic send exactly that request,
 * differing only in which element they expect back. So GetQueue on a topic
 * does not fail: it parses a TopicDescription looking for a QueueDescription,
 * finds none, and hands back a response with every field nil. A driver that
 * trusted a non-nil response would report every topic as a queue with no
 * settings, which is what it did before this existed.
 *
 * Status is the discriminator because the service sets it on every entity it
 * describes - Active, Disabled, SendDisabled or ReceiveDisabled - so its
 * absence means the element was never there. A missing entity is different
 * again: the admin client answers a nil response and a nil error.
 */
func (c *Conn) describeEntity(ctx context.Context, name string) (described, error) {
	queue, err := c.management.GetQueue(ctx, name, nil)
	if err != nil {
		return described{}, err
	}
	if queue != nil && queue.Status != nil {
		properties := queue.QueueProperties
		return described{queue: &properties}, nil
	}

	topic, err := c.management.GetTopic(ctx, name, nil)
	if err != nil {
		return described{}, err
	}
	if topic != nil && topic.Status != nil {
		properties := topic.TopicProperties
		return described{topic: &properties}, nil
	}
	return described{}, nil
}

// listQueues pages through the namespace's queues, honouring the prefix.
//
// The prefix is applied here rather than by the service, because the
// management API has no filter of any kind: the listing takes a skip and a top
// and nothing else.
func (c *Conn) listQueues(ctx context.Context) ([]admin.QueueItem, error) {
	pager := c.management.NewListQueuesPager(nil)
	var queues []admin.QueueItem
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, queue := range page.Queues {
			if c.matchesPrefix(queue.QueueName) {
				queues = append(queues, queue)
			}
		}
	}
	return queues, nil
}

func (c *Conn) listTopics(ctx context.Context) ([]admin.TopicItem, error) {
	pager := c.management.NewListTopicsPager(nil)
	var topics []admin.TopicItem
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, topic := range page.Topics {
			if c.matchesPrefix(topic.TopicName) {
				topics = append(topics, topic)
			}
		}
	}
	return topics, nil
}

// subscriptionNames is every subscription on one topic.
//
// Not narrowed by the connection's prefix, deliberately. The prefix filters
// what the boards list; how many readers a topic actually has is a fact about
// the topic, and a count that hid some of them would make a topic nothing
// reads look identical to one three teams read.
func (c *Conn) subscriptionNames(ctx context.Context, topic string) ([]string, error) {
	pager := c.management.NewListSubscriptionsPager(topic, nil)
	names := make([]string, 0, 4)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, subscription := range page.Subscriptions {
			names = append(names, subscription.SubscriptionName)
		}
	}
	sort.Strings(names)
	return names, nil
}

/*
 * queueDestination turns one queue into a canonical destination.
 *
 * Subscribers is unknown rather than zero, and that is the family: a queue is
 * read by whoever opens a receiver on it, and Service Bus keeps no register of
 * who that was. Zero would read as "nothing is consuming this", which is the
 * one thing the column cannot support.
 */
func (c *Conn) queueDestination(ctx context.Context, name string, properties admin.QueueProperties) *model.Destination {
	attributes := map[string]string{
		AttrEntityType:              EntityQueue,
		AttrRequiresSession:         boolOf(properties.RequiresSession),
		AttrRequiresDuplicateDetect: boolOf(properties.RequiresDuplicateDetection),
		AttrDeadLetterOnExpiry:      boolOf(properties.DeadLetteringOnMessageExpiration),
		AttrPartitioned:             boolOf(properties.EnablePartitioning),
	}
	if properties.Status != nil {
		attributes[AttrStatus] = string(*properties.Status)
	}
	if properties.MaxDeliveryCount != nil {
		attributes[AttrMaxDeliveryCount] = strconv.FormatInt(int64(*properties.MaxDeliveryCount), 10)
	}
	if properties.MaxSizeInMegabytes != nil {
		attributes[AttrMaxSizeMB] = strconv.FormatInt(int64(*properties.MaxSizeInMegabytes), 10)
	}
	putDuration(attributes, AttrLockDurationSec, properties.LockDuration)
	putDuration(attributes, AttrTTLSec, properties.DefaultMessageTimeToLive)
	putDuration(attributes, AttrAutoDeleteOnIdleSec, properties.AutoDeleteOnIdle)
	putString(attributes, AttrForwardTo, properties.ForwardTo)
	putString(attributes, AttrForwardDeadLettersTo, properties.ForwardDeadLetteredMessagesTo)

	destination := &model.Destination{
		Ref: model.DestinationRef{Name: name},
		// A queue is not split. Partitioning spreads one across the service's
		// own brokers and reports no shard, no count and no range - which is
		// why it is a flag among the attributes rather than a number here.
		Partitions: model.UnknownMetric,
		// Nothing registers as a consumer.
		Subscribers: model.UnknownMetric,
		Depth:       model.UnknownMetric,
		// No rates anywhere in the management API. Service Bus publishes them
		// to Azure Monitor, which is a different API under a different
		// credential, and two samples taken here would be this app's
		// arithmetic presented as the service's.
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		Attributes: attributes,
	}

	if held, known := c.queueCounts(ctx, name); known {
		destination.Depth = held.active
		attributes[AttrDeadLetterCount] = strconv.FormatInt(held.deadLetter, 10)
		attributes[AttrScheduledCount] = strconv.FormatInt(held.scheduled, 10)
	}
	return destination
}

/*
 * topicDestination turns one topic into a canonical destination.
 *
 * Depth is unknown rather than zero, always, and that is the family rather
 * than a gap here: a topic stores nothing. A send is copied into every
 * subscription whose rules let it through and discarded if none do, so there
 * is no backlog to count - and zero would read as "this topic is empty", which
 * is true of every topic that ever existed.
 *
 * Subscribers is the figure the row leads with, because a topic with none
 * discards everything sent to it.
 */
func (c *Conn) topicDestination(
	ctx context.Context, name string, properties admin.TopicProperties, subscriptions []string,
) *model.Destination {
	attributes := map[string]string{
		AttrEntityType:              EntityTopic,
		AttrRequiresDuplicateDetect: boolOf(properties.RequiresDuplicateDetection),
		AttrPartitioned:             boolOf(properties.EnablePartitioning),
	}
	if properties.Status != nil {
		attributes[AttrStatus] = string(*properties.Status)
	}
	if properties.MaxSizeInMegabytes != nil {
		attributes[AttrMaxSizeMB] = strconv.FormatInt(int64(*properties.MaxSizeInMegabytes), 10)
	}
	putDuration(attributes, AttrTTLSec, properties.DefaultMessageTimeToLive)
	putDuration(attributes, AttrAutoDeleteOnIdleSec, properties.AutoDeleteOnIdle)
	if len(subscriptions) > 0 {
		shown := subscriptions
		if len(shown) > subscriptionNamesShown {
			shown = shown[:subscriptionNamesShown]
		}
		attributes[AttrSubscriptionNames] = strings.Join(shown, ",")
	}

	destination := &model.Destination{
		Ref:         model.DestinationRef{Name: name},
		Partitions:  model.UnknownMetric,
		Subscribers: len(subscriptions),
		Depth:       model.UnknownMetric,
		RateIn:      model.UnknownMetric,
		RateOut:     model.UnknownMetric,
		Attributes:  attributes,
	}
	// A topic does hold one thing: messages scheduled for later, which have
	// not been copied anywhere yet. That is the only count it has.
	if held, known := c.topicCounts(ctx, name); known {
		attributes[AttrScheduledCount] = strconv.FormatInt(held.scheduled, 10)
	}
	return destination
}

func boolOf(value *bool) string {
	return strconv.FormatBool(value != nil && *value)
}

// putDuration writes an ISO-8601 duration as whole seconds.
//
// The API spells every duration that way - PT1M, P10675199DT2H48M5.4775807S -
// and the boards want a number. The outsized one is the service's own way of
// saying "never", and it survives as the enormous number it is rather than
// being turned into a zero that would read as "immediately".
func putDuration(attributes map[string]string, key string, value *string) {
	if value == nil || *value == "" {
		return
	}
	seconds, ok := isoSeconds(*value)
	if !ok {
		return
	}
	attributes[key] = strconv.FormatInt(seconds, 10)
}

func putString(attributes map[string]string, key string, value *string) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return
	}
	attributes[key] = strings.TrimSpace(*value)
}
