package sqs

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * DeadLetterQueues finds the queues other queues redrive into.
 *
 * This is DeadLetterTopology's shape rather than DeadLetterReader's, and it has
 * to be: a dead-letter queue in SQS is an ordinary queue that another queue's
 * RedrivePolicy points at. Nothing marks it, nothing is named after a consumer
 * group - there are no consumer groups - and reading one afterwards is an
 * ordinary browse.
 *
 * Finding one is therefore a walk backwards. The listing already carries every
 * queue's redrive target, so the candidates cost no extra request; what each
 * one's sources actually are is then read from the service, because
 * ListDeadLetterSourceQueues answers for the whole region and the listing only
 * sees what the connection's prefix let through.
 *
 * The consequence of that prefix is worth stating: a dead-letter queue every
 * one of whose sources sits outside it is not found here at all. The prefix is
 * the user's own filter, and widening it is what makes such a queue appear.
 */
func (c *Conn) DeadLetterQueues(ctx context.Context, _ string) ([]*model.DeadLetterQueue, error) {
	// The namespace argument is ignored: a queue name is flat and unique
	// within an account and region, and SQS has no tenant, vhost or account
	// inside it for one to belong to.
	if err := c.live(); err != nil {
		return nil, err
	}

	queues, err := c.ListDestinations(ctx, model.DestinationFilter{})
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*model.Destination, len(queues))
	targets := make(map[string]bool)
	for _, queue := range queues {
		byName[queue.Ref.Name] = queue
		if target := queue.Attribute(AttrDeadLetterQueue); target != "" {
			targets[target] = true
		}
	}

	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	found := make([]*model.DeadLetterQueue, len(names))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(attributeFanOut)
	for index, name := range names {
		group.Go(func() error {
			sources, err := c.deadLetterSources(groupCtx, name)
			if err != nil {
				// A target that has been deleted, or that lives in another
				// account this connection cannot read, is not an error the
				// board should show: the queue pointing at it is still listed
				// with the name in its own row.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", name, err)
			}
			found[index] = &model.DeadLetterQueue{
				Name:  name,
				Depth: depthOf(byName[name]),
				// SQS keeps no record of who reads a queue, so this cannot be
				// zero: zero would read as "nothing is draining this
				// backlog", which is the one thing the page exists to say and
				// the service cannot support.
				Consumers: model.UnknownMetric,
				Sources:   sources,
			}
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

// depthOf reads a listed queue's depth, or reports it unknown for a target the
// listing did not carry - one outside the connection's prefix, or in another
// account.
func depthOf(queue *model.Destination) int64 {
	if queue == nil {
		return model.UnknownMetric
	}
	return queue.Depth
}

// deadLetterSources asks the service which queues redrive into this one.
//
// Read from ListDeadLetterSourceQueues rather than inverted from the listing,
// because the two answer different questions: the listing sees what the
// connection's prefix let through, and this answers for the whole region.
//
// The fields a RabbitMQ source carries stay empty, and none of them has a
// counterpart. A redrive policy belongs to the queue rather than to a reader
// of it, so there is no subscription; and a message is moved rather than
// re-published, so there is no exchange and no routing key.
func (c *Conn) deadLetterSources(ctx context.Context, name string) ([]*model.DeadLetterSource, error) {
	url, err := c.queueURL(ctx, name)
	if err != nil {
		return nil, err
	}

	sources := make([]*model.DeadLetterSource, 0, 4)
	var token *string
	for {
		page, err := c.client.ListDeadLetterSourceQueues(ctx, &awssqs.ListDeadLetterSourceQueuesInput{
			QueueUrl:  aws.String(url),
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, sourceURL := range page.QueueUrls {
			sources = append(sources, &model.DeadLetterSource{Queue: queueNameOf(sourceURL)})
		}
		if page.NextToken == nil || aws.ToString(page.NextToken) == "" {
			break
		}
		token = page.NextToken
	}
	sort.Slice(sources, func(a, b int) bool { return sources[a].Queue < sources[b].Queue })
	return sources, nil
}
