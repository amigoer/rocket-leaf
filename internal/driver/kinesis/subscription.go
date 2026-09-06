package kinesis

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Attribute keys this driver puts on a subscription.
//
// A contract between this package and frontend/src/mq/kinesis/subscriptions.ts,
// not part of the shared vocabulary.
const (
	AttrConsumerARN = "consumerArn"
	// AttrConsumerStatus is CREATING, DELETING or ACTIVE. Only an ACTIVE
	// consumer can be subscribed to.
	AttrConsumerStatus = "consumerStatus"
	AttrConsumerSince  = "createdAt"
	AttrStream         = "stream"
)

// consumerFanOut caps how many streams are asked for their consumers at once.
//
// ListStreamConsumers takes one stream, and the canonical listing asks for
// every subscription the connection can see - so a region with a thousand
// streams is a thousand requests, which unbounded would arrive as a throttled
// listing rather than a slow one.
const consumerFanOut = 8

/*
 * ListSubscriptions enumerates every registered consumer in the region.
 *
 * A registered consumer is an enhanced fan-out consumer: a real object with a
 * name and an ARN, created and removed on its own, and the only reader a
 * Kinesis stream knows anything about at all. A classic consumer - the KCL, a
 * Lambda event source, anything calling GetRecords - registers nothing and
 * keeps its position in a DynamoDB table this connection never sees, so it
 * cannot appear here and its absence is not a gap in this driver.
 *
 * One call per stream, because the API has no account-wide listing: the port
 * takes no destination and the service takes nothing else.
 */
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	streams, err := c.listStreamNames(ctx)
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	var collected []*model.Subscription
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(consumerFanOut)
	for _, stream := range streams {
		group.Go(func() error {
			consumers, err := c.streamConsumers(groupCtx, stream)
			if err != nil {
				// A stream that has gone since the listing is not an error the
				// board should show.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", stream, err)
			}
			mu.Lock()
			collected = append(collected, consumers...)
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(collected, func(i, j int) bool {
		if collected[i].Ref.Namespace != collected[j].Ref.Namespace {
			return collected[i].Ref.Namespace < collected[j].Ref.Namespace
		}
		return collected[i].Ref.Name < collected[j].Ref.Name
	})
	for index, subscription := range collected {
		subscription.ID = index + 1
	}
	return collected, nil
}

/*
 * SubscriptionDetail reads one consumer.
 *
 * The ref's namespace is the stream. A consumer belongs to exactly one stream,
 * its name is unique only within that stream, and every call that names one
 * takes the stream's ARN - so a name on its own addresses nothing, which is
 * the same shape the record ids have.
 */
func (c *Conn) SubscriptionDetail(ctx context.Context, ref model.SubscriptionRef) (*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	stream, name, err := consumerRef(ref)
	if err != nil {
		return nil, err
	}
	arn, err := c.streamARN(ctx, stream)
	if err != nil {
		return nil, err
	}

	out, err := c.client.DescribeStreamConsumer(ctx, &awskinesis.DescribeStreamConsumerInput{
		StreamARN:    aws.String(arn),
		ConsumerName: aws.String(name),
	})
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no consumer named %q registered on %s", name, stream)
		}
		return nil, err
	}
	description := out.ConsumerDescription
	return subscriptionOf(stream, types.Consumer{
		ConsumerARN:               description.ConsumerARN,
		ConsumerName:              description.ConsumerName,
		ConsumerStatus:            description.ConsumerStatus,
		ConsumerCreationTimestamp: description.ConsumerCreationTimestamp,
	}), nil
}

/*
 * CreateSubscription registers a consumer on a stream.
 *
 * It is not a subscribe: registering reserves the name and the dedicated two
 * megabytes a second of read throughput this consumer gets on every shard, and
 * an application then calls SubscribeToShard against the ARN. Nothing here
 * reads on its behalf, and the registration outlives whatever does.
 *
 * A stream takes at most twenty of these, which is a quota rather than a
 * setting - the service refuses the twenty-first, and that refusal is passed
 * through because there is nothing this app could do about it.
 */
func (c *Conn) CreateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	stream, name, err := consumerRef(spec.Ref)
	if err != nil {
		return err
	}
	arn, err := c.streamARN(ctx, stream)
	if err != nil {
		return err
	}

	_, err = c.client.RegisterStreamConsumer(ctx, &awskinesis.RegisterStreamConsumerInput{
		StreamARN:    aws.String(arn),
		ConsumerName: aws.String(name),
	})
	if err != nil {
		var exists *types.ResourceInUseException
		if errors.As(err, &exists) {
			return fmt.Errorf("a consumer named %q is already registered on %s: %w",
				name, stream, err)
		}
		return err
	}
	return nil
}

// UpdateSubscription is not offered. See errNoConsumerUpdate.
func (c *Conn) UpdateSubscription(context.Context, model.SubscriptionSpec) error {
	return errNoConsumerUpdate
}

// errNoConsumerUpdate is why the update half of SubscriptionAdmin does
// nothing here.
//
// A registered consumer is a name, an ARN, a status and a creation time, and
// every one of those is the service's to set. There is no filter, no
// acknowledgement deadline, no dead-letter policy and no retention - a
// consumer configures nothing, because everything that could be configured
// belongs to the stream.
var errNoConsumerUpdate = errors.New(
	"a registered consumer has nothing to change: its name, ARN, status and creation " +
		"time are all the service's, and every setting a reader might want belongs to " +
		"the stream instead")

/*
 * RemoveSubscription deregisters a consumer.
 *
 * It stops the dedicated throughput being reserved and frees the name. It does
 * not stop anything reading: an application still holding a subscription is
 * cut off at its next call rather than at this one, and a classic consumer -
 * which registered nothing - is unaffected entirely.
 */
func (c *Conn) RemoveSubscription(ctx context.Context, ref model.SubscriptionRef) error {
	if err := c.live(); err != nil {
		return err
	}
	stream, name, err := consumerRef(ref)
	if err != nil {
		return err
	}
	arn, err := c.streamARN(ctx, stream)
	if err != nil {
		return err
	}

	_, err = c.client.DeregisterStreamConsumer(ctx, &awskinesis.DeregisterStreamConsumerInput{
		StreamARN:    aws.String(arn),
		ConsumerName: aws.String(name),
	})
	if err != nil {
		if notFound(err) {
			return fmt.Errorf("no consumer named %q registered on %s", name, stream)
		}
		return err
	}
	return nil
}

/*
 * streamConsumers lists one stream's registered consumers.
 *
 * The paging guard is not decoration. The emulator this suite runs against
 * returns a NextToken on the last page as well as on a full one, where the
 * service omits it - so a loop driven by the token alone makes one wasted call
 * per stream, and would spin if the empty page carried a token too. Stopping
 * on an empty page is what makes both behaviours terminate.
 */
func (c *Conn) streamConsumers(ctx context.Context, stream string) ([]*model.Subscription, error) {
	arn, err := c.streamARN(ctx, stream)
	if err != nil {
		return nil, err
	}

	var subscriptions []*model.Subscription
	var token *string
	for {
		// The ARN goes on every page, unlike ListShards' stream name: this
		// operation requires it and the SDK refuses the request without one,
		// token or no token.
		page, err := c.client.ListStreamConsumers(ctx, &awskinesis.ListStreamConsumersInput{
			StreamARN: aws.String(arn),
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, consumer := range page.Consumers {
			subscriptions = append(subscriptions, subscriptionOf(stream, consumer))
		}
		if len(page.Consumers) == 0 || page.NextToken == nil || aws.ToString(page.NextToken) == "" {
			return subscriptions, nil
		}
		token = page.NextToken
	}
}

/*
 * subscriptionOf turns one registered consumer into a canonical subscription.
 *
 * Every figure a consumers page would want is UnknownMetric, and that is the
 * service rather than this driver. A registered consumer carries no position,
 * so there is no backlog to compute; the stream keeps no record of who is
 * attached, so there are no members to count; and nothing anywhere reports a
 * consume rate - the per-consumer metrics are CloudWatch's, under a different
 * permission.
 *
 * MillisBehindLatest on a GetRecords response is not the missing number. It is
 * how far behind the tip that one read was, which belongs to whoever made the
 * call - this app, when it browses - and has nothing to do with any registered
 * consumer's progress.
 */
func subscriptionOf(stream string, consumer types.Consumer) *model.Subscription {
	attributes := map[string]string{
		AttrConsumerARN:    aws.ToString(consumer.ConsumerARN),
		AttrConsumerStatus: string(consumer.ConsumerStatus),
		AttrStream:         stream,
	}
	if consumer.ConsumerCreationTimestamp != nil {
		attributes[AttrConsumerSince] = strconv.FormatInt(
			consumer.ConsumerCreationTimestamp.UTC().UnixMilli(), 10)
	}

	return &model.Subscription{
		Ref: model.SubscriptionRef{
			Namespace: stream,
			Name:      aws.ToString(consumer.ConsumerName),
		},
		Status:  consumerStatus(consumer.ConsumerStatus),
		Members: model.UnknownMetric,
		// Exactly one, always: a consumer is registered on a stream and
		// cannot be moved or shared.
		Destinations: 1,
		Backlog:      model.UnknownMetric,
		RateOut:      model.UnknownMetric,
		LastUpdated:  time.Now().UTC().Format(time.RFC3339),
		Attributes:   attributes,
	}
}

// consumerStatus maps the three states a registration can be in.
//
// CREATING and DELETING are warnings rather than offline: the consumer exists
// and is on its way somewhere, and a page that showed it as down would be
// describing a registration that is about to work.
func consumerStatus(status types.ConsumerStatus) model.SubscriptionStatus {
	switch status {
	case types.ConsumerStatusActive:
		return model.SubscriptionOnline
	case types.ConsumerStatusCreating, types.ConsumerStatusDeleting:
		return model.SubscriptionWarning
	default:
		return model.SubscriptionOffline
	}
}

// consumerRef splits a subscription ref into the stream and the consumer name.
//
// Both halves are required, for the same reason a record needs its shard: a
// consumer name is unique only within its stream, and every call that names
// one takes the stream's ARN as well.
func consumerRef(ref model.SubscriptionRef) (stream, name string, err error) {
	stream = strings.TrimSpace(ref.Namespace)
	name = strings.TrimSpace(ref.Name)
	if stream == "" {
		return "", "", errors.New(
			"a registered consumer belongs to one stream, and the stream has to be named: " +
				"a consumer name is unique only within it")
	}
	if name == "" {
		return "", "", errors.New("a registered consumer needs a name")
	}
	return stream, name, nil
}
