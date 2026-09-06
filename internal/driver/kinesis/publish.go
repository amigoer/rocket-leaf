package kinesis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

// putBatch is the most records one PutRecords may carry. Five hundred is the
// service's own maximum, and the batch is also capped at five megabytes.
const putBatch = 500

// maxSendCount caps one console send.
//
// The cap is this driver's, not the service's: a send console is for producing
// a handful by hand rather than for load generation, and a stream's write
// allowance is a thousand records a second per shard.
const maxSendCount = 1000

// maxRecordBytes is the largest payload one record may carry. A megabyte, and
// the service refuses anything larger with ValidationException.
const maxRecordBytes = 1024 * 1024

// PublishRequest is a send in Kinesis's own vocabulary.
//
// Deliberately not MessagePublisher's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary, of which a Kinesis record
// has the destination and nothing else. A record is bytes plus the two values
// that decide where it lands, and there is no header table, no tag and no way
// anywhere in the service to hold one back.
type PublishRequest struct {
	Stream string
	Body   string

	// PartitionKey is required on every send and is what places the record:
	// Kinesis hashes it into the 128-bit key space and the shard whose range
	// covers the hash takes it. Records sharing a key land in order on one
	// shard, which is the only ordering guarantee the service makes.
	PartitionKey string

	// ExplicitHashKey overrides that hash with one chosen by the sender,
	// which is how a record is aimed at a particular shard. It is the only
	// way to write to a shard by name, and it is why the shards page shows
	// each shard's range.
	ExplicitHashKey string

	// Count sends the same body more than once, for filling a stream to watch
	// a consumer work through it. Each copy gets its own partition key suffix
	// unless a hash key aims them all at one shard.
	Count int
}

// PublishResult is what the send did.
type PublishResult struct {
	// Sent is how many the service accepted.
	Sent int
	// SequenceNumber is the first record's, and ShardID is the shard that took
	// it. Both, because neither addresses a record on its own.
	SequenceNumber string
	ShardID        string
	// Failed counts records the service rejected individually. A PutRecords
	// that returns no error can still have refused most of its batch, which is
	// the failure mode a count of successes alone would hide.
	Failed int
}

/*
 * Publish sends one body, or the same body several times.
 *
 * Batched in five hundreds because PutRecords takes five hundred, and a
 * batch's entries succeed and fail individually - a request that returned no
 * error can still have refused most of what it carried, usually because a
 * shard was over its write allowance. So the failures are counted and
 * reported rather than inferred from the successes.
 *
 * A repeated body gets a partition key per copy. Sending them all under one
 * key would put every copy on a single shard, which is a fair thing to ask for
 * and a terrible default: a console asked to fill a stream would load one
 * shard and leave the rest idle, and the browse afterwards would look as
 * though the stream were not spreading records at all.
 */
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	stream := strings.TrimSpace(request.Stream)
	if stream == "" {
		return nil, errors.New("a send needs a stream")
	}
	if request.Body == "" {
		// The service answers ValidationException naming Data, which is
		// correct and says nothing about which field was blank.
		return nil, errors.New("kinesis refuses an empty record")
	}
	if len(request.Body) > maxRecordBytes {
		return nil, fmt.Errorf("a kinesis record carries at most %d bytes; this one is %d",
			maxRecordBytes, len(request.Body))
	}
	key := strings.TrimSpace(request.PartitionKey)
	if key == "" {
		return nil, errors.New(
			"every record needs a partition key: it is what decides which shard takes it")
	}

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d records at once", maxSendCount)
	}

	hashKey := strings.TrimSpace(request.ExplicitHashKey)
	if hashKey != "" {
		if err := validHashKey(hashKey); err != nil {
			return nil, err
		}
	}

	result := &PublishResult{}
	for start := 0; start < count; start += putBatch {
		end := min(start+putBatch, count)
		entries := make([]types.PutRecordsRequestEntry, 0, end-start)
		for index := start; index < end; index++ {
			entry := types.PutRecordsRequestEntry{
				Data:         []byte(request.Body),
				PartitionKey: aws.String(partitionKeyFor(key, index, count, hashKey)),
			}
			if hashKey != "" {
				entry.ExplicitHashKey = aws.String(hashKey)
			}
			entries = append(entries, entry)
		}

		out, err := c.client.PutRecords(ctx, &awskinesis.PutRecordsInput{
			StreamName: aws.String(stream),
			Records:    entries,
		})
		if err != nil {
			if notFound(err) {
				return result, fmt.Errorf("no stream named %q in %s", stream, c.config.region)
			}
			return result, fmt.Errorf("%s took %d of %d before failing: %w",
				stream, result.Sent, count, err)
		}
		for _, record := range out.Records {
			if record.ErrorCode != nil {
				result.Failed++
				continue
			}
			result.Sent++
			if result.SequenceNumber == "" {
				result.SequenceNumber = aws.ToString(record.SequenceNumber)
				result.ShardID = aws.ToString(record.ShardId)
			}
		}
		if failed := aws.ToInt32(out.FailedRecordCount); failed > 0 {
			return result, fmt.Errorf(
				"%s refused %d of %d, usually because a shard is over its write allowance: %s",
				stream, failed, count, firstPutError(out.Records))
		}
	}
	return result, nil
}

/*
 * partitionKeyFor gives each copy of a repeated body a key of its own.
 *
 * Not when a hash key is set: that aims every record at one shard on purpose,
 * and varying the partition key would be doing nothing while looking like it
 * did something. Not when there is one record either - a single send should
 * carry exactly the key that was typed, because the point of typing one is to
 * see which shard it lands on.
 */
func partitionKeyFor(key string, index, count int, hashKey string) string {
	if count <= 1 || hashKey != "" {
		return key
	}
	return key + "-" + strconv.Itoa(index)
}

// validHashKey checks the sender-chosen hash a record can be aimed with.
//
// It is a 128-bit unsigned integer written in decimal, which is why it is a
// string here and everywhere else: the largest of them is 39 digits and does
// not fit an int64. The service refuses a bad one as ValidationException
// quoting a regular expression, which names neither the field nor the range.
func validHashKey(value string) error {
	if len(value) > 39 {
		return fmt.Errorf("an explicit hash key is a 128-bit number; %q is too long", value)
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return fmt.Errorf(
				"an explicit hash key is a decimal number between 0 and 2^128-1; %q is not", value)
		}
	}
	return nil
}

// firstPutError names why a batch's entries were refused, since the count
// alone says nothing about what to change.
func firstPutError(records []types.PutRecordsResultEntry) string {
	for _, record := range records {
		if record.ErrorCode != nil {
			return aws.ToString(record.ErrorCode) + ": " + aws.ToString(record.ErrorMessage)
		}
	}
	return "the service gave no reason"
}

/*
 * SendMessage publishes through the canonical port.
 *
 * The port is RocketMQ's shape and three of its five arguments have no
 * counterpart here, so each is handled deliberately rather than dropped:
 *
 *   - tags is refused. A Kinesis record is bytes and a partition key, with no
 *     header table of any kind, so a tag would be silently discarded and the
 *     send reported as having carried it.
 *   - keys carries the partition key, which is what a record actually has:
 *     the value the service hashes to choose a shard.
 *   - delayLevel is refused rather than ignored. Nothing anywhere in Kinesis
 *     holds a record back - a record is readable as soon as it is written -
 *     and accepting a delay would report a scheduled send that happened
 *     immediately.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	if strings.TrimSpace(tags) != "" {
		return "", errors.New(
			"a kinesis record is bytes and a partition key; there is no tag and no header to put one in")
	}
	if delayLevel != 0 {
		return "", errors.New(
			"kinesis delivers a record as soon as it is written; nothing in the service holds one back")
	}
	result, err := c.Publish(ctx, PublishRequest{
		Stream:       topic,
		Body:         body,
		PartitionKey: keys,
		Count:        1,
	})
	if result == nil {
		return "", err
	}
	// The pair, because a sequence number addresses nothing without the shard
	// that holds it - which is what MessageByID takes.
	if result.SequenceNumber == "" {
		return "", err
	}
	return result.ShardID + idSeparator + result.SequenceNumber, err
}
