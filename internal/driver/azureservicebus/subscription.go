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

/*
 * Subscriptions, which are where a topic's messages actually end up.
 *
 * A Service Bus subscription is a real object - created, listed and deleted on
 * its own - and it is also where every message goes: a send to a topic is
 * copied into each subscription whose rules let it through, so a subscription
 * has a backlog and its topic never does. That is the opposite of the queue on
 * the same board, and the reason both pages exist.
 *
 * Two things separate it from the Pub/Sub subscription the previous family
 * had. It does not outlive its topic: deleting the topic deletes it and
 * everything it was holding. And what reaches it is decided by rules that are
 * objects of their own rather than a filter field, which is why they have a
 * page rather than a row on this one.
 *
 * A subscription is addressed as (topic, name), so the ref's Namespace carries
 * the topic - the same shape Pulsar, NATS and Redis Stream use for a
 * subscription that belongs to something.
 */

// Attribute keys this driver puts on a subscription.
//
// A contract between this package and
// frontend/src/mq/azureservicebus/subscriptions.ts, not part of the shared
// vocabulary.
const (
	SubAttrTopic                = "topic"
	SubAttrStatus               = "status"
	SubAttrLockDurationSec      = "lockDurationSec"
	SubAttrMaxDeliveryCount     = "maxDeliveryCount"
	SubAttrTTLSec               = "ttlSec"
	SubAttrAutoDeleteOnIdleSec  = "autoDeleteOnIdleSec"
	SubAttrRequiresSession      = "requiresSession"
	SubAttrDeadLetterOnExpiry   = "deadLetterOnExpiry"
	SubAttrDeadLetterOnRuleFail = "deadLetterOnRuleError"
	SubAttrForwardTo            = "forwardTo"
	SubAttrForwardDeadLettersTo = "forwardDeadLettersTo"
	SubAttrDeadLetterCount      = "deadLetterCount"
	SubAttrRuleNames            = "ruleNames"
)

// ruleFanOut caps how many rule listings run at once, for the same reason the
// subscription fan-out is bounded: one request per row at a management API
// that throttles.
const ruleFanOut = 8

/*
 * ListSubscriptions enumerates every subscription in the namespace.
 *
 * Across every topic, because the board is one board: the management API lists
 * a topic's subscriptions and has no call that lists them all, so this walks
 * the topics first. Both listings are narrowed by the connection's prefix -
 * which for a subscription means its own name rather than its topic's, so a
 * prefix that keeps a topic keeps whatever is on it.
 *
 * The rule names come with each row rather than being left to the routing
 * page, because a subscription with no rule at all receives nothing and there
 * is no other figure on this board that says so.
 */
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	topics, err := c.listTopics(ctx)
	if err != nil {
		return nil, err
	}

	found := make([][]admin.SubscriptionPropertiesItem, len(topics))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, topic := range topics {
		group.Go(func() error {
			listed, err := c.listSubscriptions(groupCtx, topic.TopicName)
			if err != nil {
				// A topic deleted since the listing takes its subscriptions
				// with it, which is a row that is simply gone rather than an
				// error the board should show.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", topic.TopicName, err)
			}
			found[index] = listed
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	var flat []admin.SubscriptionPropertiesItem
	for _, listed := range found {
		flat = append(flat, listed...)
	}

	subscriptions := make([]*model.Subscription, len(flat))
	rules := make([][]string, len(flat))
	group, groupCtx = errgroup.WithContext(ctx)
	group.SetLimit(ruleFanOut)
	for index, item := range flat {
		group.Go(func() error {
			names, err := c.ruleNames(groupCtx, item.TopicName, item.SubscriptionName)
			if err != nil {
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s/%s: %w", item.TopicName, item.SubscriptionName, err)
			}
			rules[index] = names
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	for index, item := range flat {
		subscriptions[index] = c.subscriptionOf(ctx, item.TopicName, item.SubscriptionName,
			item.SubscriptionProperties, rules[index])
	}

	sort.Slice(subscriptions, func(a, b int) bool {
		if subscriptions[a].Ref.Namespace != subscriptions[b].Ref.Namespace {
			return subscriptions[a].Ref.Namespace < subscriptions[b].Ref.Namespace
		}
		return subscriptions[a].Ref.Name < subscriptions[b].Ref.Name
	})
	return subscriptions, nil
}

// SubscriptionDetail reads one subscription.
func (c *Conn) SubscriptionDetail(ctx context.Context, ref model.SubscriptionRef) (*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	topic, name, err := subscriptionParts(ref)
	if err != nil {
		return nil, err
	}

	// Nil with a nil error is the admin client's way of saying it is not
	// there, so the miss is a nil check rather than an error one.
	found, err := c.management.GetSubscription(ctx, topic, name, nil)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("no subscription named %q on %s in %s", name, topic, c.config.namespace)
	}
	rules, err := c.ruleNames(ctx, topic, name)
	if err != nil {
		return nil, err
	}
	return c.subscriptionOf(ctx, topic, name, found.SubscriptionProperties, rules), nil
}

/*
 * CreateSubscription declares a subscription on a topic.
 *
 * The topic is required and cannot change afterwards: a subscription reads
 * exactly one topic, chosen when it is made. Sessions are fixed at creation
 * too, which is why they are only sent here.
 *
 * What is not set here is what reaches it. Every new subscription gets a
 * $Default rule matching everything, and narrowing that is the routing page's
 * job - a rule is an object rather than a field, so creating one alongside the
 * subscription would be a second write hidden inside the first.
 */
func (c *Conn) CreateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	topic, name, err := subscriptionParts(spec.Ref)
	if err != nil {
		return err
	}

	properties := &admin.SubscriptionProperties{}
	if err := applySubscriptionSettings(properties, spec.Attributes, true); err != nil {
		return err
	}

	_, err = c.management.CreateSubscription(ctx, topic, name,
		&admin.CreateSubscriptionOptions{Properties: properties})
	switch {
	case alreadyExists(err):
		return fmt.Errorf("a subscription named %q already exists on %s", name, topic)
	case notFound(err):
		// The service's own message names the subscription rather than the
		// topic that is actually missing.
		return fmt.Errorf("no topic named %q in %s to subscribe to", topic, c.config.namespace)
	}
	return err
}

/*
 * UpdateSubscription changes what can be changed.
 *
 * The whole description is written back, the way an entity's is, so the
 * current one is read first and the spec's values laid over it - an attribute
 * the spec leaves out keeps its stored value. Sessions and the topic are fixed
 * at creation and are not among them.
 */
func (c *Conn) UpdateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	topic, name, err := subscriptionParts(spec.Ref)
	if err != nil {
		return err
	}

	found, err := c.management.GetSubscription(ctx, topic, name, nil)
	if err != nil {
		return err
	}
	if found == nil {
		return fmt.Errorf("no subscription named %q on %s in %s", name, topic, c.config.namespace)
	}
	if err := applySubscriptionSettings(&found.SubscriptionProperties, spec.Attributes, false); err != nil {
		return err
	}
	_, err = c.management.UpdateSubscription(ctx, topic, name, found.SubscriptionProperties, nil)
	return err
}

// RemoveSubscription deletes a subscription and everything it was holding.
//
// Its dead letters go with it: $DeadLetterQueue is a sub-entity of the
// subscription rather than a queue of its own. Nothing is handed back to the
// topic - a copy that reached this subscription was never the topic's again.
func (c *Conn) RemoveSubscription(ctx context.Context, ref model.SubscriptionRef) error {
	if err := c.live(); err != nil {
		return err
	}
	topic, name, err := subscriptionParts(ref)
	if err != nil {
		return err
	}
	_, err = c.management.DeleteSubscription(ctx, topic, name, nil)
	if notFound(err) {
		return fmt.Errorf("no subscription named %q on %s in %s", name, topic, c.config.namespace)
	}
	return err
}

// listSubscriptions pages through one topic's subscriptions, honouring the
// prefix on the subscription's own name.
func (c *Conn) listSubscriptions(ctx context.Context, topic string) ([]admin.SubscriptionPropertiesItem, error) {
	pager := c.management.NewListSubscriptionsPager(topic, nil)
	var found []admin.SubscriptionPropertiesItem
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		found = append(found, page.Subscriptions...)
	}
	return found, nil
}

/*
 * subscriptionOf turns one subscription into the canonical shape.
 *
 * Members is unknown rather than zero, and that is the family: a subscription
 * is read by whoever opens a receiver on it, and Service Bus keeps no register
 * of who that was. Zero would read as "nothing is consuming this", which is
 * the one thing the column cannot support.
 *
 * Backlog is the active message count, which is a real figure on a real
 * namespace and unavailable against the emulator - so it is unknown there, and
 * conn.go degrades the capability with a reason rather than printing a zero.
 */
func (c *Conn) subscriptionOf(
	ctx context.Context, topic, name string, properties admin.SubscriptionProperties, rules []string,
) *model.Subscription {
	attributes := map[string]string{
		SubAttrTopic:                topic,
		SubAttrRequiresSession:      boolOf(properties.RequiresSession),
		SubAttrDeadLetterOnExpiry:   boolOf(properties.DeadLetteringOnMessageExpiration),
		SubAttrDeadLetterOnRuleFail: boolOf(properties.EnableDeadLetteringOnFilterEvaluationExceptions),
	}
	if properties.Status != nil {
		attributes[SubAttrStatus] = string(*properties.Status)
	}
	if properties.MaxDeliveryCount != nil {
		attributes[SubAttrMaxDeliveryCount] = strconv.FormatInt(int64(*properties.MaxDeliveryCount), 10)
	}
	putDuration(attributes, SubAttrLockDurationSec, properties.LockDuration)
	putDuration(attributes, SubAttrTTLSec, properties.DefaultMessageTimeToLive)
	putDuration(attributes, SubAttrAutoDeleteOnIdleSec, properties.AutoDeleteOnIdle)
	putString(attributes, SubAttrForwardTo, properties.ForwardTo)
	putString(attributes, SubAttrForwardDeadLettersTo, properties.ForwardDeadLetteredMessagesTo)
	if len(rules) > 0 {
		attributes[SubAttrRuleNames] = strings.Join(rules, ",")
	}

	subscription := &model.Subscription{
		Ref:    model.SubscriptionRef{Namespace: topic, Name: name},
		Status: subscriptionStatus(properties, rules),
		// Nothing registers as a consumer.
		Members: model.UnknownMetric,
		// Exactly one topic, chosen at creation and unchangeable.
		Destinations: 1,
		Backlog:      model.UnknownMetric,
		// No rates in the management API. Service Bus publishes them to Azure
		// Monitor, which is a different API under a different credential.
		RateOut:    model.UnknownMetric,
		Attributes: attributes,
	}

	if held, known := c.subscriptionCounts(ctx, topic, name); known {
		subscription.Backlog = held.active
		attributes[SubAttrDeadLetterCount] = strconv.FormatInt(held.deadLetter, 10)
	}
	return subscription
}

/*
 * subscriptionStatus is the health a subscription reports about itself.
 *
 * Offline covers the two states in which nothing will arrive: a subscription
 * whose status disables receiving, and one with no rules at all. The second is
 * the quiet one - a subscription is created with a $Default rule matching
 * everything, and deleting that without adding another leaves an object that
 * looks healthy on every figure and can never receive a message again.
 */
func subscriptionStatus(properties admin.SubscriptionProperties, rules []string) model.SubscriptionStatus {
	if len(rules) == 0 {
		return model.SubscriptionOffline
	}
	if properties.Status == nil {
		return model.SubscriptionOnline
	}
	switch *properties.Status {
	case admin.EntityStatusActive:
		return model.SubscriptionOnline
	case admin.EntityStatusDisabled, admin.EntityStatusReceiveDisabled:
		return model.SubscriptionOffline
	default:
		return model.SubscriptionWarning
	}
}

// subscriptionParts splits a ref into the topic and the subscription.
//
// Both are required and the message says which is missing: a subscription is
// addressed as (topic, name) and a ref carrying only a name would address
// nothing at all.
func subscriptionParts(ref model.SubscriptionRef) (string, string, error) {
	topic, err := requiredName("topic", ref.Namespace)
	if err != nil {
		return "", "", fmt.Errorf(
			"a subscription belongs to exactly one topic, and this one names none")
	}
	name, err := requiredName("subscription", ref.Name)
	if err != nil {
		return "", "", err
	}
	return topic, name, nil
}

// applySubscriptionSettings writes the spec's values onto a description.
//
// creating decides the one setting the service refuses to change: sessions are
// fixed once the subscription exists, so it is sent on a create and silently
// left out of an update rather than producing a failure the form cannot act on.
func applySubscriptionSettings(
	properties *admin.SubscriptionProperties, attributes map[string]string, creating bool,
) error {
	if seconds, given, err := boundedSeconds(attributes, SubAttrLockDurationSec,
		"lock duration", minLockDurationSec, maxLockDurationSec); err != nil {
		return err
	} else if given {
		properties.LockDuration = pointer(isoDuration(seconds))
	}
	if seconds, given, err := positiveSeconds(attributes, SubAttrTTLSec, "time to live"); err != nil {
		return err
	} else if given {
		properties.DefaultMessageTimeToLive = pointer(isoDuration(seconds))
	}
	if seconds, given, err := positiveSeconds(attributes, SubAttrAutoDeleteOnIdleSec,
		"auto-delete on idle"); err != nil {
		return err
	} else if given {
		properties.AutoDeleteOnIdle = pointer(isoDuration(seconds))
	}

	if raw := strings.TrimSpace(attributes[SubAttrMaxDeliveryCount]); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the delivery limit has to be a number, not %q", raw)
		}
		if count < minDeliveryCount || count > maxDeliveryCount {
			return fmt.Errorf(
				"service bus dead-letters after between %d and %d delivery attempts; %d is outside that",
				minDeliveryCount, maxDeliveryCount, count)
		}
		properties.MaxDeliveryCount = pointer(int32(count))
	}

	if raw := strings.TrimSpace(attributes[SubAttrDeadLetterOnExpiry]); raw != "" {
		properties.DeadLetteringOnMessageExpiration = pointer(raw == "true")
	}
	if raw := strings.TrimSpace(attributes[SubAttrDeadLetterOnRuleFail]); raw != "" {
		properties.EnableDeadLetteringOnFilterEvaluationExceptions = pointer(raw == "true")
	}
	if value, given := attributes[SubAttrForwardTo]; given {
		properties.ForwardTo = pointer(strings.TrimSpace(value))
	}
	if value, given := attributes[SubAttrForwardDeadLettersTo]; given {
		properties.ForwardDeadLetteredMessagesTo = pointer(strings.TrimSpace(value))
	}

	if creating {
		if raw := strings.TrimSpace(attributes[SubAttrRequiresSession]); raw != "" {
			properties.RequiresSession = pointer(raw == "true")
		}
	}
	return nil
}

/*
 * ruleNames is what decides which of a topic's messages reach one
 * subscription.
 *
 * Read here rather than only on the routing page because the count is a fact
 * about the subscription: a new one is created with a $Default rule matching
 * everything, and a subscription whose rules have all been deleted receives
 * nothing while every other figure on the board still looks healthy.
 */
func (c *Conn) ruleNames(ctx context.Context, topic, subscription string) ([]string, error) {
	pager := c.management.NewListRulesPager(topic, subscription, nil)
	names := make([]string, 0, 2)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, rule := range page.Rules {
			names = append(names, rule.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}
