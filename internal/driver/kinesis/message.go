package kinesis

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Message property keys this driver fills in. A contract between this package
// and frontend/src/mq/kinesis/messages.ts.
const (
	PropShardID        = "shardId"
	PropSequenceNumber = "sequenceNumber"
	PropPartitionKey   = "partitionKey"
	PropEncryption     = "encryptionType"
)

// Filter keys the browse understands, on model.MessageQueryParams.Filters.
const (
	// FilterShardID narrows a browse to one shard. Without it the read
	// touches every shard the stream has, which is what makes a browse cost
	// the read quota of the whole stream rather than of one part of it.
	FilterShardID = "shardId"
)

// idSeparator joins a shard id to a sequence number.
//
// A Kinesis record has no id of its own: the sender may set a partition key,
// which is not unique, and the service assigns a sequence number, which is
// unique only within its shard. So the handle that addresses one record is the
// pair, and GetShardIterator proves it - a sequence number offered against the
// wrong shard is refused outright.
const idSeparator = ":"

// maxBrowse caps one browse however many were asked for.
//
// The cap is this driver's rather than the service's. Nothing is consumed by
// reading, so the risk is not lost messages - it is that a browse of a
// hundred thousand would spend minutes of every shard's read quota, which the
// applications reading that stream share.
const maxBrowse = 500

// browseCallsPerShard bounds how many GetRecords a browse makes per shard.
//
// Five, which is exactly one second of a shard's read allowance: the service
// permits five GetRecords a second per shard, shared with every classic
// consumer on it. A browse that spent more than that would be taking read
// capacity from a running application to fill a page.
const browseCallsPerShard = 5

// browseShardFanOut caps how many shards are read at once. Each is its own
// quota, so this is about the account's request rate rather than the stream's.
const browseShardFanOut = 8

/*
 * QueryMessages reads what a stream is holding, without taking any of it.
 *
 * This really is a browse, and the difference from every other hosted family
 * here matters: GetRecords removes nothing, hides nothing and marks nothing.
 * A record stays until the retention period expires and any number of readers
 * can read the same one, so the caveat SQS and Pub/Sub carry - that looking at
 * a message takes it away from a consumer - would be false here.
 *
 * What is true is that reading is not free. A shard allows five GetRecords a
 * second and two megabytes a second, and that budget is shared with every
 * classic consumer on it - so a browse arriving beside a busy application can
 * push it into ProvisionedThroughputExceededException without having taken a
 * single record. That is what the capability's caveat says, and it is why the
 * per-shard call budget above is a hard limit rather than a tuning knob.
 *
 * A stream has no single log to read, so this is one iterator per shard, read
 * in parallel and merged by arrival time. Filters narrows it to one shard for
 * a caller that knows which one it wants.
 */
func (c *Conn) QueryMessages(ctx context.Context, params model.MessageQueryParams) ([]*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	stream := strings.TrimSpace(params.Topic)
	if stream == "" {
		return nil, errors.New("a browse needs a stream name")
	}

	wanted := params.MaxResults
	if wanted <= 0 || wanted > maxBrowse {
		wanted = maxBrowse
	}

	shards, err := c.browsableShards(ctx, stream, params.Filters[FilterShardID])
	if err != nil {
		return nil, err
	}
	if len(shards) == 0 {
		return nil, nil
	}

	// Split evenly rather than first-come, so one busy shard cannot fill the
	// page and leave the others invisible. A shard with less than its share
	// simply returns fewer.
	perShard := wanted/len(shards) + 1

	var mu sync.Mutex
	var collected []*model.MessageItem
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(browseShardFanOut)
	for _, shard := range shards {
		group.Go(func() error {
			records, err := c.readShard(groupCtx, stream, shard, params, perShard)
			if err != nil {
				return fmt.Errorf("%s: %w", shard, err)
			}
			mu.Lock()
			collected = append(collected, records...)
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	// Newest first, which is what every other browse in this app shows. Within
	// one millisecond the sequence number decides, because it is the only
	// thing that orders two records that arrived together.
	sort.SliceStable(collected, func(i, j int) bool {
		if collected[i].StoreTimestamp != collected[j].StoreTimestamp {
			return collected[i].StoreTimestamp > collected[j].StoreTimestamp
		}
		return collected[i].MessageID > collected[j].MessageID
	})
	if len(collected) > wanted {
		collected = collected[:wanted]
	}
	for index, message := range collected {
		message.ID = index + 1
	}
	return collected, nil
}

/*
 * MessageByID fetches one record by the pair that addresses it.
 *
 * The id is "<shard id>:<sequence number>", and both halves are needed rather
 * than one being a convenience. A sequence number is unique within its shard
 * and nowhere else, and GetShardIterator refuses one offered against a
 * different shard - so there is no call anywhere in the API that takes a
 * sequence number alone. That is the same fact the shards page exists for,
 * met at a different height.
 */
func (c *Conn) MessageByID(ctx context.Context, topic, messageID string) (*model.MessageItem, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	stream := strings.TrimSpace(topic)
	if stream == "" {
		return nil, errors.New("a record is addressed within a stream")
	}
	shard, sequence, ok := strings.Cut(strings.TrimSpace(messageID), idSeparator)
	if !ok || shard == "" || sequence == "" {
		return nil, fmt.Errorf(
			"a kinesis record is addressed by shard and sequence number, written "+
				"%q; %q names only one of them", "shardId-000000000000:49590...", messageID)
	}

	iterator, err := c.client.GetShardIterator(ctx, &awskinesis.GetShardIteratorInput{
		StreamName:             aws.String(stream),
		ShardId:                aws.String(shard),
		ShardIteratorType:      types.ShardIteratorTypeAtSequenceNumber,
		StartingSequenceNumber: aws.String(sequence),
	})
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no shard %s on %s", shard, stream)
		}
		return nil, fmt.Errorf("%s is not a sequence number on %s: %w", sequence, shard, err)
	}

	out, err := c.client.GetRecords(ctx, &awskinesis.GetRecordsInput{
		ShardIterator: iterator.ShardIterator,
		Limit:         aws.Int32(1),
	})
	if err != nil {
		return nil, browseError(err, shard)
	}
	if len(out.Records) == 0 {
		return nil, fmt.Errorf(
			"%s holds no record at %s; it may have aged out of the retention period",
			shard, sequence)
	}
	return messageOf(stream, shard, out.Records[0]), nil
}

// browsableShards is which shards a browse reads.
//
// Closed shards are included when no shard was named. They still hold every
// record written before the split or merge that closed them, so leaving them
// out would make a browse of a resized stream silently miss the older half.
func (c *Conn) browsableShards(ctx context.Context, stream, only string) ([]string, error) {
	if named := strings.TrimSpace(only); named != "" {
		return []string{named}, nil
	}
	shards, err := c.ListShards(ctx, model.DestinationRef{Name: stream})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(shards))
	for _, shard := range shards {
		ids = append(ids, shard.ID)
	}
	return ids, nil
}

/*
 * readShard pulls up to wanted records from one shard.
 *
 * The loop ends on whichever comes first: enough records, the call budget, a
 * shard that has been read to its end - which a closed shard signals by
 * returning no next iterator - or an empty batch from a shard that says it is
 * caught up. That last condition is what MillisBehindLatest is used for here,
 * and it is the only thing it is used for: it describes this read rather than
 * any consumer's position, so it goes nowhere near a message or a backlog.
 */
func (c *Conn) readShard(
	ctx context.Context, stream, shard string, params model.MessageQueryParams, wanted int,
) ([]*model.MessageItem, error) {
	input := &awskinesis.GetShardIteratorInput{
		StreamName:        aws.String(stream),
		ShardId:           aws.String(shard),
		ShardIteratorType: types.ShardIteratorTypeTrimHorizon,
	}
	if params.StartTime > 0 {
		input.ShardIteratorType = types.ShardIteratorTypeAtTimestamp
		input.Timestamp = aws.Time(time.UnixMilli(params.StartTime).UTC())
	}

	iterator, err := c.client.GetShardIterator(ctx, input)
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no such shard on %s", stream)
		}
		return nil, err
	}

	collected := make([]*model.MessageItem, 0, wanted)
	cursor := iterator.ShardIterator
	for calls := 0; calls < browseCallsPerShard && len(collected) < wanted; calls++ {
		if cursor == nil || aws.ToString(cursor) == "" {
			break
		}
		out, err := c.client.GetRecords(ctx, &awskinesis.GetRecordsInput{
			ShardIterator: cursor,
			Limit:         aws.Int32(int32(min(wanted-len(collected), maxBrowse))),
		})
		if err != nil {
			return nil, browseError(err, shard)
		}
		for _, record := range out.Records {
			message := messageOf(stream, shard, record)
			if !matches(message, params) {
				continue
			}
			collected = append(collected, message)
		}
		cursor = out.NextShardIterator
		// An empty batch from a shard that reports itself caught up is the end
		// of the shard, not a slow read. Anything else is worth another call.
		if len(out.Records) == 0 && aws.ToInt64(out.MillisBehindLatest) == 0 {
			break
		}
	}
	return collected, nil
}

// matches applies the filters the service cannot.
//
// Kinesis has no server-side selection of any kind: GetRecords returns what is
// there in order, so an end time and a partition key are both applied here.
// That is why a narrow search still costs a whole read.
func matches(message *model.MessageItem, params model.MessageQueryParams) bool {
	if params.EndTime > 0 && message.StoreTimestamp > params.EndTime {
		return false
	}
	if key := strings.TrimSpace(params.MessageKey); key != "" {
		if message.Keys != key {
			return false
		}
	}
	return true
}

/*
 * messageOf turns one record into a canonical message.
 *
 * QueueID and QueueOffset are left alone deliberately. They are a partition
 * index and a position in it, and a shard is neither: its id is a name and its
 * sequence number is a 56-digit value that does not fit an int64. Both travel
 * as properties instead, and MessageID carries the pair that addresses the
 * record.
 */
func messageOf(stream, shard string, record types.Record) *model.MessageItem {
	sequence := aws.ToString(record.SequenceNumber)
	arrived := time.Time{}
	if record.ApproximateArrivalTimestamp != nil {
		arrived = record.ApproximateArrivalTimestamp.UTC()
	}
	return &model.MessageItem{
		Topic:     stream,
		MessageID: shard + idSeparator + sequence,
		// The partition key is the nearest thing a record has to a key, and it
		// is the sender's own: it decides the shard by its hash and is carried
		// through unchanged.
		Keys:           aws.ToString(record.PartitionKey),
		StoreTime:      arrived.Format(time.RFC3339),
		StoreTimestamp: arrived.UnixMilli(),
		Status:         model.MsgNormal,
		Body:           string(record.Data),
		Properties: map[string]string{
			PropShardID:        shard,
			PropSequenceNumber: sequence,
			PropPartitionKey:   aws.ToString(record.PartitionKey),
			PropEncryption:     string(record.EncryptionType),
		},
	}
}

// browseError names the limit a failed read hit.
//
// Worth separating because nothing about the stream looks wrong when this
// happens: the service answers over its per-shard read allowance with an
// exception rather than with a slow response, and the allowance is shared with
// whatever else is reading that shard.
func browseError(err error, shard string) error {
	var exceeded *types.ProvisionedThroughputExceededException
	if errors.As(err, &exceeded) {
		return fmt.Errorf(
			"%s is over its read allowance - five GetRecords a second and two "+
				"megabytes a second, shared with every consumer reading it: %w", shard, err)
	}
	return err
}
