package ibmmq

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

/*
 * Subscriptions, which are the one thing on this queue manager that looks like
 * a consumer group and is not one.
 *
 * A subscription registers interest in a topic string and names a queue to
 * deliver to. That queue is where the messages actually sit, so everything the
 * canonical model calls a backlog belongs to it rather than to the
 * subscription - which is why this reads the queue listing as well and why the
 * two halves can disagree in a way worth showing: a subscription can have
 * received two hundred publications and be owed none, because something has
 * been draining its queue all along.
 *
 * The definitions come from MQSC rather than from the subscription resource.
 * The resource is read only and rich, but its runtime half is not there at
 * all: how many publications a subscription has received, when the last one
 * arrived and whether anything is attached come from DISPLAY SBSTATUS, and
 * joining a REST read to an MQSC read by name would be two shapes for one row.
 *
 * # What is not here
 *
 * Creating and removing one. DEFINE SUB and DELETE SUB exist and work through
 * the same endpoint, so this is this driver's omission rather than the
 * family's: a subscription's identity is a topic string plus a destination
 * queue plus a durability, and giving that a form is a page of its own rather
 * than a name field. CapSubscriptionCreate and CapSubscriptionDelete stay
 * undeclared until it has one.
 */

// Subscription attribute keys this driver writes into Attributes.
//
// A contract between this package and frontend/src/mq/ibmmq/subscriptions.ts.
const (
	SubAttrTopicString   = "topicString"
	SubAttrDestination   = "destination"
	SubAttrDestinationQM = "destinationQueueManager"
	SubAttrDurable       = "durable"
	SubAttrType          = "subscriptionType"
	SubAttrUser          = "user"
	SubAttrSelector      = "selector"
	SubAttrID            = "subscriptionId"
	SubAttrMessages      = "messagesReceived"
	SubAttrLastMessage   = "lastMessageAt"
	SubAttrAttached      = "attached"
	// SubAttrReaders is how many applications have the destination queue open
	// for input. It is not the same as an attached subscriber and is usually
	// the more useful number: an administrative subscription has nothing
	// attached by design, and something else drains its queue.
	SubAttrReaders = "queueReaders"
)

// The MQSC attributes each half asks for.
var (
	subscriptionAttributes = []string{
		"topicstr", "dest", "destqmgr", "durable", "subtype", "subuser", "selector",
		"subid", "altdate", "alttime",
	}
	subscriptionStatus = []string{
		"topicstr", "actconn", "nummsgs", "lmsgdate", "lmsgtime", "subid", "durable", "subtype",
	}
)

// noConnection is the all-zero connection identifier SBSTATUS reports when
// nothing is attached. It is a value rather than an absent field, so it has to
// be recognised rather than checked for emptiness.
const noConnection = "0"

// ListSubscriptions enumerates the queue manager's subscriptions.
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	definitions, err := c.display(ctx, "sub", "*", subscriptionAttributes...)
	if err != nil {
		return nil, err
	}
	statuses, err := c.display(ctx, "sbstatus", "*", subscriptionStatus...)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]map[string]json.RawMessage, len(statuses))
	for _, status := range statuses {
		byName[mqscString(status, "sub")] = status
	}

	// The destination queues, for the backlog. One listing rather than a read
	// per subscription: a queue manager with forty subscriptions would
	// otherwise cost forty round trips to fill one column.
	queues, err := c.listQueues(ctx, model.DestinationFilter{IncludeInternal: true}, "")
	if err != nil {
		return nil, err
	}
	depths := make(map[string]*model.Destination, len(queues))
	for _, queue := range queues {
		depths[queue.Ref.Name] = queue
	}

	subscriptions := make([]*model.Subscription, 0, len(definitions))
	for _, definition := range definitions {
		name := mqscString(definition, "sub")
		if name == "" {
			continue
		}
		subscriptions = append(subscriptions,
			subscriptionOf(name, definition, byName[name], depths))
	}

	sort.Slice(subscriptions, func(i, j int) bool {
		return subscriptions[i].Ref.Name < subscriptions[j].Ref.Name
	})
	for index, subscription := range subscriptions {
		subscription.ID = index + 1
	}
	return subscriptions, nil
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
	return nil, fmt.Errorf("%s has no subscription named %q", c.qmgr, ref.Name)
}

// errNoSubscriptionAdmin is why this driver reads subscriptions and does not
// write them. See the package comment above: the commands exist, the form does
// not, and CapSubscriptionCreate and CapSubscriptionDelete are undeclared so
// no page offers a control this returns.
var errNoSubscriptionAdmin = errors.New(
	"this driver does not create or remove subscriptions: one is a topic string, a " +
		"destination queue and a durability together, which needs a form of its own")

// CreateSubscription is not offered. See errNoSubscriptionAdmin.
func (c *Conn) CreateSubscription(_ context.Context, _ model.SubscriptionSpec) error {
	return errNoSubscriptionAdmin
}

// UpdateSubscription is not offered. See errNoSubscriptionAdmin.
func (c *Conn) UpdateSubscription(_ context.Context, _ model.SubscriptionSpec) error {
	return errNoSubscriptionAdmin
}

// RemoveSubscription is not offered. See errNoSubscriptionAdmin.
func (c *Conn) RemoveSubscription(_ context.Context, _ model.SubscriptionRef) error {
	return errNoSubscriptionAdmin
}

/*
 * subscriptionOf folds a definition, its runtime status and its destination
 * queue's depth into one row.
 *
 * The backlog is the destination queue's depth, which is the only honest
 * answer: a subscription stores nothing itself, and what it is owed is what
 * has been delivered to its queue and not read. A managed subscription - one
 * that let the queue manager pick the queue - has a destination this listing
 * may not carry, and its backlog is UnknownMetric rather than zero.
 */
func subscriptionOf(
	name string,
	definition map[string]json.RawMessage,
	status map[string]json.RawMessage,
	queues map[string]*model.Destination,
) *model.Subscription {
	destination := mqscString(definition, "dest")
	attributes := map[string]string{
		SubAttrTopicString:   mqscString(definition, "topicstr"),
		SubAttrDestination:   destination,
		SubAttrDestinationQM: mqscString(definition, "destqmgr"),
		SubAttrDurable:       strings.ToLower(mqscString(definition, "durable")),
		SubAttrType:          strings.ToLower(mqscString(definition, "subtype")),
		SubAttrUser:          mqscString(definition, "subuser"),
		SubAttrSelector:      mqscString(definition, "selector"),
		SubAttrID:            mqscString(definition, "subid"),
	}

	subscription := &model.Subscription{
		Ref: model.SubscriptionRef{Name: name},
		// One topic string, always. A subscription registers interest in one
		// place; reading several means several subscriptions.
		Destinations: 1,
		Backlog:      model.UnknownMetric,
		// No rate. The queue manager counts how many publications a
		// subscription has received since it was created, which is a total
		// rather than a rate, and nothing anywhere reports the second.
		RateOut:     model.UnknownMetric,
		Status:      model.SubscriptionOffline,
		LastUpdated: mqscTimestamp(definition),
		Attributes:  attributes,
	}

	attached := false
	if status != nil {
		attached = strings.Trim(mqscString(status, "actconn"), noConnection) != ""
		if received, found := mqscInt(status, "nummsgs"); found {
			attributes[SubAttrMessages] = strconv.FormatInt(received, 10)
		}
		joined := joinDateAndTime(mqscString(status, "lmsgdate"), mqscString(status, "lmsgtime"))
		if joined != "" {
			attributes[SubAttrLastMessage] = joined
		}
	}
	attributes[SubAttrAttached] = strconv.FormatBool(attached)
	if attached {
		// At most one: SBSTATUS reports a single connection identifier, so
		// there is no way for this to be a count of several.
		subscription.Members = 1
	}

	readers := 0
	if queue := queues[destination]; queue != nil {
		subscription.Backlog = queue.Depth
		if open := queue.Attribute(AttrOpenInput); open != "" {
			if count, err := strconv.Atoi(open); err == nil {
				readers = count
			}
		}
	}
	attributes[SubAttrReaders] = strconv.Itoa(readers)

	subscription.Status = subscriptionStatusOf(attached, readers, subscription.Backlog)
	return subscription
}

/*
 * subscriptionStatusOf decides which of the three canonical states this is.
 *
 * "Nothing attached" is not offline here, and that is the point. An
 * administrative subscription is created by an operator and has no subscriber
 * attached by design: the publications land on its destination queue and
 * whichever application reads that queue is the consumer. So what is looked at
 * is whether anything is draining the queue, and the warning is the case that
 * actually needs somebody - a subscription collecting publications that
 * nothing is reading.
 */
func subscriptionStatusOf(attached bool, readers int, backlog int64) model.SubscriptionStatus {
	switch {
	case attached || readers > 0:
		return model.SubscriptionOnline
	case backlog > 0:
		return model.SubscriptionWarning
	default:
		return model.SubscriptionOffline
	}
}
