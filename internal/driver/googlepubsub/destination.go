package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"

	"github.com/amigoer/mq-studio/internal/model"
)

// Attribute keys this driver puts on a destination.
//
// A contract between this package and frontend/src/mq/googlepubsub/topics.ts,
// not part of the shared vocabulary.
const (
	AttrPath              = "path"
	AttrRetentionSec      = "retentionSec"
	AttrSubscriptionNames = "subscriptionNames"
	AttrKmsKey            = "kmsKey"
	AttrSchema            = "schema"
	AttrSchemaEncoding    = "schemaEncoding"
	AttrStorageRegions    = "storageRegions"
	AttrState             = "state"
	// AttrLabelPrefix carries one label per attribute. Keys are the user's
	// own, so they are prefixed to keep them apart from the fields above.
	AttrLabelPrefix = "label."
)

// subscriptionFanOut caps how many ListTopicSubscriptions calls run at once.
//
// ListTopics answers with topics and nothing else, so the one figure this
// board exists to show - how many subscriptions a topic has - is a second
// request per topic. Unbounded that is a thousand requests fired together at
// an API with a per-project rate limit, and being throttled would arrive as a
// failed listing rather than a slow one.
const subscriptionFanOut = 16

// subscriptionNamesShown caps how many subscription names a topic row carries.
//
// The count beside it is the true one. A topic may have thousands of
// subscriptions and the row exists to say which few read it, not to reproduce
// the subscriptions board inside a cell.
const subscriptionNamesShown = 25

/*
 * ListDestinations enumerates the project's topics and counts what reads each.
 *
 * Two calls per board rather than one, and the second is the point of the
 * board. A Pub/Sub topic holds nothing: it fans a publish out to whatever
 * subscriptions exist at that moment and discards it if none do. So there is
 * no depth to show and the figure that matters is how many subscriptions are
 * attached - which ListTopics does not report and ListTopicSubscriptions
 * answers one topic at a time.
 *
 * The fan-out is bounded and the failures are tolerated per topic: a listing
 * that races a delete drops the row rather than failing the board, which is
 * the ordinary case in a project several teams share.
 */
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	// The filter's IncludeInternal has nothing to hide. Pub/Sub has no topics
	// of its own: every name in the listing is one somebody created.
	if err := c.live(); err != nil {
		return nil, err
	}

	topics, err := c.listTopics(ctx)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, len(topics))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, topic := range topics {
		group.Go(func() error {
			names, err := c.topicSubscriptions(groupCtx, topic.GetName())
			if err != nil {
				// A topic that has gone since the listing is not an error the
				// board should show. Anything else is, because a throttled or
				// forbidden read would otherwise be a row silently missing.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", shortName(topic.GetName()), err)
			}
			destinations[index] = destinationOf(topic, names)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	kept := make([]*model.Destination, 0, len(destinations))
	for _, destination := range destinations {
		if destination != nil {
			kept = append(kept, destination)
		}
	}
	return kept, nil
}

// DestinationDetail reads one topic.
//
// Not a walk of the listing: GetTopic takes one topic, so a project with a
// thousand of them should not answer for all of them to describe one.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	name, err := requiredName("topic", ref.Name)
	if err != nil {
		return nil, err
	}
	topic, err := c.client.TopicAdminClient.GetTopic(ctx, &pubsubpb.GetTopicRequest{
		Topic: c.topicPath(name),
	})
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no topic named %q in %s", name, c.config.project)
		}
		return nil, err
	}
	names, err := c.topicSubscriptions(ctx, topic.GetName())
	if err != nil {
		return nil, err
	}
	return destinationOf(topic, names), nil
}

// listTopics pages through the project's topics, honouring the profile's
// prefix.
//
// The prefix is applied here rather than by the service, because the API has
// no filter of any kind: ListTopics takes a project and a page size and
// nothing else. So the pages are still fetched in full and the narrowing only
// saves the per-topic request below - which is the expensive half.
func (c *Conn) listTopics(ctx context.Context) ([]*pubsubpb.Topic, error) {
	listing := c.client.TopicAdminClient.ListTopics(ctx, &pubsubpb.ListTopicsRequest{
		Project: c.projectPath(),
	})
	var topics []*pubsubpb.Topic
	for {
		topic, err := listing.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if c.matchesPrefix(shortName(topic.GetName())) {
			topics = append(topics, topic)
		}
	}
	sort.Slice(topics, func(a, b int) bool { return topics[a].GetName() < topics[b].GetName() })
	return topics, nil
}

// topicSubscriptions is every subscription attached to one topic, short names.
//
// Not narrowed by the connection's prefix, deliberately. The prefix is a
// filter on what the boards list; how many readers a topic actually has is a
// fact about the topic, and a count that hid some of them would make a topic
// nothing reads look identical to one three teams read.
func (c *Conn) topicSubscriptions(ctx context.Context, path string) ([]string, error) {
	listing := c.client.TopicAdminClient.ListTopicSubscriptions(ctx,
		&pubsubpb.ListTopicSubscriptionsRequest{Topic: path})
	var names []string
	for {
		subscription, err := listing.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		names = append(names, shortName(subscription))
	}
	sort.Strings(names)
	return names, nil
}

/*
 * destinationOf turns one topic into a canonical destination.
 *
 * Depth is unknown rather than zero, and that is the family rather than a gap
 * here: a topic stores nothing a caller can count. Retention, where it is set,
 * keeps published messages available for a subscription to seek back into -
 * but nothing reports how many are being kept, and zero would read as "this
 * topic is empty" when the truth is "there is no such number".
 *
 * Subscribers is the figure the board leads with, because a topic with none
 * discards everything published to it.
 */
func destinationOf(topic *pubsubpb.Topic, subscriptions []string) *model.Destination {
	attributes := map[string]string{
		AttrPath:  topic.GetName(),
		AttrState: topic.GetState().String(),
	}
	if retention := topic.GetMessageRetentionDuration(); retention != nil {
		attributes[AttrRetentionSec] = strconv.FormatInt(int64(retention.AsDuration().Seconds()), 10)
	}
	if key := topic.GetKmsKeyName(); key != "" {
		attributes[AttrKmsKey] = key
	}
	if schema := topic.GetSchemaSettings(); schema != nil {
		attributes[AttrSchema] = shortName(schema.GetSchema())
		attributes[AttrSchemaEncoding] = schema.GetEncoding().String()
	}
	if policy := topic.GetMessageStoragePolicy(); policy != nil && len(policy.GetAllowedPersistenceRegions()) > 0 {
		attributes[AttrStorageRegions] = strings.Join(policy.GetAllowedPersistenceRegions(), ",")
	}
	for key, value := range topic.GetLabels() {
		attributes[AttrLabelPrefix+key] = value
	}
	if len(subscriptions) > 0 {
		shown := subscriptions
		if len(shown) > subscriptionNamesShown {
			shown = shown[:subscriptionNamesShown]
		}
		attributes[AttrSubscriptionNames] = strings.Join(shown, ",")
	}

	return &model.Destination{
		Ref: model.DestinationRef{Name: shortName(topic.GetName())},
		// A topic is not split. Pub/Sub spreads one across its own servers and
		// reports no shard, no count and no range.
		Partitions:  model.UnknownMetric,
		Subscribers: len(subscriptions),
		// Nothing counts what a topic is holding. Retention keeps messages
		// available to seek back into and reports no figure for them.
		Depth: model.UnknownMetric,
		// No rates anywhere in the admin API. Pub/Sub publishes them to Cloud
		// Monitoring, which is a different API under a different credential,
		// and two samples taken here would be this app's arithmetic presented
		// as the service's.
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		Attributes: attributes,
	}
}
