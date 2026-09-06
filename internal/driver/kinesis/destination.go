package kinesis

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Attribute keys this driver puts on a destination.
//
// A contract between this package and frontend/src/mq/kinesis/destinations.ts,
// not part of the shared vocabulary.
const (
	AttrARN = "arn"
	// AttrStatus is CREATING, DELETING, ACTIVE or UPDATING. Every call that
	// names a stream is refused while it is not ACTIVE, so it is the first
	// thing to look at when an operation fails for no visible reason.
	AttrStatus = "status"
	// AttrMode is PROVISIONED or ON_DEMAND. It decides whether the shard
	// count is the operator's to set at all.
	AttrMode           = "streamMode"
	AttrRetentionHours = "retentionHours"
	AttrOpenShards     = "openShards"
	AttrConsumers      = "consumers"
	AttrCreatedAt      = "createdAt"
	// AttrEncryption is NONE or KMS. Which key is a KMS question this
	// connection is not signed to answer, so the key id is carried as given.
	AttrEncryption = "encryption"
	AttrKeyID      = "kmsKeyId"
	// AttrShardMetrics is which per-shard metrics are being published to
	// CloudWatch, comma separated. Empty is the default, and it is the reason
	// a shard's own throughput cannot be read anywhere in this app.
	AttrShardMetrics = "shardLevelMetrics"
)

// listPageSize is how many stream names one ListStreams call asks for. 10000
// is the service's own maximum, and paging is the only way past it.
const listPageSize = 10000

// summaryFanOut caps how many DescribeStreamSummary calls run at once.
//
// ListStreams answers with names, so every figure on the board is a second
// request per stream. Unbounded that is a thousand requests fired together at
// an API that throttles per account, and the throttling would arrive as a
// failed listing rather than a slow one.
const summaryFanOut = 16

/*
 * ListDestinations enumerates the region's streams and describes each one.
 *
 * Two calls per board rather than one, and the reason is not the same as SQS's.
 * ListStreams on a current API version does return a summary per stream - but
 * it carries the name, the ARN, the status, the mode and the creation time,
 * and a row here also shows the retention, the open shard count, the consumer
 * count and the encryption, none of which are on it. So the fan-out is not an
 * optimisation that was skipped; there is no listing that answers the board.
 *
 * The fan-out is bounded and its failures are tolerated per stream: a listing
 * that races a delete drops the row rather than failing the board, which is
 * the ordinary case in an account several teams share.
 */
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	// The filter's IncludeInternal has nothing to hide. Kinesis has no streams
	// of its own: every name in the listing is one somebody created.
	if err := c.live(); err != nil {
		return nil, err
	}

	names, err := c.listStreamNames(ctx)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, len(names))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(summaryFanOut)
	for index, name := range names {
		group.Go(func() error {
			summary, err := c.streamSummary(groupCtx, name)
			if err != nil {
				// A stream that has gone since the listing is not an error the
				// board should show. Anything else is, because a throttled or
				// forbidden read would otherwise be a row silently missing.
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", name, err)
			}
			destinations[index] = destinationOf(summary)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	kept := make([]*model.Destination, 0, len(destinations))
	for _, destination := range destinations {
		if destination != nil {
			c.rememberARN(destination.Ref.Name, destination.Attribute(AttrARN))
			kept = append(kept, destination)
		}
	}
	return kept, nil
}

// DestinationDetail reads one stream.
//
// Not a walk of the listing: DescribeStreamSummary takes one stream, so an
// account with a thousand of them should not answer for all of them to
// describe one.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	summary, err := c.streamSummary(ctx, strings.TrimSpace(ref.Name))
	if err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("no stream named %q in %s", ref.Name, c.config.region)
		}
		return nil, err
	}
	destination := destinationOf(summary)
	c.rememberARN(destination.Ref.Name, destination.Attribute(AttrARN))
	return destination, nil
}

/*
 * listStreamNames pages through the region's streams, honouring the prefix.
 *
 * Paged by ExclusiveStartStreamName rather than by NextToken, and that is not
 * a preference. Both are on the request, and the field the service actually
 * fills in on the way back differs by API version and by emulator - the one
 * this suite runs against reports HasMoreStreams and no token at all, so a
 * loop driven by the token would stop after the first page and silently show
 * a truncated account.
 *
 * The prefix is applied here rather than by the service, unlike SQS's:
 * ListStreams has no filter parameter, so the names come back whole and are
 * narrowed afterwards. That is a difference in cost rather than in behaviour,
 * and it is the reason the prefix cannot save a request.
 */
func (c *Conn) listStreamNames(ctx context.Context) ([]string, error) {
	var names []string
	var after *string
	for {
		page, err := c.client.ListStreams(ctx, &awskinesis.ListStreamsInput{
			Limit:                    aws.Int32(listPageSize),
			ExclusiveStartStreamName: after,
		})
		if err != nil {
			return nil, err
		}
		for _, name := range page.StreamNames {
			if c.config.prefix == "" || strings.HasPrefix(name, c.config.prefix) {
				names = append(names, name)
			}
		}
		if len(page.StreamNames) == 0 || !aws.ToBool(page.HasMoreStreams) {
			break
		}
		after = aws.String(page.StreamNames[len(page.StreamNames)-1])
	}
	sort.Strings(names)
	return names, nil
}

// streamSummary describes one stream.
func (c *Conn) streamSummary(ctx context.Context, name string) (*types.StreamDescriptionSummary, error) {
	out, err := c.client.DescribeStreamSummary(ctx, &awskinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	return out.StreamDescriptionSummary, nil
}

/*
 * destinationOf turns one stream's summary into a canonical destination.
 *
 * Partitions carries the open shard count, and that is a deliberate reading of
 * a field the canonical model gave a narrower name. A stream really is divided
 * into N shards that are open for writing, so the count is true and it is the
 * figure a listing wants. What it cannot say is anything that makes a shard a
 * shard - its id, the slice of the hash space it owns, its own read quota, and
 * the parent it was split from - so none of that is squeezed in here. It has a
 * port and a page of its own, and this driver does not declare CapPartitions,
 * whose page is built around a partition number.
 *
 * The count excludes shards closed by a split or a merge. Those still exist
 * and still hold their records until retention expires, so they are not
 * nothing - but they take no writes, and adding them here would make a stream
 * look permanently wider than it is every time it was resized.
 *
 * Subscribers is the registered consumer count, which is every reader the
 * stream itself knows about. A classic consumer registers nothing and keeps
 * its position in a DynamoDB table this connection never sees, so a stream
 * being read hard by three applications can report zero here. That is the
 * service's own answer rather than a gap in this driver, and the consumers
 * page says so.
 */
func destinationOf(summary *types.StreamDescriptionSummary) *model.Destination {
	name := aws.ToString(summary.StreamName)
	collected := map[string]string{
		AttrARN:            aws.ToString(summary.StreamARN),
		AttrStatus:         string(summary.StreamStatus),
		AttrRetentionHours: strconv.Itoa(int(aws.ToInt32(summary.RetentionPeriodHours))),
		AttrOpenShards:     strconv.Itoa(int(aws.ToInt32(summary.OpenShardCount))),
		AttrConsumers:      strconv.Itoa(int(aws.ToInt32(summary.ConsumerCount))),
		AttrEncryption:     string(summary.EncryptionType),
	}
	if summary.StreamModeDetails != nil {
		collected[AttrMode] = string(summary.StreamModeDetails.StreamMode)
	}
	if key := aws.ToString(summary.KeyId); key != "" {
		collected[AttrKeyID] = key
	}
	if summary.StreamCreationTimestamp != nil {
		collected[AttrCreatedAt] = strconv.FormatInt(
			summary.StreamCreationTimestamp.UTC().UnixMilli(), 10)
	}
	if metrics := shardMetricsOf(summary.EnhancedMonitoring); metrics != "" {
		collected[AttrShardMetrics] = metrics
	}

	return &model.Destination{
		Ref:         model.DestinationRef{Name: name},
		Partitions:  int(aws.ToInt32(summary.OpenShardCount)),
		Subscribers: int(aws.ToInt32(summary.ConsumerCount)),
		// Nothing reports how many records a stream is holding. Kinesis bills
		// by shard hour and payload, keeps no count of what is stored, and the
		// only way to produce one would be to read every shard end to end -
		// which is the browse, and it would spend the read quota of every
		// shard on the board to show a figure that was stale on arrival.
		Depth: model.UnknownMetric,
		// No rates either. Kinesis publishes IncomingRecords and friends to
		// CloudWatch, which is a different service under a different
		// permission, and two samples taken here would be this app's
		// arithmetic presented as AWS's.
		RateIn:      model.UnknownMetric,
		RateOut:     model.UnknownMetric,
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
		Attributes:  collected,
	}
}

// shardMetricsOf flattens the per-shard CloudWatch metrics a stream publishes.
//
// The API nests them in a list of one entry, which is a shape rather than a
// meaning: there is one set per stream.
func shardMetricsOf(monitoring []types.EnhancedMetrics) string {
	var names []string
	for _, entry := range monitoring {
		for _, metric := range entry.ShardLevelMetrics {
			names = append(names, string(metric))
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
