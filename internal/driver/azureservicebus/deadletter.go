package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Dead letters, which on this family are a place rather than a topology.
 *
 * Every queue and every subscription is created with a $DeadLetterQueue of its
 * own. The broker names it, moves messages into it, and lets a reader open it
 * directly at <entity>/$DeadLetterQueue - there is nothing pointing at
 * anything, nothing to walk backwards through, and no convention to follow.
 * That is DeadLetterReader's shape, and it is why this driver declares CapDLQ
 * rather than CapDeadLetterTopology.
 *
 * The distinction is not cosmetic, and the two hosted families before this one
 * are both on the other side of it. An SQS dead-letter queue is an ordinary
 * queue that another queue's redrive policy points at; a Pub/Sub one is an
 * ordinary topic a subscription's policy points at. Both are found by reading
 * every object's configuration and inverting it, and both can be deleted,
 * renamed, or shared between sources. A $DeadLetterQueue can be none of those:
 * it is part of the entity, it goes when the entity goes, and it cannot be
 * sent to.
 *
 * ForwardDeadLetteredMessagesTo does exist and does point one entity's
 * failures at another, which looks like the topology shape. It is not the same
 * thing: it is an optional forwarding rule laid over the built-in store rather
 * than the store itself, and an entity with no forwarding still has a
 * $DeadLetterQueue full of messages. Reading the built-in one is what this
 * page is for; the forwarding is a field on the entity board.
 *
 * The group argument is RocketMQ's word and carries an entity path here: a
 * queue name, or "topic/subscription". There is no consumer group to name -
 * a queue's dead letters belong to the queue, not to whoever was reading it.
 */

// resendSearch caps how far a resend will look for one message.
//
// Service Bus offers dead letters in order and has no call that takes one by
// sequence number, so reaching the fiftieth means receiving the forty-nine in
// front of it. Bounded rather than unbounded because each of those is
// abandoned afterwards, and a search through ten thousand would raise ten
// thousand delivery counts to put one message back.
const resendSearch = 200

/*
 * DLQMessages reads what an entity has given up on.
 *
 * A peek, like the ordinary browse and for the same reason: it takes nothing,
 * locks nothing, and leaves the dead letters exactly where they were. Reading
 * a backlog of failures should not be the thing that changes it.
 */
func (c *Conn) DLQMessages(ctx context.Context, group string, maxResults int) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	entity, subscription, err := entityPath(group)
	if err != nil {
		return nil, err
	}
	if maxResults <= 0 {
		maxResults = peekBatch
	}
	if maxResults > maxBrowse {
		maxResults = maxBrowse
	}

	peeked, err := c.peek(ctx, entity, subscription, true, 0, maxResults)
	if err != nil {
		return nil, err
	}

	label := browseLabel(entity, subscription, true)
	items := make([]*model.MessageItem, 0, len(peeked))
	for index, message := range peeked {
		items = append(items, messageItemOf(index+1, label, message, true))
	}
	return items, nil
}

/*
 * RetryMessages has nothing to read, and that is the family rather than a gap.
 *
 * RocketMQ moves a failed message to a %RETRY% topic per consumer group and
 * hands it back from there, so the retries are a place. Service Bus redelivers
 * in place: a message that is abandoned or whose lock expires goes back into
 * the same entity with its delivery count raised, and it is only when that
 * count passes the limit that it moves anywhere at all - into the dead-letter
 * store above. There is no third place, and inventing one would mean showing
 * the ordinary backlog under a name that says it is failing.
 */
func (c *Conn) RetryMessages(context.Context, string, int) ([]*model.MessageItem, error) {
	return nil, errNoRetryStore
}

var errNoRetryStore = errors.New(
	"service bus keeps no retry store: a failed message goes back into the same entity with " +
		"its delivery count raised, and moves to the dead-letter queue only when that count " +
		"passes the limit")

/*
 * ResendMessage puts one dead letter back where it came from.
 *
 * Three steps and no atomicity, because the service offers none: the message
 * is received from the dead-letter store, a copy is sent to the parent entity,
 * and only then is the original completed. In that order deliberately - a
 * crash between the send and the complete leaves a duplicate, which is
 * recoverable, where the other order would lose the message outright.
 *
 * Getting hold of one message is the awkward part and the comment worth
 * reading. Dead letters are offered in order and nothing takes one by sequence
 * number, so reaching a message means receiving the ones in front of it. Those
 * are abandoned rather than completed, so they stay where they are - but
 * abandoning raises their delivery count, which is a mark this operation
 * leaves on messages it did not resend. On a dead-letter store that count is
 * cosmetic: nothing dead-letters a dead letter, so no message is moved or lost
 * by it.
 *
 * The signature is RocketMQ's. consumerGroup carries the entity path,
 * messageID carries a sequence number, and clientID and topic have no
 * counterpart - a dead letter belongs to its entity rather than to whoever was
 * reading it, and there is no connected client to hand it to.
 */
func (c *Conn) ResendMessage(ctx context.Context, consumerGroup, _, _, messageID string) (string, error) {
	if err := c.live(); err != nil {
		return "", err
	}
	entity, subscription, err := entityPath(consumerGroup)
	if err != nil {
		return "", err
	}
	sequence, err := strconv.ParseInt(strings.TrimSpace(messageID), 10, 64)
	if err != nil {
		return "", fmt.Errorf(
			"a dead letter is addressed by its sequence number, not by %q", messageID)
	}

	receiver, err := c.receiver(entity, subscription, true)
	if err != nil {
		return "", err
	}
	defer func() { _ = receiver.Close(context.WithoutCancel(ctx)) }()

	// The parent entity, which is the topic for a subscription's dead letters:
	// a subscription cannot be sent to, so putting a message back means
	// publishing it again and letting the rules place it.
	sender, err := c.data.NewSender(entity, nil)
	if err != nil {
		return "", fmt.Errorf("opening a sender on %s: %w", entity, err)
	}
	defer func() { _ = sender.Close(context.WithoutCancel(ctx)) }()

	searched := 0
	for searched < resendSearch {
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		batch, err := receiver.ReceiveMessages(waitCtx, min(resendSearch-searched, 20), nil)
		cancel()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("reading %s: %w", browseLabel(entity, subscription, true), err)
		}
		if len(batch) == 0 {
			break
		}
		searched += len(batch)

		for _, message := range batch {
			if message.SequenceNumber == nil || *message.SequenceNumber != sequence {
				// Straight back, unread. The delivery count goes up and
				// nothing else does: a dead letter cannot be dead-lettered.
				_ = receiver.AbandonMessage(context.WithoutCancel(ctx), message, nil)
				continue
			}
			if err := sender.SendMessage(ctx, resubmit(message), nil); err != nil {
				_ = receiver.AbandonMessage(context.WithoutCancel(ctx), message, nil)
				return "", fmt.Errorf("putting sequence %d back on %s: %w", sequence, entity, err)
			}
			// Only now. A failure before this point leaves the dead letter
			// where it was; a failure after it leaves a duplicate, and a
			// duplicate is the recoverable one.
			if err := receiver.CompleteMessage(context.WithoutCancel(ctx), message, nil); err != nil {
				return "", fmt.Errorf(
					"sequence %d was sent back to %s but is still in the dead letters: %w",
					sequence, entity, err)
			}
			return strconv.FormatInt(sequence, 10), nil
		}
	}
	return "", fmt.Errorf(
		"sequence %d is not among the first %d dead letters on %s; dead letters are offered in "+
			"order and nothing here takes one by number",
		sequence, resendSearch, browseLabel(entity, subscription, true))
}

/*
 * resubmit builds the copy that goes back.
 *
 * A copy rather than the message itself, because there is no call that moves
 * one: what returns to the entity is a new message carrying the old one's body
 * and its sender-set fields.
 *
 * What it deliberately does not carry is the three annotations the service
 * added when it gave up - and they need removing by hand, because Service Bus
 * writes them as ordinary application properties rather than as fields of
 * their own. Copying the property table wholesale therefore carries them
 * along, and the resent message arrives in the queue already saying it was
 * dead-lettered. A live test pins that: a message re-entering the queue has
 * not failed yet.
 */
func resubmit(message *azservicebus.ReceivedMessage) *azservicebus.Message {
	copied := &azservicebus.Message{
		Body:          message.Body,
		Subject:       message.Subject,
		CorrelationID: message.CorrelationID,
		ContentType:   message.ContentType,
		ReplyTo:       message.ReplyTo,
		To:            message.To,
	}
	if message.SessionID != nil && *message.SessionID != "" {
		copied.SessionID = message.SessionID
	}
	if message.PartitionKey != nil {
		copied.PartitionKey = message.PartitionKey
	}
	if len(message.ApplicationProperties) > 0 {
		copied.ApplicationProperties = make(map[string]any, len(message.ApplicationProperties))
		for name, value := range message.ApplicationProperties {
			if deadLetterAnnotations[name] {
				continue
			}
			copied.ApplicationProperties[name] = value
		}
	}
	return copied
}

// The property names Service Bus writes when it dead-letters a message. They
// look like the sender's own and are the service's, which is what makes
// stripping them a deliberate step rather than an obvious one.
var deadLetterAnnotations = map[string]bool{
	"DeadLetterReason":           true,
	"DeadLetterErrorDescription": true,
	"DeadLetterSource":           true,
}

// entityPath splits "queue" or "topic/subscription" into its parts.
//
// The one thing it must not accept is a path that already names a sub-entity:
// "orders/$DeadLetterQueue" would otherwise be turned into
// "orders/$DeadLetterQueue/$DeadLetterQueue", which does not exist.
func entityPath(path string) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", errors.New("no queue or subscription named")
	}
	entity, subscription, split := strings.Cut(trimmed, "/")
	if !split {
		name, err := requiredName("queue", trimmed)
		return name, "", err
	}
	if _, err := requiredName("topic", entity); err != nil {
		return "", "", err
	}
	if _, err := requiredName("subscription", subscription); err != nil {
		return "", "", err
	}
	return entity, subscription, nil
}
