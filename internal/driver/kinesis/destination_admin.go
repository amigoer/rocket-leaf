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

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating, reconfiguring and deleting streams.
//
// Nothing here is one call. A stream's capacity, its retention and its mode
// are three separate operations on the service, each asynchronous, and each
// refused while the stream is not ACTIVE - so an update that changes two
// settings has to make the second call after the first one has settled. That
// is why this file is longer than the create it starts with.

// retentionBounds are what the service accepts, in hours: a day at the least,
// a year at the most. Checked here so the message can name the field rather
// than arriving as InvalidArgumentException.
const (
	minRetentionHours = 24
	maxRetentionHours = 8760
)

/*
 * CreateDestination declares a stream.
 *
 * The shard count and the stream mode are one decision rather than two: an
 * on-demand stream's capacity is the service's to choose, and CreateStream
 * refuses a shard count alongside ON_DEMAND. So the mode decides which of the
 * two is sent, and a spec asking for both is refused here - where the message
 * can name what the user filled in.
 */
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(spec.Ref.Name)
	if name == "" {
		return errors.New("a stream needs a name")
	}

	input := &awskinesis.CreateStreamInput{StreamName: aws.String(name)}
	if onDemand(spec.Attributes[AttrMode]) {
		input.StreamModeDetails = &types.StreamModeDetails{StreamMode: types.StreamModeOnDemand}
		if spec.Partitions > 0 {
			return errors.New(
				"an on-demand stream has no shard count to set; AWS scales it, and a " +
					"provisioned stream is what takes a number")
		}
	} else {
		if spec.Partitions <= 0 {
			return errors.New("a provisioned stream needs at least one shard")
		}
		input.ShardCount = aws.Int32(int32(spec.Partitions))
		input.StreamModeDetails = &types.StreamModeDetails{StreamMode: types.StreamModeProvisioned}
	}

	if _, err := c.client.CreateStream(ctx, input); err != nil {
		var exists *types.ResourceInUseException
		if errors.As(err, &exists) {
			return fmt.Errorf("a stream named %q already exists in %s: %w", name, c.config.region, err)
		}
		return err
	}

	// The retention is not on CreateStream at all: a new stream keeps 24 hours
	// and is changed afterwards, by a call that is refused until it is ACTIVE.
	hours, given, err := retentionOf(spec.Attributes)
	if err != nil {
		return err
	}
	if !given || hours == minRetentionHours {
		return nil
	}
	if err := c.awaitActive(ctx, name); err != nil {
		return err
	}
	return c.setRetention(ctx, name, hours, minRetentionHours)
}

/*
 * UpdateDestination changes a stream's capacity, retention or mode.
 *
 * Three operations, applied in the order that cannot contradict itself: the
 * mode first, because switching to on demand takes the shard count out of the
 * operator's hands entirely; then the capacity, which only a provisioned
 * stream has; then the retention, which is the one setting both modes share.
 * Each waits for the stream to come back to ACTIVE, because the next call is
 * refused while it is not.
 *
 * What is deliberately not here is encryption. A stream is encrypted with a
 * KMS key, choosing one is a KMS decision, and this connection is signed for
 * Kinesis alone - so the pages show what a stream is using and do not offer
 * to change it.
 */
func (c *Conn) UpdateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(spec.Ref.Name)
	if name == "" {
		return errors.New("a stream needs a name")
	}
	summary, err := c.streamSummary(ctx, name)
	if err != nil {
		if notFound(err) {
			return fmt.Errorf("no stream named %q in %s", name, c.config.region)
		}
		return err
	}

	current := types.StreamModeProvisioned
	if summary.StreamModeDetails != nil {
		current = summary.StreamModeDetails.StreamMode
	}
	wanted := current
	if mode, set := spec.Attributes[AttrMode]; set && strings.TrimSpace(mode) != "" {
		if onDemand(mode) {
			wanted = types.StreamModeOnDemand
		} else {
			wanted = types.StreamModeProvisioned
		}
	}

	changed := false
	if wanted != current {
		if _, err := c.client.UpdateStreamMode(ctx, &awskinesis.UpdateStreamModeInput{
			StreamARN:         summary.StreamARN,
			StreamModeDetails: &types.StreamModeDetails{StreamMode: wanted},
		}); err != nil {
			return err
		}
		if err := c.awaitActive(ctx, name); err != nil {
			return err
		}
		changed = true
	}

	open := int(aws.ToInt32(summary.OpenShardCount))
	if spec.Partitions > 0 && spec.Partitions != open {
		if wanted == types.StreamModeOnDemand {
			return errors.New(
				"an on-demand stream's shard count is the service's to choose; " +
					"switch it to provisioned to set one")
		}
		if _, err := c.client.UpdateShardCount(ctx, &awskinesis.UpdateShardCountInput{
			StreamName:       aws.String(name),
			TargetShardCount: aws.Int32(int32(spec.Partitions)),
			ScalingType:      types.ScalingTypeUniformScaling,
		}); err != nil {
			return fmt.Errorf("resizing %s from %d shards to %d: %w", name, open, spec.Partitions, err)
		}
		if err := c.awaitActive(ctx, name); err != nil {
			return err
		}
		changed = true
	}

	hours, given, err := retentionOf(spec.Attributes)
	if err != nil {
		return err
	}
	stored := int(aws.ToInt32(summary.RetentionPeriodHours))
	if given && hours != stored {
		if err := c.setRetention(ctx, name, hours, stored); err != nil {
			return err
		}
		changed = true
	}

	if !changed {
		return errors.New("nothing to change")
	}
	return nil
}

/*
 * RemoveDestination deletes a stream and everything in it.
 *
 * EnforceConsumerDeletion is not sent. A stream with registered consumers
 * refuses the delete, and that refusal is worth keeping: an enhanced fan-out
 * consumer is an application somebody stood up, and deleting it as a side
 * effect of deleting the stream is a bigger action than the one asked for.
 * The error names them, and the consumers page is where they are removed.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return errors.New("a stream needs a name")
	}
	_, err := c.client.DeleteStream(ctx, &awskinesis.DeleteStreamInput{StreamName: aws.String(name)})
	c.forgetARN(name)
	if err != nil {
		if notFound(err) {
			return fmt.Errorf("no stream named %q in %s", name, c.config.region)
		}
		var inUse *types.ResourceInUseException
		if errors.As(err, &inUse) {
			return fmt.Errorf(
				"%s still has registered consumers; deregister them first: %w", name, err)
		}
		return err
	}
	return nil
}

// setRetention moves the retention period, choosing the call by direction.
//
// Two operations rather than one, and they are not interchangeable: the
// service refuses an increase sent to the decrease endpoint and the other way
// round, with a message that names neither the current value nor the wanted
// one.
func (c *Conn) setRetention(ctx context.Context, name string, hours, from int) error {
	if hours > from {
		_, err := c.client.IncreaseStreamRetentionPeriod(ctx,
			&awskinesis.IncreaseStreamRetentionPeriodInput{
				StreamName:           aws.String(name),
				RetentionPeriodHours: aws.Int32(int32(hours)),
			})
		return err
	}
	_, err := c.client.DecreaseStreamRetentionPeriod(ctx,
		&awskinesis.DecreaseStreamRetentionPeriodInput{
			StreamName:           aws.String(name),
			RetentionPeriodHours: aws.Int32(int32(hours)),
		})
	return err
}

// retentionOf reads the wanted retention off a spec, in hours.
//
// Bounded here rather than at the service, because the service's own refusal
// names an argument the form never drew.
func retentionOf(attributes map[string]string) (hours int, given bool, err error) {
	raw := strings.TrimSpace(attributes[AttrRetentionHours])
	if raw == "" {
		return 0, false, nil
	}
	hours, err = strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("the retention has to be a number of hours, not %q", raw)
	}
	if hours < minRetentionHours || hours > maxRetentionHours {
		return 0, false, fmt.Errorf(
			"kinesis keeps a record for between %d and %d hours; %d is outside that",
			minRetentionHours, maxRetentionHours, hours)
	}
	return hours, true, nil
}

// onDemand reads the stream mode a spec asked for.
func onDemand(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), string(types.StreamModeOnDemand))
}
