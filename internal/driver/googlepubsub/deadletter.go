package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * DeadLetterQueues finds the topics subscriptions give up into.
 *
 * This is DeadLetterTopology's shape rather than DeadLetterReader's, and it
 * has to be: a dead-letter topic in Pub/Sub is an ordinary topic that some
 * subscription's DeadLetterPolicy points at. Nothing marks it, nothing is
 * named after a consumer group, and reading one afterwards means subscribing
 * to it like any other topic.
 *
 * Where it differs from the other family with this shape is which object holds
 * the policy. A RabbitMQ queue is declared with a dead-letter exchange, so a
 * source is a queue; here the policy belongs to the *subscription*, so one
 * topic read by three subscriptions can give up into three different places
 * for three different reasons. Every source therefore names both: the topic
 * the message came from, and the subscription that stopped trying.
 *
 * The consumer count is the figure worth reading. A dead-letter topic is a
 * topic, so it stores nothing - a dead letter published to one with no
 * subscription is discarded on the spot, and the messages a system gave up on
 * are exactly the ones nobody notices disappearing. Depth is unknown for the
 * same reason it is unknown everywhere else on this family: no number exists.
 */
func (c *Conn) DeadLetterQueues(ctx context.Context, _ string) ([]*model.DeadLetterQueue, error) {
	// The namespace argument is ignored: a topic name is flat and unique
	// within a project, and Pub/Sub has no tenant or vhost inside one.
	if err := c.live(); err != nil {
		return nil, err
	}

	sources, err := c.deadLetterSources(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	found := make([]*model.DeadLetterQueue, len(names))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, name := range names {
		group.Go(func() error {
			readers, err := c.topicSubscriptions(groupCtx, c.topicPath(name))
			if err != nil {
				// A target that has been deleted, or that lives in another
				// project this credential cannot read, is not an error the
				// board should show: the subscription pointing at it is still
				// listed with the name in its own row.
				if notFound(err) {
					found[index] = deadLetterQueue(name, model.UnknownMetric, sources[name])
					return nil
				}
				return fmt.Errorf("%s: %w", name, err)
			}
			found[index] = deadLetterQueue(name, len(readers), sources[name])
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	kept := make([]*model.DeadLetterQueue, 0, len(found))
	for _, queue := range found {
		if queue != nil {
			kept = append(kept, queue)
		}
	}
	return kept, nil
}

func deadLetterQueue(name string, readers int, sources []*model.DeadLetterSource) *model.DeadLetterQueue {
	sort.Slice(sources, func(a, b int) bool {
		if sources[a].Queue != sources[b].Queue {
			return sources[a].Queue < sources[b].Queue
		}
		return sources[a].Subscription < sources[b].Subscription
	})
	return &model.DeadLetterQueue{
		Name: name,
		// A topic holds nothing a caller can count, dead letters included.
		// Zero would read as "this backlog has been dealt with", which is the
		// one thing this page exists to say and the service cannot support.
		Depth: model.UnknownMetric,
		// How many subscriptions read it. Zero is the state worth finding:
		// every dead letter published here is discarded on arrival.
		Consumers: readers,
		Sources:   sources,
	}
}

/*
 * deadLetterSources walks the project's subscriptions and inverts their
 * policies.
 *
 * One listing rather than a request per topic, because the policy is on the
 * subscription: every dead-letter target in the project is already in the
 * answer to ListSubscriptions, and there is no call that asks a topic what
 * gives up into it.
 *
 * The connection's prefix narrows which subscriptions are read, and the
 * consequence is worth stating: a dead-letter topic every one of whose sources
 * sits outside the prefix is not found here at all. The prefix is the user's
 * own filter, and widening it is what makes such a topic appear.
 */
func (c *Conn) deadLetterSources(ctx context.Context) (map[string][]*model.DeadLetterSource, error) {
	listing := c.client.SubscriptionAdminClient.ListSubscriptions(ctx,
		&pubsubpb.ListSubscriptionsRequest{Project: c.projectPath()})

	sources := map[string][]*model.DeadLetterSource{}
	for {
		subscription, err := listing.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := shortName(subscription.GetName())
		if !c.matchesPrefix(name) {
			continue
		}
		policy := subscription.GetDeadLetterPolicy()
		if policy == nil || policy.GetDeadLetterTopic() == "" {
			continue
		}
		target := shortName(policy.GetDeadLetterTopic())
		sources[target] = append(sources[target], &model.DeadLetterSource{
			// The topic the message came from, and the subscription that gave
			// up on it. Both, because the policy belongs to the subscription:
			// one topic read three ways has three separate answers.
			Queue:        topicNameOf(subscription.GetTopic()),
			Subscription: name,
			// Nothing is re-published through a routing layer - the service
			// moves the message itself - so there is no exchange and no
			// routing key to report.
		})
	}
	return sources, nil
}
