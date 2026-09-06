package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"

	"github.com/amigoer/mq-studio/internal/model"
)

// Message property keys this driver fills in. A contract between this package
// and frontend/src/mq/googlepubsub/messages.ts.
const (
	PropDeliveryAttempt = "deliveryAttempt"
	PropOrderingKey     = "orderingKey"
	PropSubscription    = "subscription"
	// PropAttributePrefix carries one publisher-set attribute each. The keys
	// are the publisher's own, so they are prefixed to keep them apart from
	// the fields above.
	PropAttributePrefix = "attr."
)

// pullBatch is how many messages one Pull asks for. A hundred rather than the
// service's own thousand: every message returned is held away from real
// consumers until the release below, and a smaller batch shortens that.
const pullBatch = 100

// pullWindow bounds one Pull call.
//
// Not an optimisation, a necessity. Pub/Sub holds an unsatisfied Pull open
// until it has something to hand over, and the emulator holds it until just
// short of the caller's deadline - so without a window of its own, browsing an
// empty subscription would burn the whole request timeout doing nothing.
const pullWindow = 2 * time.Second

// maxBrowse caps one browse however many were asked for.
//
// The cap is this driver's rather than the service's: every message returned
// is a delivery that counts against the dead-letter policy, so a browse of a
// hundred thousand would be an outage dressed as a page.
const maxBrowse = 500

// ackBatch is how many ack ids one ModifyAckDeadline carries when the browse
// hands its messages back.
const ackBatch = 100

/*
 * QueryMessages reads what a subscription is holding, and hands it straight
 * back.
 *
 * Two things about it are worth reading twice.
 *
 * The first is what it reads. Every other family browses a destination; here
 * the topic holds nothing, so this browses a *subscription* - params.Topic
 * carries a subscription name, and the messages page offers subscriptions
 * rather than topics for exactly that reason. Two subscriptions on one topic
 * hold different messages, and there is no third place with all of them.
 *
 * The second is that this is not a browse, and the capability carries a caveat
 * saying so. Pull is the only read Pub/Sub has and it is the same call a
 * consumer makes: what it returns is held away from every other reader for the
 * subscription's ack deadline, and its delivery attempt goes up - which counts
 * towards the dead-letter policy's limit. So a page of messages read here is a
 * page a consumer running at the same moment did not get, and a message read
 * enough times ends up in the dead-letter topic with nothing having failed.
 *
 * What the driver can do is shorten the window and it does: every message is
 * handed back with a deadline of zero as soon as the batch is assembled, so
 * they are available again in about as long as the request takes. It cannot
 * close the window, and it cannot undo the delivery attempt. Hence the caveat
 * rather than a silent best effort.
 *
 * Filtering is left to the caller for the same reason the read is destructive:
 * a subscription's filter is fixed at creation and there is no per-request
 * selector of any kind, so narrowing would mean pulling everything and
 * discarding most of it - which would hide far more messages, for far longer,
 * than the page showed.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	name, err := requiredName("subscription", params.Topic)
	if err != nil {
		return nil, errors.New(
			"a pub/sub browse reads a subscription rather than a topic, and this one names none")
	}
	path := c.subscriptionPath(name)

	wanted := params.MaxResults
	if wanted <= 0 {
		wanted = pullBatch
	}
	if wanted > maxBrowse {
		wanted = maxBrowse
	}

	collected, ackIDs, err := c.pullHeld(ctx, path, wanted)
	// The release runs whatever happened, including on the error path: the
	// messages already taken are held either way, and leaving them so because
	// the next call failed is the worst outcome available.
	defer c.release(context.WithoutCancel(ctx), path, ackIDs)
	if err != nil {
		return nil, err
	}

	items := make([]*model.MessageItem, 0, len(collected))
	for index, message := range collected {
		items = append(items, messageItemOf(index+1, name, message))
	}
	// Newest first, which is what every other family's browse shows. Pub/Sub
	// returns whatever its servers offered, in no order at all unless the
	// subscription was created with ordering on - so the ordering is this
	// app's and the publish time is the only thing it can honestly sort on.
	sort.SliceStable(items, func(a, b int) bool {
		return items[a].StoreTimestamp > items[b].StoreTimestamp
	})
	return items, nil
}

/*
 * MessageByID is not offered. Pub/Sub has no call that takes a message id: an
 * id is assigned on publish and echoed on delivery, and the only way to reach
 * a message is to be handed it. The capability is not declared, so nothing in
 * the UI reaches this.
 */
func (c *Conn) MessageByID(context.Context, string, string) (*model.MessageItem, error) {
	return nil, errNoMessageByID
}

var errNoMessageByID = errors.New(
	"pub/sub has no call that fetches a message by id; an id is assigned on publish and " +
		"echoed on delivery, and nothing indexes one")

// pullHeld pulls up to wanted messages, holding each one so the next call
// offers something new.
//
// The hold is the subscription's own ack deadline rather than anything set
// here - Pull takes no deadline of its own - which is what makes several calls
// add up to a page instead of returning the same batch over and over. It ends
// with the release below.
func (c *Conn) pullHeld(ctx context.Context, path string, wanted int) ([]*pubsubpb.ReceivedMessage, []string, error) {
	collected := make([]*pubsubpb.ReceivedMessage, 0, wanted)
	ackIDs := make([]string, 0, wanted)
	seen := make(map[string]bool, wanted)

	for len(collected) < wanted {
		batch := min(wanted-len(collected), pullBatch)
		pullCtx, cancel := context.WithTimeout(ctx, pullWindow)
		out, err := c.client.SubscriptionAdminClient.Pull(pullCtx, &pubsubpb.PullRequest{
			Subscription: path,
			MaxMessages:  int32(batch),
		})
		cancel()
		if err != nil {
			if notFound(err) {
				return collected, ackIDs, fmt.Errorf(
					"no subscription named %q in %s", shortName(path), c.config.project)
			}
			return collected, ackIDs, err
		}
		if len(out.GetReceivedMessages()) == 0 {
			// An exhausted pull means the subscription has nothing more to
			// offer that is not already held here. It is the only stop
			// condition there is: no count is exact enough to compare against.
			break
		}
		for _, received := range out.GetReceivedMessages() {
			ackIDs = append(ackIDs, received.GetAckId())
			// A duplicate is possible - at-least-once delivery applies to this
			// read too, and a short ack deadline can redeliver mid-browse - and
			// showing one message twice would read as a duplicate in the
			// subscription rather than in the reading of it.
			id := received.GetMessage().GetMessageId()
			if seen[id] {
				continue
			}
			seen[id] = true
			collected = append(collected, received)
		}
	}
	return collected, ackIDs, nil
}

/*
 * release hands every browsed message straight back.
 *
 * In batches, because one request carrying every ack id of a five-hundred
 * message browse is a request the service may refuse for its size, and best
 * effort throughout: an ack id that has already expired fails and the rest
 * must still be returned. What it cannot do is make the read never have
 * happened - the delivery attempt has already gone up - which is what the
 * caveat is for.
 */
func (c *Conn) release(ctx context.Context, path string, ackIDs []string) {
	for start := 0; start < len(ackIDs); start += ackBatch {
		end := min(start+ackBatch, len(ackIDs))
		_ = c.client.SubscriptionAdminClient.ModifyAckDeadline(ctx,
			&pubsubpb.ModifyAckDeadlineRequest{
				Subscription:       path,
				AckIds:             ackIDs[start:end],
				AckDeadlineSeconds: 0,
			})
	}
}

// messageItemOf turns one received message into the canonical shape.
//
// Several of the canonical fields have no counterpart and stay empty rather
// than being filled with something plausible: there is no tag, no queue id, no
// offset, and no store or born host - a message arrives over HTTPS from
// whoever authenticated the publish.
func messageItemOf(index int, subscription string, received *pubsubpb.ReceivedMessage) *model.MessageItem {
	message := received.GetMessage()
	item := &model.MessageItem{
		ID: index,
		// The subscription rather than the topic, because that is what was
		// read: the topic holds nothing and two subscriptions on it hold
		// different messages.
		Topic:       subscription,
		MessageID:   message.GetMessageId(),
		QueueID:     model.UnknownMetric,
		QueueOffset: model.UnknownMetric,
		Status:      model.MsgNormal,
		Body:        string(message.GetData()),
		Properties:  map[string]string{PropSubscription: subscription},
	}

	if published := message.GetPublishTime(); published != nil {
		item.StoreTimestamp = published.AsTime().UnixMilli()
		item.StoreTime = published.AsTime().Format(time.RFC3339)
	}
	// The delivery attempt is the field to read twice. It counts every
	// delivery, including a browse from this app, and the dead-letter policy
	// compares it against maxDeliveryAttempts - so a message browsed enough
	// times is dead-lettered with nothing having failed. It is only reported
	// at all on a subscription that has such a policy.
	if attempt := received.GetDeliveryAttempt(); attempt > 0 {
		item.Properties[PropDeliveryAttempt] = strconv.FormatInt(int64(attempt), 10)
		item.RetryTimes = int(attempt) - 1
	}
	if key := message.GetOrderingKey(); key != "" {
		item.Properties[PropOrderingKey] = key
		// What a reader groups by, which is the nearest thing RocketMQ's keys
		// field is for.
		item.Keys = key
	}
	for name, value := range message.GetAttributes() {
		item.Properties[PropAttributePrefix+name] = value
	}
	return item
}
