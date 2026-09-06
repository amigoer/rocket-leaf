package kinesis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

	"github.com/amigoer/mq-studio/internal/model"
)

// shardPageSize is how many shards one ListShards call asks for. 10000 is the
// service's own maximum, and paging is the only way past it.
const shardPageSize = 10000

/*
 * ListShards is the whole of what this family needed a port for.
 *
 * The listing is not filtered to the open shards, and that is the point. A
 * stream resized yesterday has closed parents that take no writes, still hold
 * every record written to them until retention expires, and are named as their
 * children's parent - so hiding them would hide both the records and the
 * reason the stream has more shards than its count says. The open shard count
 * on the streams page is the other half of the same fact.
 *
 * The result is ordered by shard id, which is also lineage order: the service
 * numbers shards as it creates them, so a parent always sorts before the
 * children split out of it.
 */
func (c *Conn) ListShards(ctx context.Context, ref model.DestinationRef) ([]*model.Shard, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return nil, fmt.Errorf("no stream name given")
	}

	var shards []*model.Shard
	var token *string
	for {
		// StreamName and NextToken are mutually exclusive: the token already
		// carries which stream it is paging, and sending both is refused.
		input := &awskinesis.ListShardsInput{MaxResults: aws.Int32(shardPageSize)}
		if token == nil {
			input.StreamName = aws.String(name)
		} else {
			input.NextToken = token
		}

		page, err := c.client.ListShards(ctx, input)
		if err != nil {
			if notFound(err) {
				return nil, fmt.Errorf("no stream named %q in %s", name, c.config.region)
			}
			return nil, err
		}
		for _, shard := range page.Shards {
			shards = append(shards, shardOf(shard))
		}
		if page.NextToken == nil || aws.ToString(page.NextToken) == "" {
			break
		}
		token = page.NextToken
	}

	sort.Slice(shards, func(i, j int) bool { return shards[i].ID < shards[j].ID })
	return shards, nil
}

/*
 * shardOf turns one shard into the canonical shape.
 *
 * Closed is read off the ending sequence number rather than a status field,
 * because the API has no status field: a shard that has been split or merged
 * is one whose sequence number range has an end, and every other shard's is
 * open ended. That is the only signal, and it is why the parent of a split
 * keeps appearing in a listing long after the resize looked finished.
 */
func shardOf(shard types.Shard) *model.Shard {
	converted := &model.Shard{
		ID:               aws.ToString(shard.ShardId),
		ParentID:         aws.ToString(shard.ParentShardId),
		AdjacentParentID: aws.ToString(shard.AdjacentParentShardId),
	}
	if shard.HashKeyRange != nil {
		converted.StartHashKey = aws.ToString(shard.HashKeyRange.StartingHashKey)
		converted.EndHashKey = aws.ToString(shard.HashKeyRange.EndingHashKey)
	}
	if shard.SequenceNumberRange != nil {
		converted.StartSequence = aws.ToString(shard.SequenceNumberRange.StartingSequenceNumber)
		converted.EndSequence = aws.ToString(shard.SequenceNumberRange.EndingSequenceNumber)
		converted.Closed = converted.EndSequence != ""
	}
	return converted
}
