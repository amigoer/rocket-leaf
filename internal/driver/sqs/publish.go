package sqs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// sendBatch is the most messages one SendMessageBatch may carry. Ten is the
// service's own maximum.
const sendBatch = 10

// maxSendCount caps one console send.
//
// The cap is this driver's, not the service's: a send console is for producing
// a handful by hand rather than for load generation, and every message costs a
// request AWS bills for.
const maxSendCount = 1000

// maxDelay is the longest SQS will hold a message back. Fifteen minutes, and
// the service refuses anything longer with InvalidParameterValue.
const maxDelay = 15 * time.Minute

// PublishRequest is a send in SQS's own vocabulary.
//
// Deliberately not MessagePublisher's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary, of which an SQS message has
// only the destination and a delay in real seconds. What it has instead is a
// table of typed attributes, and on a FIFO queue two fields that are not
// optional: the group a message is ordered within, and the id it is
// deduplicated by.
type PublishRequest struct {
	Queue string
	Body  string

	// Count sends the same body more than once, for filling a queue to watch
	// a consumer work through it.
	Count int

	// Delay holds the message back from consumers. Up to fifteen minutes,
	// which is the service's limit rather than this driver's.
	Delay time.Duration

	// Attributes are the producer's own, sent as SQS string attributes. They
	// are metadata beside the body rather than inside it, which is the only
	// thing an SQS message carries apart from its payload.
	Attributes map[string]string

	// GroupID is required on a FIFO queue and refused on a standard one: it
	// is what a message is ordered against, and there is no default.
	GroupID string
	// DeduplicationID is required on a FIFO queue unless the queue
	// deduplicates on content. Two sends with the same id inside five minutes
	// are one message, and the second is accepted and silently discarded.
	DeduplicationID string
}

// PublishResult is what the send did.
type PublishResult struct {
	// Sent is how many the service accepted.
	Sent int
	// MessageID is the id of the first message, so a page can name what it
	// just produced. It addresses nothing: no call takes a message id.
	MessageID string
}

/*
 * Publish sends one body, or the same body several times.
 *
 * Batched in tens because SendMessageBatch takes ten, and a batch's entries
 * succeed and fail individually - so a partial send is a real outcome and the
 * count is carried in the error as well as in the result. The bridge above
 * hands a page the error alone.
 *
 * The FIFO rules are checked here rather than left to the service. SQS refuses
 * a FIFO send with no group id as MissingParameter naming MessageGroupId,
 * which is a field the console does draw - but it refuses a group id on a
 * standard queue by naming it too, and a user who filled it in by habit would
 * be told the field they typed is missing.
 */
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Queue) == "" {
		return nil, errors.New("a send needs a queue")
	}
	if request.Body == "" {
		// SQS answers InvalidParameterValue naming MessageBody, which is
		// correct and says nothing about which field was blank.
		return nil, errors.New("sqs refuses an empty message body")
	}
	if request.Delay < 0 {
		return nil, errors.New("a delay cannot be negative")
	}
	if request.Delay > maxDelay {
		return nil, fmt.Errorf("sqs holds a message back for at most %v", maxDelay)
	}

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d messages at once", maxSendCount)
	}

	fifo := isFIFO(request.Queue)
	if fifo && strings.TrimSpace(request.GroupID) == "" {
		return nil, errors.New(
			"a FIFO queue orders messages within a group, so every send needs a group id")
	}
	if !fifo && strings.TrimSpace(request.GroupID) != "" {
		return nil, errors.New(
			"only a FIFO queue takes a group id; this queue's name does not end in .fifo")
	}
	// Refused before anything is sent rather than after. A FIFO queue takes no
	// per-message delay, and sending the batch anyway would deliver messages
	// immediately under a report that they had been held back.
	if fifo && request.Delay > 0 {
		return nil, errors.New(
			"a FIFO queue takes no per-message delay; set a delivery delay on the queue instead")
	}

	url, err := c.queueURL(ctx, request.Queue)
	if err != nil {
		return nil, err
	}

	attributes := messageAttributesOf(request.Attributes)
	result := &PublishResult{}
	for start := 0; start < count; start += sendBatch {
		end := min(start+sendBatch, count)
		entries := make([]types.SendMessageBatchRequestEntry, 0, end-start)
		for index := start; index < end; index++ {
			entry := types.SendMessageBatchRequestEntry{
				Id:                aws.String(strconv.Itoa(index)),
				MessageBody:       aws.String(request.Body),
				DelaySeconds:      int32(request.Delay.Seconds()),
				MessageAttributes: attributes,
			}
			if fifo {
				entry.MessageGroupId = aws.String(strings.TrimSpace(request.GroupID))
				// Every copy needs its own deduplication id, or SQS keeps the
				// first and discards the rest without saying so - a repeat of
				// ten would arrive as one.
				entry.MessageDeduplicationId = deduplicationIDFor(request, index)
			}
			entries = append(entries, entry)
		}

		out, err := c.client.SendMessageBatch(ctx, &awssqs.SendMessageBatchInput{
			QueueUrl: aws.String(url),
			Entries:  entries,
		})
		if err != nil {
			return result, fmt.Errorf("%s took %d of %d before failing: %w",
				request.Queue, result.Sent, count, err)
		}
		for _, ok := range out.Successful {
			result.Sent++
			if result.MessageID == "" {
				result.MessageID = aws.ToString(ok.MessageId)
			}
		}
		// A batch's entries fail individually, so a request that returned no
		// error can still have refused messages - and reporting only the
		// successes would call that a clean send.
		if len(out.Failed) > 0 {
			return result, fmt.Errorf("%s refused %d of %d: %s",
				request.Queue, len(out.Failed), count, aws.ToString(out.Failed[0].Message))
		}
	}
	return result, nil
}

// deduplicationIDFor gives each copy of a repeated body its own id.
//
// Without one SQS deduplicates them into a single message inside its
// five-minute window - accepted, acknowledged and silently discarded - so a
// console asked for ten would report ten and deliver one. A queue that
// deduplicates on content has the same problem and cannot be given an explicit
// id, which is why the index is appended to whatever the caller supplied
// rather than replacing it.
func deduplicationIDFor(request PublishRequest, index int) *string {
	base := strings.TrimSpace(request.DeduplicationID)
	if base == "" {
		base = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if request.Count <= 1 {
		return aws.String(base)
	}
	return aws.String(fmt.Sprintf("%s-%d", base, index))
}

// messageAttributesOf turns the console's key/value table into SQS attributes.
//
// String only. SQS also carries Number and Binary, and a console that guessed
// between them from the text typed would send "007" as the number seven.
func messageAttributesOf(attributes map[string]string) map[string]types.MessageAttributeValue {
	if len(attributes) == 0 {
		return nil
	}
	built := make(map[string]types.MessageAttributeValue, len(attributes))
	for name, value := range attributes {
		if strings.TrimSpace(name) == "" || value == "" {
			continue
		}
		built[name] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}
	if len(built) == 0 {
		return nil
	}
	return built
}

/*
 * SendMessage publishes through the canonical port.
 *
 * The port is RocketMQ's shape and three of its five arguments land
 * differently here, so each is handled deliberately rather than dropped:
 *
 *   - tags is refused. An SQS message has a body and a table of named
 *     attributes and nothing else, so a tag would be silently discarded and
 *     the send reported as having carried it.
 *   - keys carries the FIFO group id, which is the nearest thing SQS has to
 *     what a RocketMQ key is for: the value a reader groups by. It is required
 *     on a FIFO queue and refused on a standard one.
 *   - delayLevel is seconds, not a level. RocketMQ's levels are an index into
 *     a broker-side table; SQS takes a duration and ports.go fixes no unit.
 *     Seconds is what Pulsar's and NSQ's drivers chose and what the console
 *     labels.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	if strings.TrimSpace(tags) != "" {
		return "", errors.New(
			"an sqs message has a body and named attributes and no tag; send it as an attribute")
	}
	if delayLevel < 0 {
		return "", errors.New("a delay cannot be negative")
	}
	result, err := c.Publish(ctx, PublishRequest{
		Queue:   topic,
		Body:    body,
		Count:   1,
		Delay:   time.Duration(delayLevel) * time.Second,
		GroupID: keys,
	})
	if result == nil {
		return "", err
	}
	return result.MessageID, err
}
