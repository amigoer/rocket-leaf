package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Subscriptions, which is what this family has and the one before it did not.
 *
 * An SQS queue is one object: it is named, it holds messages, and a consumer
 * is whoever calls ReceiveMessage. Pub/Sub splits that in two, and the split
 * is not cosmetic - a subscription is created, listed and deleted on its own,
 * it outlives its topic, it carries the whole of the delivery configuration,
 * and its backlog is its own rather than the topic's. Two subscriptions on one
 * topic each receive every message and each fall behind separately.
 *
 * What a subscription does not carry is that backlog as a number. There is no
 * num_undelivered_messages on the Subscription the admin API returns; the
 * figure is a Cloud Monitoring metric, under a different API and a different
 * credential, and the emulator serves no Monitoring at all. So the lag
 * capability is degraded with a reason rather than filled in - the only way to
 * produce a number here would be to pull the backlog and count it, which would
 * deliver every message counted.
 */

// Attribute keys this driver puts on a subscription.
//
// A contract between this package and
// frontend/src/mq/googlepubsub/subscriptions.ts, not part of the shared
// vocabulary.
const (
	SubAttrPath            = "path"
	SubAttrTopic           = "topic"
	SubAttrAckDeadlineSec  = "ackDeadlineSec"
	SubAttrRetentionSec    = "retentionSec"
	SubAttrRetainAcked     = "retainAcked"
	SubAttrExpirationSec   = "expirationTtlSec"
	SubAttrFilter          = "filter"
	SubAttrOrdering        = "messageOrdering"
	SubAttrExactlyOnce     = "exactlyOnce"
	SubAttrDetached        = "detached"
	SubAttrState           = "state"
	SubAttrDelivery        = "delivery"
	SubAttrPushEndpoint    = "pushEndpoint"
	SubAttrDeadLetterTopic = "deadLetterTopic"
	SubAttrMaxAttempts     = "maxDeliveryAttempts"
	SubAttrRetryMinSec     = "retryMinBackoffSec"
	SubAttrRetryMaxSec     = "retryMaxBackoffSec"
	// SubAttrLabelPrefix carries one label per attribute, the way a topic's do.
	SubAttrLabelPrefix = "label."
)

// How a subscription is delivered, which is four different objects wearing one
// name. Only a pull subscription is one this app can read messages from.
const (
	DeliveryPull         = "pull"
	DeliveryPush         = "push"
	DeliveryBigQuery     = "bigquery"
	DeliveryCloudStorage = "cloudStorage"
	DeliveryBigtable     = "bigtable"
)

// Bounds the service enforces, checked here so the message can name the field
// the form draws rather than the proto field the API names.
const (
	minAckDeadline      = 10 * time.Second
	maxAckDeadline      = 600 * time.Second
	minSubRetention     = 10 * time.Minute
	maxSubRetention     = 31 * 24 * time.Hour
	minDeliveryAttempts = 5
	maxDeliveryAttempts = 100
)

// ListSubscriptions enumerates the project's subscriptions.
//
// One call, unlike the topics board: everything a subscription reports is on
// the Subscription itself. What is not on it is the backlog, which is why the
// lag capability is degraded rather than answered here.
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	listing := c.client.SubscriptionAdminClient.ListSubscriptions(ctx,
		&pubsubpb.ListSubscriptionsRequest{Project: c.projectPath()})

	var found []*pubsubpb.Subscription
	for {
		subscription, err := listing.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if c.matchesPrefix(shortName(subscription.GetName())) {
			found = append(found, subscription)
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].GetName() < found[b].GetName() })

	subscriptions := make([]*model.Subscription, 0, len(found))
	for _, subscription := range found {
		subscriptions = append(subscriptions, subscriptionOf(subscription))
	}
	return subscriptions, nil
}

// SubscriptionDetail reads one subscription.
func (c *Conn) SubscriptionDetail(ctx context.Context, ref model.SubscriptionRef) (*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	name, err := requiredName("subscription", ref.Name)
	if err != nil {
		return nil, err
	}
	subscription, err := c.client.SubscriptionAdminClient.GetSubscription(ctx,
		&pubsubpb.GetSubscriptionRequest{Subscription: c.subscriptionPath(name)})
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no subscription named %q in %s", name, c.config.project)
		}
		return nil, err
	}
	return subscriptionOf(subscription), nil
}

/*
 * CreateSubscription declares a subscription on a topic.
 *
 * The topic is required and cannot change afterwards: a subscription reads
 * exactly one topic, chosen when it is made. Two of the settings are also
 * fixed at creation and are the reason this is not simply an update with a
 * name - message ordering and the filter can be set here and nowhere else,
 * and a filter is the only way to have a subscription receive part of a topic.
 */
func (c *Conn) CreateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("subscription", spec.Ref.Name)
	if err != nil {
		return err
	}
	topic, err := requiredName("topic", spec.Attributes[SubAttrTopic])
	if err != nil {
		return errors.New("a subscription reads exactly one topic, and this one names none")
	}

	subscription := &pubsubpb.Subscription{
		Name:                  c.subscriptionPath(name),
		Topic:                 c.topicPath(topic),
		Labels:                labelsFrom(spec.Attributes, SubAttrLabelPrefix),
		Filter:                strings.TrimSpace(spec.Attributes[SubAttrFilter]),
		EnableMessageOrdering: spec.Attributes[SubAttrOrdering] == "true",
	}
	if err := c.applySettings(ctx, subscription, spec.Attributes, nil); err != nil {
		return err
	}
	if endpoint := strings.TrimSpace(spec.Attributes[SubAttrPushEndpoint]); endpoint != "" {
		subscription.PushConfig = &pubsubpb.PushConfig{PushEndpoint: endpoint}
	}

	_, err = c.client.SubscriptionAdminClient.CreateSubscription(ctx, subscription)
	switch {
	case alreadyExists(err):
		return fmt.Errorf("a subscription named %q already exists in %s", name, c.config.project)
	case notFound(err):
		// The service says "Subscription topic does not exist", which names
		// the subscription and not the topic that is actually missing.
		return fmt.Errorf("no topic named %q in %s to subscribe to", topic, c.config.project)
	}
	return err
}

/*
 * UpdateSubscription changes what can be changed.
 *
 * The mask is built from what the spec carries, so an omitted setting keeps
 * its value rather than being reset. Four things are refused rather than sent:
 * the topic, the filter, message ordering and the name are all fixed at
 * creation, and the service's own refusals name proto fields the form does not
 * draw.
 */
func (c *Conn) UpdateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("subscription", spec.Ref.Name)
	if err != nil {
		return err
	}

	subscription := &pubsubpb.Subscription{Name: c.subscriptionPath(name)}
	var paths []string
	if err := c.applySettings(ctx, subscription, spec.Attributes, &paths); err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("nothing to change")
	}

	_, err = c.client.SubscriptionAdminClient.UpdateSubscription(ctx,
		&pubsubpb.UpdateSubscriptionRequest{
			Subscription: subscription,
			UpdateMask:   &fieldmaskpb.FieldMask{Paths: paths},
		})
	if notFound(err) {
		return fmt.Errorf("no subscription named %q in %s", name, c.config.project)
	}
	return err
}

// RemoveSubscription deletes a subscription and everything it was holding.
//
// There is no undo. The messages it had not acknowledged are gone with it -
// they were never the topic's to hand out again, which is the whole point of
// the split.
func (c *Conn) RemoveSubscription(ctx context.Context, ref model.SubscriptionRef) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("subscription", ref.Name)
	if err != nil {
		return err
	}
	err = c.client.SubscriptionAdminClient.DeleteSubscription(ctx,
		&pubsubpb.DeleteSubscriptionRequest{Subscription: c.subscriptionPath(name)})
	if notFound(err) {
		return fmt.Errorf("no subscription named %q in %s", name, c.config.project)
	}
	return err
}

/*
 * applySettings fills in the fields a create and an update share.
 *
 * paths is nil on a create - everything set is simply sent - and non-nil on an
 * update, where the field mask decides what is written and an untouched
 * setting has to stay out of it.
 *
 * The dead-letter topic is the one that reaches outside the spec: it names
 * another topic, and Pub/Sub wants that topic's full resource path.
 */
func (c *Conn) applySettings(
	_ context.Context,
	subscription *pubsubpb.Subscription,
	attributes map[string]string,
	paths *[]string,
) error {
	mark := func(path string) {
		if paths != nil {
			*paths = append(*paths, path)
		}
	}

	if raw := strings.TrimSpace(attributes[SubAttrAckDeadlineSec]); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the ack deadline has to be a number of seconds, not %q", raw)
		}
		deadline := time.Duration(seconds) * time.Second
		if deadline < minAckDeadline || deadline > maxAckDeadline {
			return fmt.Errorf("pub/sub holds a message for between %v and %v; %v is outside that",
				minAckDeadline, maxAckDeadline, deadline)
		}
		subscription.AckDeadlineSeconds = int32(seconds)
		mark("ack_deadline_seconds")
	}

	if raw := strings.TrimSpace(attributes[SubAttrRetentionSec]); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the retention has to be a number of seconds, not %q", raw)
		}
		retention := time.Duration(seconds) * time.Second
		if retention < minSubRetention || retention > maxSubRetention {
			return fmt.Errorf(
				"pub/sub keeps a subscription's messages for between %v and %v; %v is outside that",
				minSubRetention, maxSubRetention, retention)
		}
		subscription.MessageRetentionDuration = durationpb.New(retention)
		mark("message_retention_duration")
	}

	if raw := strings.TrimSpace(attributes[SubAttrRetainAcked]); raw != "" {
		subscription.RetainAckedMessages = raw == "true"
		mark("retain_acked_messages")
	}

	if raw := strings.TrimSpace(attributes[SubAttrExactlyOnce]); raw != "" {
		subscription.EnableExactlyOnceDelivery = raw == "true"
		mark("enable_exactly_once_delivery")
	}

	if policy, given, err := retryPolicyOf(attributes); err != nil {
		return err
	} else if given {
		subscription.RetryPolicy = policy
		mark("retry_policy")
	}

	// The dead-letter policy is written whenever the spec mentions the target
	// at all, because an empty target is how it is removed: the mask names the
	// field, so sending it with nothing inside clears the policy.
	if target, given := attributes[SubAttrDeadLetterTopic]; given {
		policy, err := c.deadLetterPolicyOf(strings.TrimSpace(target), attributes)
		if err != nil {
			return err
		}
		subscription.DeadLetterPolicy = policy
		mark("dead_letter_policy")
	}
	return nil
}

// retryPolicyOf reads the backoff pair off a spec. Both bounds or neither: a
// policy with one of them is one the service rejects.
func retryPolicyOf(attributes map[string]string) (*pubsubpb.RetryPolicy, bool, error) {
	minimum := strings.TrimSpace(attributes[SubAttrRetryMinSec])
	maximum := strings.TrimSpace(attributes[SubAttrRetryMaxSec])
	if minimum == "" && maximum == "" {
		return nil, false, nil
	}
	if minimum == "" || maximum == "" {
		return nil, false, errors.New(
			"a retry policy needs both a minimum and a maximum backoff")
	}
	low, err := strconv.Atoi(minimum)
	if err != nil {
		return nil, false, fmt.Errorf("the minimum backoff has to be a number of seconds, not %q", minimum)
	}
	high, err := strconv.Atoi(maximum)
	if err != nil {
		return nil, false, fmt.Errorf("the maximum backoff has to be a number of seconds, not %q", maximum)
	}
	if low > high {
		return nil, false, fmt.Errorf(
			"the minimum backoff (%ds) is longer than the maximum (%ds)", low, high)
	}
	return &pubsubpb.RetryPolicy{
		MinimumBackoff: durationpb.New(time.Duration(low) * time.Second),
		MaximumBackoff: durationpb.New(time.Duration(high) * time.Second),
	}, true, nil
}

// deadLetterPolicyOf turns a target topic name into the policy the API takes.
//
// An empty target returns nil, which with the field in the update mask is how
// a policy is removed - there is no separate call for that.
func (c *Conn) deadLetterPolicyOf(target string, attributes map[string]string) (*pubsubpb.DeadLetterPolicy, error) {
	if target == "" {
		return nil, nil
	}
	name, err := requiredName("dead-letter topic", target)
	if err != nil {
		return nil, err
	}
	attempts := 5
	if raw := strings.TrimSpace(attributes[SubAttrMaxAttempts]); raw != "" {
		attempts, err = strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf(
				"the delivery limit before a message is dead-lettered has to be a number, not %q", raw)
		}
	}
	if attempts < minDeliveryAttempts || attempts > maxDeliveryAttempts {
		return nil, fmt.Errorf(
			"pub/sub dead-letters after between %d and %d delivery attempts; %d is outside that",
			minDeliveryAttempts, maxDeliveryAttempts, attempts)
	}
	return &pubsubpb.DeadLetterPolicy{
		DeadLetterTopic:     c.topicPath(name),
		MaxDeliveryAttempts: int32(attempts),
	}, nil
}

/*
 * subscriptionOf turns one subscription into the canonical shape.
 *
 * Backlog is unknown rather than zero, always, and that is the whole of what
 * CapSubscriptionLag being degraded means: the number exists in Cloud
 * Monitoring and nowhere in this API. Zero would say "this subscription is
 * caught up", which is the one thing the page is opened to find out.
 *
 * Members is unknown for a related reason: Pub/Sub keeps no registry of who is
 * pulling. A subscription is read by whoever calls Pull or holds a streaming
 * pull open, and the service reports neither.
 */
func subscriptionOf(subscription *pubsubpb.Subscription) *model.Subscription {
	topic := subscription.GetTopic()
	orphaned := topic == deletedTopic

	attributes := map[string]string{
		SubAttrPath: subscription.GetName(),
		// The literal _deleted-topic_ is kept as-is rather than shortened: it
		// is the service's own marker and not a resource path, and a page
		// showing "_deleted-topic_" is showing the truth.
		SubAttrTopic:          topicNameOf(topic),
		SubAttrAckDeadlineSec: strconv.FormatInt(int64(subscription.GetAckDeadlineSeconds()), 10),
		SubAttrRetainAcked:    strconv.FormatBool(subscription.GetRetainAckedMessages()),
		SubAttrOrdering:       strconv.FormatBool(subscription.GetEnableMessageOrdering()),
		SubAttrExactlyOnce:    strconv.FormatBool(subscription.GetEnableExactlyOnceDelivery()),
		SubAttrDetached:       strconv.FormatBool(subscription.GetDetached()),
		SubAttrState:          subscription.GetState().String(),
		SubAttrDelivery:       deliveryOf(subscription),
	}
	if retention := subscription.GetMessageRetentionDuration(); retention != nil {
		attributes[SubAttrRetentionSec] = seconds(retention)
	}
	if expiry := subscription.GetExpirationPolicy(); expiry != nil && expiry.GetTtl() != nil {
		attributes[SubAttrExpirationSec] = seconds(expiry.GetTtl())
	}
	if filter := subscription.GetFilter(); filter != "" {
		attributes[SubAttrFilter] = filter
	}
	if push := subscription.GetPushConfig(); push != nil && push.GetPushEndpoint() != "" {
		attributes[SubAttrPushEndpoint] = push.GetPushEndpoint()
	}
	if dead := subscription.GetDeadLetterPolicy(); dead != nil {
		attributes[SubAttrDeadLetterTopic] = shortName(dead.GetDeadLetterTopic())
		attributes[SubAttrMaxAttempts] = strconv.FormatInt(int64(dead.GetMaxDeliveryAttempts()), 10)
	}
	if retry := subscription.GetRetryPolicy(); retry != nil {
		attributes[SubAttrRetryMinSec] = seconds(retry.GetMinimumBackoff())
		attributes[SubAttrRetryMaxSec] = seconds(retry.GetMaximumBackoff())
	}
	for key, value := range subscription.GetLabels() {
		attributes[SubAttrLabelPrefix+key] = value
	}

	destinations := 1
	if orphaned {
		destinations = 0
	}
	return &model.Subscription{
		Ref:    model.SubscriptionRef{Name: shortName(subscription.GetName())},
		Status: statusOf(subscription, orphaned),
		// Nothing registers as a consumer. A subscription is read by whoever
		// calls Pull, and the service keeps no record of who that was - zero
		// would read as "nothing is consuming this", which it cannot support.
		Members:      model.UnknownMetric,
		Destinations: destinations,
		// Cloud Monitoring's num_undelivered_messages, which is not a field on
		// this object and not reachable through this credential.
		Backlog: model.UnknownMetric,
		RateOut: model.UnknownMetric,

		Attributes: attributes,
	}
}

// statusOf is the health a subscription reports about itself.
//
// Offline covers the two states in which nothing will ever arrive again: a
// topic that has been deleted, and a subscription deliberately detached.
// Neither is an error and neither will recover on its own.
func statusOf(subscription *pubsubpb.Subscription, orphaned bool) model.SubscriptionStatus {
	switch {
	case orphaned, subscription.GetDetached():
		return model.SubscriptionOffline
	case subscription.GetState() == pubsubpb.Subscription_RESOURCE_ERROR:
		return model.SubscriptionWarning
	default:
		return model.SubscriptionOnline
	}
}

// deliveryOf is which of the four kinds of subscription this is.
//
// They wear one name and behave nothing alike: only a pull subscription is one
// this app can read a message from, and the others write straight into a push
// endpoint, BigQuery, Cloud Storage or Bigtable without a consumer at all.
func deliveryOf(subscription *pubsubpb.Subscription) string {
	switch {
	case subscription.GetBigqueryConfig() != nil:
		return DeliveryBigQuery
	case subscription.GetCloudStorageConfig() != nil:
		return DeliveryCloudStorage
	case subscription.GetBigtableConfig() != nil:
		return DeliveryBigtable
	case subscription.GetPushConfig() != nil && subscription.GetPushConfig().GetPushEndpoint() != "":
		return DeliveryPush
	default:
		return DeliveryPull
	}
}

// topicNameOf shortens a subscription's topic, leaving the service's own
// deleted marker intact: it is not a resource path and shortening it would
// produce a plausible-looking topic name nothing could be looked up by.
func topicNameOf(topic string) string {
	if topic == deletedTopic {
		return deletedTopic
	}
	return shortName(topic)
}

func seconds(duration *durationpb.Duration) string {
	return strconv.FormatInt(int64(duration.AsDuration().Seconds()), 10)
}

// labelsFrom collects the label attributes off a spec, under the prefix the
// object uses. Nil when there are none, because nil and empty mean different
// things to an update.
func labelsFrom(attributes map[string]string, prefix string) map[string]string {
	var labels map[string]string
	for key, value := range attributes {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if labels == nil {
			labels = map[string]string{}
		}
		labels[strings.TrimPrefix(key, prefix)] = value
	}
	return labels
}
