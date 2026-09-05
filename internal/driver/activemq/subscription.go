package activemq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// Durable subscriptions, which the two products keep in unrelated places.
//
// Artemis stores one as a queue bound to a multicast address: the address is
// the topic, and each queue under it holds what one subscriber still owes.
// Classic stores one as a consumer registered against a topic, identified by
// the pair (clientId, subscriptionName) - and a subscriber that has never
// connected still exists, which is what durable means, so the broker lists
// active and inactive ones separately.
//
// The canonical ref carries one name, so Classic's pair is joined with a
// separator this file owns. Namespace holds the topic, which is the one thing
// both products agree a subscription belongs to.

// classicSubscriptionSeparator joins a Classic subscription's two identifying
// halves into the one name a canonical ref carries.
//
// A vertical bar rather than a slash or a colon: JMS client ids and
// subscription names routinely contain both, and a separator that appears
// inside a name cannot be split back out again.
const classicSubscriptionSeparator = "|"

// Subscription attribute keys, on top of the shared ones in attributes.go.
const (
	AttrClientID         = "clientId"
	AttrSubscriptionName = "subscriptionName"
	AttrTopic            = "topic"
	AttrSelector         = "selector"
	AttrActive           = "active"
	// AttrPendingAck is what has been handed to the subscriber and not
	// acknowledged, which is different from the backlog: those are still owed
	// and these are already in flight.
	AttrPendingAck = "pendingAck"
	AttrDispatched = "dispatched"
	AttrConsumed   = "consumed"
	AttrPrefetch   = "prefetchSize"
	AttrSlow       = "slowConsumer"
	AttrDurableSub = "durable"
)

var errNoSubscriptionUpdate = errors.New(
	"an activemq subscription's topic, client id and selector are fixed when it is created")

// ListSubscriptions enumerates the broker's durable subscriptions.
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if c.tiers.product == artemis {
		return c.listArtemisSubscriptions(ctx)
	}
	return c.listClassicSubscriptions(ctx)
}

// SubscriptionDetail re-reads one subscription.
func (c *Conn) SubscriptionDetail(ctx context.Context, ref model.SubscriptionRef) (*model.Subscription, error) {
	found, err := c.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	for _, subscription := range found {
		if subscription.Ref.Name == ref.Name {
			return subscription, nil
		}
	}
	return nil, fmt.Errorf("no subscription named %q", ref.Name)
}

// CreateSubscription registers a durable subscription on a topic.
//
// Both products can, which was not a given: Classic's is a JMX operation on
// the broker, and Artemis's is a queue bound to the address with a multicast
// routing type. What neither can do is create one on a queue - a JMS queue has
// no named subscribers, its consumers are connections and they come and go.
func (c *Conn) CreateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	topic := spec.Ref.Namespace
	if topic == "" {
		topic = spec.Attributes[AttrTopic]
	}
	if topic == "" {
		return errors.New("a durable subscription needs the topic it reads")
	}

	if c.tiers.product == artemis {
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"createQueue(java.lang.String,java.lang.String,java.lang.String)",
			topic, spec.Ref.Name, "MULTICAST"))
		return err
	}

	clientID, name := splitClassicSubscription(spec.Ref.Name)
	if clientID == "" || name == "" {
		return fmt.Errorf("a classic subscription is named client%ssubscription", classicSubscriptionSeparator)
	}
	// A selector must be null rather than empty: the broker parses "" as a JMS
	// selector expression and rejects it with InvalidSelectorException, which
	// reads as a broken driver rather than as an empty field.
	var selector any
	if value := spec.Attributes[AttrSelector]; value != "" {
		selector = value
	}
	_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
		"createDurableSubscriber(java.lang.String,java.lang.String,java.lang.String,java.lang.String)",
		clientID, name, topic, selector))
	return err
}

// UpdateSubscription is not offered. See errNoSubscriptionUpdate.
func (c *Conn) UpdateSubscription(_ context.Context, _ model.SubscriptionSpec) error {
	return errNoSubscriptionUpdate
}

// RemoveSubscription unsubscribes, discarding whatever it was still owed.
func (c *Conn) RemoveSubscription(ctx context.Context, ref model.SubscriptionRef) error {
	if c.tiers.product == artemis {
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"destroyQueue(java.lang.String,boolean,boolean)", ref.Name, true, true))
		return err
	}
	clientID, name := splitClassicSubscription(ref.Name)
	if clientID == "" || name == "" {
		return fmt.Errorf("a classic subscription is named client%ssubscription", classicSubscriptionSeparator)
	}
	_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
		"destroyDurableSubscriber(java.lang.String,java.lang.String)", clientID, name))
	return err
}

// classicSubscriptionAttributes is the read set for one subscriber MBean.
var classicSubscriptionAttributes = []string{
	"ClientId", "SubscriptionName", "DestinationName", "Selector", "Active",
	"PendingQueueSize", "DispatchedQueueSize", "DispatchedCounter",
	"DequeueCounter", "MessageCountAwaitingAcknowledge", "PrefetchSize",
	"SlowConsumer",
}

func (c *Conn) listClassicSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	// Active and inactive are two attributes, and both are subscriptions: an
	// inactive one is a durable subscriber whose client is not connected,
	// which is the state durability exists for. Listing only the active ones
	// would hide exactly the subscriptions somebody is looking for.
	values, err := c.jolokia.batch(ctx, []request{
		readAttribute(c.names.brokerMBean(), "DurableTopicSubscribers"),
		readAttribute(c.names.brokerMBean(), "InactiveDurableTopicSubscribers"),
	})
	if err != nil {
		return nil, err
	}

	var mbeans []string
	for _, value := range values {
		var refs []struct {
			ObjectName string `json:"objectName"`
		}
		if err := json.Unmarshal(value, &refs); err != nil {
			return nil, fmt.Errorf("the broker's subscriber list is not a set of object names: %w", err)
		}
		for _, ref := range refs {
			mbeans = append(mbeans, ref.ObjectName)
		}
	}

	requests := make([]request, 0, len(mbeans)*len(classicSubscriptionAttributes))
	for _, mbean := range mbeans {
		for _, attribute := range classicSubscriptionAttributes {
			requests = append(requests, readAttribute(mbean, attribute))
		}
	}
	read, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	subscriptions := make([]*model.Subscription, 0, len(mbeans))
	for i := range mbeans {
		set := attributeSet(classicSubscriptionAttributes, read[i*len(classicSubscriptionAttributes):])
		if subscription := c.classicSubscription(set); subscription != nil {
			subscriptions = append(subscriptions, subscription)
		}
	}
	sortSubscriptions(subscriptions)
	return subscriptions, nil
}

func (c *Conn) classicSubscription(read map[string]json.RawMessage) *model.Subscription {
	clientID := stringOr(read["ClientId"])
	name := stringOr(read["SubscriptionName"])
	if clientID == "" || name == "" {
		return nil
	}

	attributes := map[string]string{
		AttrProduct:          string(classic),
		AttrClientID:         clientID,
		AttrSubscriptionName: name,
		AttrTopic:            stringOr(read["DestinationName"]),
		AttrDurableSub:       "true",
	}
	putString(attributes, AttrSelector, read["Selector"])
	putBool(attributes, AttrActive, read["Active"])
	putInt(attributes, AttrPendingAck, read["MessageCountAwaitingAcknowledge"])
	putInt(attributes, AttrDispatched, read["DispatchedCounter"])
	putInt(attributes, AttrConsumed, read["DequeueCounter"])
	putInt(attributes, AttrPrefetch, read["PrefetchSize"])
	putBool(attributes, AttrSlow, read["SlowConsumer"])

	active := attributes[AttrActive] == "true"
	members := 0
	if active {
		members = 1
	}

	return &model.Subscription{
		Ref: model.SubscriptionRef{
			Namespace: stringOr(read["DestinationName"]),
			Name:      clientID + classicSubscriptionSeparator + name,
		},
		// Offline is the ordinary resting state of a durable subscription, not
		// a fault: the broker is keeping messages for a client that is not
		// connected, which is the whole point. The board says so rather than
		// colouring every idle subscription red.
		Status:       statusOf(active, read["SlowConsumer"]),
		Members:      members,
		Destinations: 1,
		Backlog:      int64(intOr(read["PendingQueueSize"], model.UnknownMetric)),
		RateOut:      model.UnknownMetric,
		Attributes:   attributes,
	}
}

func (c *Conn) listArtemisSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	// Every queue bound to a multicast address is a durable subscription, so
	// the list starts from the destinations and keeps the topics.
	destinations, err := c.listArtemisDestinations(ctx, model.DestinationFilter{})
	if err != nil {
		return nil, err
	}

	type binding struct{ address, queue string }
	var bindings []binding
	for _, destination := range destinations {
		if destination.Attributes[AttrKind] != string(topicKind) {
			continue
		}
		queues, err := c.artemisQueuesUnder(ctx, destination.Ref.Name)
		if err != nil {
			return nil, err
		}
		for _, queue := range queues {
			bindings = append(bindings, binding{address: destination.Ref.Name, queue: queue})
		}
	}

	requests := make([]request, 0, len(bindings)*len(artemisQueueAttributes))
	for _, b := range bindings {
		for _, attribute := range artemisQueueAttributes {
			requests = append(requests, readAttribute(
				c.names.artemisQueue(b.address, b.queue, multicast), attribute))
		}
	}
	read, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	subscriptions := make([]*model.Subscription, 0, len(bindings))
	for i, b := range bindings {
		set := attributeSet(artemisQueueAttributes, read[i*len(artemisQueueAttributes):])
		subscriptions = append(subscriptions, c.artemisSubscription(b.address, b.queue, set))
	}
	sortSubscriptions(subscriptions)
	return subscriptions, nil
}

func (c *Conn) artemisSubscription(address, queue string, read map[string]json.RawMessage) *model.Subscription {
	attributes := map[string]string{
		AttrProduct:          string(artemis),
		AttrSubscriptionName: queue,
		AttrTopic:            address,
		AttrAddress:          address,
	}
	putBool(attributes, AttrDurableSub, read["Durable"])
	putString(attributes, AttrSelector, read["Filter"])
	putInt(attributes, AttrPendingAck, read["DeliveringCount"])
	putInt(attributes, AttrDispatched, read["MessagesAdded"])
	putInt(attributes, AttrConsumed, read["MessagesAcknowledged"])
	putString(attributes, AttrDeadLetter, read["DeadLetterAddress"])
	putString(attributes, AttrExpiry, read["ExpiryAddress"])

	consumers := intOr(read["ConsumerCount"], 0)
	attributes[AttrActive] = strconv.FormatBool(consumers > 0)

	return &model.Subscription{
		Ref:          model.SubscriptionRef{Namespace: address, Name: queue},
		Status:       statusOf(consumers > 0, nil),
		Members:      consumers,
		Destinations: 1,
		Backlog:      int64(intOr(read["MessageCount"], model.UnknownMetric)),
		RateOut:      model.UnknownMetric,
		Attributes:   attributes,
	}
}

// statusOf maps a subscription's connection state onto the canonical three.
//
// Offline is the resting state of a durable subscription rather than a fault:
// the broker is holding messages for a client that is not connected, which is
// what durability is for. Warning is reserved for the one thing that is a
// problem - a consumer the broker has marked slow, meaning it is falling
// behind what is being dispatched to it.
func statusOf(active bool, slow json.RawMessage) model.SubscriptionStatus {
	if !active {
		return model.SubscriptionOffline
	}
	var isSlow bool
	if slow != nil {
		_ = json.Unmarshal(slow, &isSlow)
	}
	if isSlow {
		return model.SubscriptionWarning
	}
	return model.SubscriptionOnline
}

func splitClassicSubscription(name string) (clientID, subscription string) {
	before, after, found := strings.Cut(name, classicSubscriptionSeparator)
	if !found {
		return "", ""
	}
	return before, after
}

func sortSubscriptions(subscriptions []*model.Subscription) {
	sort.SliceStable(subscriptions, func(i, j int) bool {
		left, right := subscriptions[i], subscriptions[j]
		if left.Ref.Namespace != right.Ref.Namespace {
			return left.Ref.Namespace < right.Ref.Namespace
		}
		return left.Ref.Name < right.Ref.Name
	})
}

func stringOr(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return ""
	}
	return *value
}
