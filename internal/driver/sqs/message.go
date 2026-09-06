package sqs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/amigoer/mq-studio/internal/model"
)

// Message property keys this driver fills in. A contract between this package
// and frontend/src/mq/sqs/messages.ts.
const (
	PropReceiveCount    = "approximateReceiveCount"
	PropFirstReceivedAt = "approximateFirstReceiveTimestamp"
	PropSenderID        = "senderId"
	PropGroupID         = "messageGroupId"
	PropDeduplicationID = "messageDeduplicationId"
	PropSequenceNumber  = "sequenceNumber"
	PropBodyMD5         = "md5OfBody"
)

// receiveBatch is the most messages one ReceiveMessage may return. Ten is the
// service's own maximum, so a browse of more than ten is several calls.
const receiveBatch = 10

// browseWaitSeconds long-polls each receive for a second.
//
// Not an optimisation. A short poll samples a subset of the servers holding a
// queue, so it can answer empty on a queue that is not - which on a browse
// page reads as "this queue has nothing in it". Long polling queries all of
// them, and one second is enough to do that without making the page wait.
const browseWaitSeconds = 1

// maxBrowse caps one browse however many were asked for.
//
// The cap is this driver's rather than the service's: every message returned
// is hidden from real consumers while the page is assembled, so a browse of a
// hundred thousand would be an outage dressed as a page.
const maxBrowse = 500

/*
 * QueryMessages reads what a queue is holding, and hands it straight back.
 *
 * This is not a browse, and the capability carries a caveat saying so. SQS has
 * exactly one read - ReceiveMessage - and it is the same call a consumer
 * makes: what it returns becomes invisible to everyone else for the visibility
 * timeout, and its receive count goes up, which counts towards the redrive
 * policy's limit. So a page of messages read here is a page a consumer running
 * at the same moment did not get, and a message read enough times ends up in
 * the dead-letter queue with nothing having failed.
 *
 * What the driver can do is shorten the window and it does: every message is
 * returned with a visibility timeout of zero as soon as the batch is
 * assembled, so the messages are available again in about as long as the
 * request takes. It cannot close the window, and it cannot undo the receive
 * count. Hence the caveat rather than a silent best effort.
 *
 * Filtering is left to the caller for the same reason the read is destructive:
 * there is no server-side selector of any kind, so narrowing means receiving
 * everything and discarding most of it - which would hide far more messages
 * for far longer than the page showed.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	url, err := c.queueURL(ctx, params.Topic)
	if err != nil {
		return nil, err
	}

	wanted := params.MaxResults
	if wanted <= 0 {
		wanted = receiveBatch
	}
	if wanted > maxBrowse {
		wanted = maxBrowse
	}

	collected, handles, err := c.receiveHeld(ctx, url, wanted)
	// The release runs whatever happened, including on the error path: the
	// messages already taken are hidden either way, and leaving them so
	// because the next call failed is the worst outcome available.
	defer c.release(context.WithoutCancel(ctx), url, handles)
	if err != nil {
		return nil, err
	}

	items := make([]*model.MessageItem, 0, len(collected))
	for index, message := range collected {
		items = append(items, messageItemOf(index+1, params.Topic, message))
	}
	// Newest first, which is what every other family's browse shows. SQS
	// returns whatever its servers offered, in no order at all on a standard
	// queue - so the ordering is this app's and the timestamp is the only
	// thing it can honestly sort on.
	sort.SliceStable(items, func(a, b int) bool {
		return items[a].StoreTimestamp > items[b].StoreTimestamp
	})
	return items, nil
}

/*
 * MessageByID is not offered. SQS has no call that takes a message id: an id
 * is assigned on send and echoed on receive, and the only way to reach a
 * message is to be handed it. The capability is not declared, so nothing in
 * the UI reaches this.
 */
func (c *Conn) MessageByID(context.Context, string, string) (*model.MessageItem, error) {
	return nil, errNoMessageByID
}

var errNoMessageByID = errors.New(
	"sqs has no call that fetches a message by id; an id is assigned on send and " +
		"echoed on receive, and nothing indexes one")

// receiveHeld pulls up to wanted messages, holding each one so the next call
// offers something new.
//
// The hold is what makes several calls add up to a page: a message left
// visible would be handed back by the very next receive, and a browse of fifty
// would return the same ten five times. It ends with the release below.
func (c *Conn) receiveHeld(ctx context.Context, url string, wanted int) ([]types.Message, []string, error) {
	collected := make([]types.Message, 0, wanted)
	handles := make([]string, 0, wanted)
	seen := make(map[string]bool, wanted)

	for len(collected) < wanted {
		batch := min(wanted-len(collected), receiveBatch)
		out, err := c.client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:            aws.String(url),
			MaxNumberOfMessages: int32(batch),
			WaitTimeSeconds:     browseWaitSeconds,
			// The hold has to outlast the whole page, not one batch: a message
			// taken in the first round would otherwise reappear in the fourth
			// and be counted twice.
			VisibilityTimeout:     browseHoldSeconds,
			MessageAttributeNames: []string{"All"},
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{
				types.MessageSystemAttributeNameAll,
			},
		})
		if err != nil {
			return collected, handles, err
		}
		if len(out.Messages) == 0 {
			// An empty long poll means the queue has nothing more to offer
			// that is not already held here. It is the only stop condition
			// there is: no count is exact enough to compare against.
			break
		}
		for _, message := range out.Messages {
			handles = append(handles, aws.ToString(message.ReceiptHandle))
			// A duplicate is possible on a standard queue - at-least-once
			// delivery applies to this read too - and showing one message
			// twice would read as a duplicate in the queue rather than in the
			// reading of it.
			id := aws.ToString(message.MessageId)
			if seen[id] {
				continue
			}
			seen[id] = true
			collected = append(collected, message)
		}
	}
	return collected, handles, nil
}

// browseHoldSeconds is how long a browsed message stays hidden if the release
// below fails. Short enough that a failure costs seconds rather than the
// queue's own timeout, which is typically thirty and may be twelve hours.
const browseHoldSeconds = 5

/*
 * release hands every browsed message straight back.
 *
 * In batches of ten, because ChangeMessageVisibilityBatch takes ten, and best
 * effort throughout: a handle that has already expired fails and the rest must
 * still be returned. What it cannot do is make the read never have happened -
 * the receive count has already gone up - which is what the caveat is for.
 */
func (c *Conn) release(ctx context.Context, url string, handles []string) {
	for start := 0; start < len(handles); start += receiveBatch {
		end := min(start+receiveBatch, len(handles))
		entries := make([]types.ChangeMessageVisibilityBatchRequestEntry, 0, end-start)
		for index, handle := range handles[start:end] {
			entries = append(entries, types.ChangeMessageVisibilityBatchRequestEntry{
				Id:                aws.String(strconv.Itoa(index)),
				ReceiptHandle:     aws.String(handle),
				VisibilityTimeout: 0,
			})
		}
		_, _ = c.client.ChangeMessageVisibilityBatch(ctx, &awssqs.ChangeMessageVisibilityBatchInput{
			QueueUrl: aws.String(url),
			Entries:  entries,
		})
	}
}

// messageItemOf turns one received message into the canonical shape.
//
// Several of the canonical fields have no counterpart and stay empty rather
// than being filled with something plausible: there is no tag, no queue id, no
// offset, and no store or born host - a message arrives over HTTPS from
// whoever signed the request.
func messageItemOf(index int, queue string, message types.Message) *model.MessageItem {
	item := &model.MessageItem{
		ID:          index,
		Topic:       queue,
		MessageID:   aws.ToString(message.MessageId),
		QueueID:     model.UnknownMetric,
		QueueOffset: model.UnknownMetric,
		Status:      model.MsgNormal,
		Body:        aws.ToString(message.Body),
		Properties:  map[string]string{},
	}

	if sent := message.Attributes["SentTimestamp"]; sent != "" {
		if millis, err := strconv.ParseInt(sent, 10, 64); err == nil {
			item.StoreTimestamp = millis
			item.StoreTime = time.UnixMilli(millis).Format(time.RFC3339)
		}
	}
	// The receive count is the field to read twice. It counts every receive,
	// including a browse from this app, and the redrive policy compares it
	// against maxReceiveCount - so a message browsed enough times is
	// dead-lettered with nothing having failed.
	if count := message.Attributes["ApproximateReceiveCount"]; count != "" {
		item.Properties[PropReceiveCount] = count
		if parsed, err := strconv.Atoi(count); err == nil {
			item.RetryTimes = parsed - 1
		}
	}
	for key, attribute := range map[string]string{
		PropFirstReceivedAt: "ApproximateFirstReceiveTimestamp",
		PropSenderID:        "SenderId",
		PropGroupID:         "MessageGroupId",
		PropDeduplicationID: "MessageDeduplicationId",
		PropSequenceNumber:  "SequenceNumber",
	} {
		if value := message.Attributes[attribute]; value != "" {
			item.Properties[key] = value
		}
	}
	if md5 := aws.ToString(message.MD5OfBody); md5 != "" {
		item.Properties[PropBodyMD5] = md5
	}

	// The message attributes a producer set, flattened. Keys are the
	// producer's own, so they are prefixed to keep them apart from the
	// system attributes above, which SQS names and this driver renames.
	for name, attribute := range message.MessageAttributes {
		item.Properties["attr."+name] = messageAttributeText(attribute)
	}
	// A group id is what makes a FIFO message ordered against its siblings,
	// and RocketMQ's keys field is the nearest canonical home for it: it is
	// what a reader groups by.
	item.Keys = item.Properties[PropGroupID]
	return item
}

// messageAttributeText renders one message attribute for display. Binary
// values are described rather than decoded: they are arbitrary bytes, and a
// browse showing mojibake would be worse than a size.
func messageAttributeText(attribute types.MessageAttributeValue) string {
	if value := aws.ToString(attribute.StringValue); value != "" {
		return value
	}
	if len(attribute.StringListValues) > 0 {
		return strings.Join(attribute.StringListValues, ", ")
	}
	if len(attribute.BinaryValue) > 0 {
		return fmt.Sprintf("<%d bytes>", len(attribute.BinaryValue))
	}
	return ""
}
