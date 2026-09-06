package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/amigoer/mq-studio/internal/model"
)

// Filter keys the browse takes, and message property keys it fills in.
//
// A contract between this package and
// frontend/src/mq/azureservicebus/messages.ts, not part of the shared
// vocabulary.
const (
	// FilterSubscription names the subscription to browse, when the entity is
	// a topic. Absent, the entity is a queue and is browsed directly.
	FilterSubscription = "subscription"
	// FilterFromSequence starts the browse at a sequence number rather than at
	// the beginning, which is how the page walks a large entity.
	FilterFromSequence = "fromSequence"
	// FilterDeadLetters browses the entity's $DeadLetterQueue instead of the
	// entity itself.
	FilterDeadLetters = "deadLetters"
)

const (
	PropState                = "state"
	PropSequenceNumber       = "sequenceNumber"
	PropEnqueuedSequence     = "enqueuedSequenceNumber"
	PropDeliveryCount        = "deliveryCount"
	PropContentType          = "contentType"
	PropCorrelationID        = "correlationId"
	PropReplyTo              = "replyTo"
	PropReplyToSessionID     = "replyToSessionId"
	PropTo                   = "to"
	PropSessionID            = "sessionId"
	PropPartitionKey         = "partitionKey"
	PropScheduledEnqueueTime = "scheduledEnqueueTime"
	PropExpiresAt            = "expiresAt"
	PropDeadLetterReason     = "deadLetterReason"
	PropDeadLetterDesc       = "deadLetterErrorDescription"
	PropDeadLetterSource     = "deadLetterSource"
	// PropAttributePrefix carries one sender-set application property each.
	// The keys are the sender's own, so they are prefixed to keep them apart
	// from the fields above.
	PropAttributePrefix = "prop."
)

// The three states a peeked message can be in. A consumer is only ever offered
// the first; the other two are why a peek shows more than a receive would.
const (
	StateActive    = "active"
	StateDeferred  = "deferred"
	StateScheduled = "scheduled"
)

// peekBatch is how many messages one PeekMessages call asks for. The service
// caps a peek at 250 whatever is requested, so asking for more is a request
// that comes back short rather than an error.
const peekBatch = 100

// maxBrowse caps one browse however many were asked for.
//
// The cap is this driver's rather than the service's, and it is about the page
// rather than the messages: a peek costs nothing and takes nothing away, so
// the only thing a browse of a hundred thousand would hurt is the renderer
// holding them.
const maxBrowse = 1000

/*
 * QueryMessages reads what an entity is holding, and takes nothing.
 *
 * This is the one page where Service Bus differs from both hosted families
 * before it, and the difference is worth reading twice. SQS's ReceiveMessage
 * hides what it read for a visibility timeout and raises its receive count;
 * Pub/Sub's Pull holds what it read away from consumers for the ack deadline
 * and raises its delivery attempt, which counts towards being dead-lettered.
 * Both had to declare a caveat saying that browsing perturbs delivery.
 *
 * PeekMessages does none of it. It takes no lock, moves nothing, changes no
 * delivery count, and a consumer running at the same moment misses nothing. So
 * CapMessageQuery here carries no caveat at all, and a test pins that absence:
 * swapping this call for ReceiveMessages would still return messages and would
 * quietly make browsing destructive.
 *
 * What a peek shows that a receive could not is the other half of it. A
 * scheduled message is held back until its enqueue time and a deferred one has
 * been set aside by sequence number - no consumer is offered either, and both
 * come back here with a state saying which.
 *
 * The one thing a peek needs from the caller is where to start. A receiver
 * keeps a cursor that advances with every peek, so a second call with no
 * sequence number returns what follows the first rather than the same page;
 * this always sends one, so a browse is the same browse twice.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	entity, err := requiredName("queue or topic", params.Topic)
	if err != nil {
		return nil, err
	}
	subscription := strings.TrimSpace(params.Filters[FilterSubscription])
	deadLetters := params.Filters[FilterDeadLetters] == "true"

	from := int64(0)
	if raw := strings.TrimSpace(params.Filters[FilterFromSequence]); raw != "" {
		from, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("the starting position is a sequence number, not %q", raw)
		}
	}

	wanted := params.MaxResults
	if wanted <= 0 {
		wanted = peekBatch
	}
	if wanted > maxBrowse {
		wanted = maxBrowse
	}

	peeked, err := c.peek(ctx, entity, subscription, deadLetters, from, wanted)
	if err != nil {
		return nil, err
	}

	label := browseLabel(entity, subscription, deadLetters)
	items := make([]*model.MessageItem, 0, len(peeked))
	for index, message := range peeked {
		items = append(items, messageItemOf(index+1, label, message, deadLetters))
	}
	// Newest first, which is what every other family's browse shows. A peek
	// arrives oldest first, in sequence order, so this is a reversal rather
	// than an ordering invented here.
	sort.SliceStable(items, func(a, b int) bool {
		return items[a].QueueOffset > items[b].QueueOffset
	})
	return items, nil
}

/*
 * MessageByID is not offered. Service Bus indexes nothing by message id: the
 * id is whatever the sender put there, it need not be unique, and no call
 * takes one. What does address a message is its sequence number, which is what
 * the browse starts from. The capability is not declared, so nothing in the UI
 * reaches this.
 */
func (c *Conn) MessageByID(context.Context, string, string) (*model.MessageItem, error) {
	return nil, errNoMessageByID
}

var errNoMessageByID = errors.New(
	"service bus indexes no message id; the id is the sender's own and need not be unique, " +
		"and what addresses a message here is its sequence number")

// peek reads up to wanted messages without taking any of them.
//
// Several calls where one would not do: the service caps a peek at 250
// whatever is asked for, so the loop continues from the last sequence number
// it saw. An empty batch is the stop condition and the only one there is - no
// count is exact enough to compare against.
func (c *Conn) peek(
	ctx context.Context, entity, subscription string, deadLetters bool, from int64, wanted int,
) ([]*azservicebus.ReceivedMessage, error) {
	receiver, err := c.receiver(entity, subscription, deadLetters)
	if err != nil {
		return nil, err
	}
	defer func() { _ = receiver.Close(context.WithoutCancel(ctx)) }()

	collected := make([]*azservicebus.ReceivedMessage, 0, wanted)
	next := from
	for len(collected) < wanted {
		batch, err := receiver.PeekMessages(ctx, min(wanted-len(collected), peekBatch),
			&azservicebus.PeekMessagesOptions{FromSequenceNumber: &next})
		if err != nil {
			if notFound(err) {
				return nil, fmt.Errorf("no %s in %s",
					browseLabel(entity, subscription, deadLetters), c.config.namespace)
			}
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		collected = append(collected, batch...)
		last := batch[len(batch)-1]
		if last.SequenceNumber == nil {
			// Nothing to continue from, so a second call would return the
			// same batch for ever.
			break
		}
		next = *last.SequenceNumber + 1
	}
	return collected, nil
}

// receiver opens a reader on a queue, a subscription, or either one's
// dead-letter sub-entity.
//
// PeekLock rather than ReceiveAndDelete, and it matters even though a peek
// settles nothing: the mode is the receiver's, so a ReceiveAndDelete receiver
// left lying around is one an accidental receive would empty the entity with.
func (c *Conn) receiver(entity, subscription string, deadLetters bool) (*azservicebus.Receiver, error) {
	options := &azservicebus.ReceiverOptions{ReceiveMode: azservicebus.ReceiveModePeekLock}
	if deadLetters {
		options.SubQueue = azservicebus.SubQueueDeadLetter
	}
	if subscription == "" {
		receiver, err := c.data.NewReceiverForQueue(entity, options)
		if err != nil {
			return nil, fmt.Errorf("opening a reader on %s: %w", entity, err)
		}
		return receiver, nil
	}
	receiver, err := c.data.NewReceiverForSubscription(entity, subscription, options)
	if err != nil {
		return nil, fmt.Errorf("opening a reader on %s/%s: %w", entity, subscription, err)
	}
	return receiver, nil
}

// browseLabel names what was read, for the message rows and for an error.
func browseLabel(entity, subscription string, deadLetters bool) string {
	name := entity
	if subscription != "" {
		name = entity + "/" + subscription
	}
	if deadLetters {
		return name + "/$DeadLetterQueue"
	}
	return name
}

/*
 * messageItemOf turns one peeked message into the canonical shape.
 *
 * QueueOffset carries the sequence number, which is the closest thing this
 * family has to an offset and is genuinely one: it is assigned in order,
 * identifies a message within its entity, and is what a browse resumes from.
 * QueueID stays unknown - a partitioned entity is spread across the service's
 * own brokers and reports no shard a caller could name.
 *
 * Tags carries the subject, which Service Bus also calls the label. It is not
 * an approximation: a correlation filter matches on it by that name, so it is
 * the one field on a message that routing decisions are made from.
 */
func messageItemOf(index int, entity string, message *azservicebus.ReceivedMessage, deadLetters bool) *model.MessageItem {
	item := &model.MessageItem{
		ID:          index,
		Topic:       entity,
		MessageID:   message.MessageID,
		QueueID:     model.UnknownMetric,
		QueueOffset: model.UnknownMetric,
		Status:      model.MsgNormal,
		Body:        string(message.Body),
		// The delivery count is how many times it has been handed out, so the
		// retries are one fewer. A peek does not move it.
		RetryTimes: int(message.DeliveryCount),
		Properties: map[string]string{PropState: stateOf(message)},
	}
	if deadLetters {
		item.Status = model.MsgDLQ
	}
	if message.DeliveryCount > 0 {
		item.RetryTimes = int(message.DeliveryCount) - 1
	}
	if message.SequenceNumber != nil {
		item.QueueOffset = *message.SequenceNumber
		item.Properties[PropSequenceNumber] = strconv.FormatInt(*message.SequenceNumber, 10)
	}
	if message.EnqueuedTime != nil {
		item.StoreTimestamp = message.EnqueuedTime.UnixMilli()
		item.StoreTime = message.EnqueuedTime.Format(time.RFC3339)
	}
	if message.Subject != nil {
		item.Tags = *message.Subject
	}
	// What a reader groups by, which is what a RocketMQ key is for. A session
	// id is the stronger of the two - it also orders delivery - so it wins.
	switch {
	case message.SessionID != nil && *message.SessionID != "":
		item.Keys = *message.SessionID
	case message.PartitionKey != nil:
		item.Keys = *message.PartitionKey
	}

	item.Properties[PropDeliveryCount] = strconv.FormatUint(uint64(message.DeliveryCount), 10)
	putProperty(item.Properties, PropContentType, message.ContentType)
	putProperty(item.Properties, PropCorrelationID, message.CorrelationID)
	putProperty(item.Properties, PropReplyTo, message.ReplyTo)
	putProperty(item.Properties, PropReplyToSessionID, message.ReplyToSessionID)
	putProperty(item.Properties, PropTo, message.To)
	putProperty(item.Properties, PropSessionID, message.SessionID)
	putProperty(item.Properties, PropPartitionKey, message.PartitionKey)
	putProperty(item.Properties, PropDeadLetterReason, message.DeadLetterReason)
	putProperty(item.Properties, PropDeadLetterDesc, message.DeadLetterErrorDescription)
	putProperty(item.Properties, PropDeadLetterSource, message.DeadLetterSource)
	putTime(item.Properties, PropScheduledEnqueueTime, message.ScheduledEnqueueTime)
	putTime(item.Properties, PropExpiresAt, message.ExpiresAt)
	if message.EnqueuedSequenceNumber != nil {
		item.Properties[PropEnqueuedSequence] = strconv.FormatInt(*message.EnqueuedSequenceNumber, 10)
	}
	for name, value := range message.ApplicationProperties {
		item.Properties[PropAttributePrefix+name] = fmt.Sprint(value)
	}
	return item
}

// stateOf is which of the three a peeked message is in.
//
// The two that are not active are the reason a peek shows more than a receive
// would: a scheduled message is held until its enqueue time and a deferred one
// has been set aside by sequence number, and no consumer is offered either.
func stateOf(message *azservicebus.ReceivedMessage) string {
	switch message.State {
	case azservicebus.MessageStateScheduled:
		return StateScheduled
	case azservicebus.MessageStateDeferred:
		return StateDeferred
	default:
		return StateActive
	}
}

func putProperty(properties map[string]string, key string, value *string) {
	if value != nil && *value != "" {
		properties[key] = *value
	}
}

func putTime(properties map[string]string, key string, value *time.Time) {
	if value != nil && !value.IsZero() {
		properties[key] = value.Format(time.RFC3339)
	}
}
