package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Moving where a subscription reads, which Pub/Sub spells two ways.
 *
 * Seek takes either a timestamp or a snapshot, and the two are different
 * gestures with different guarantees - which is why they land on two different
 * ports here rather than one.
 *
 * A timestamp names a moment and lets the service work out where that falls.
 * That is ProgressAdmin. The emulator serves it, with one hole: a subscription
 * created with message ordering on gets Unimplemented, which is that
 * subscription's setting rather than anything about the endpoint - so it is
 * reported at the call, where the message can name the setting, rather than by
 * taking the capability off the whole connection.
 *
 * A snapshot names the place itself. It is a copy of one subscription's
 * acknowledgement state taken at a moment, it has a name, and seeking to it
 * restores exactly that place - so it is StreamPositionAdmin's shape, where
 * the caller already holds the position rather than describing it. The
 * emulator serves this one.
 *
 * Neither can reach further back than the subscription's retention, and
 * neither can un-acknowledge a message unless the subscription was created
 * with retain_acked_messages on. That is the service's rule and the boards say
 * so; nothing here pretends otherwise.
 */

/*
 * ResetOffset moves a subscription to a moment in time.
 *
 * ResetOffsetRequest is RocketMQ's shape and two of its four fields do not
 * apply: Topic is ignored because a subscription reads exactly one and it was
 * chosen at creation, and Force has no counterpart - Pub/Sub takes no account
 * of whether anything is attached, so there is no unforced variant to refuse.
 */
func (c *Conn) ResetOffset(ctx context.Context, request model.ResetOffsetRequest) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("subscription", request.Group)
	if err != nil {
		return err
	}
	if request.Timestamp <= 0 {
		return errors.New("a reset names the moment to read from, and this one names none")
	}

	moment := time.UnixMilli(request.Timestamp)
	_, err = c.client.SubscriptionAdminClient.Seek(ctx, &pubsubpb.SeekRequest{
		Subscription: c.subscriptionPath(name),
		Target:       &pubsubpb.SeekRequest_Time{Time: timestamppb.New(moment)},
	})
	switch {
	case notFound(err):
		return fmt.Errorf("no subscription named %q in %s", name, c.config.project)
	case unimplemented(err):
		// The emulator's one hole, and it is narrow: seeking an ordered
		// subscription to a timestamp. The service's own message is the bare
		// word "Unimplemented", which says nothing about which of the two
		// things it would not do.
		return fmt.Errorf(
			"%s will not seek %q to a moment in time; it is a subscription with message "+
				"ordering on, and seeking to a snapshot is what works here instead: %w",
			c.endpointName(), name, err)
	}
	return err
}

/*
 * SetSubscriptionPosition moves a subscription to a named snapshot.
 *
 * PositionRequest's Position is an entry id on the family this port was
 * written for. Here it is a snapshot name, which is the same idea: the caller
 * is not describing a moment and asking the service to find it, but naming a
 * place it already holds.
 */
func (c *Conn) SetSubscriptionPosition(ctx context.Context, request model.PositionRequest) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("subscription", request.Ref.Name)
	if err != nil {
		return err
	}
	snapshot, err := requiredName("snapshot", request.Position)
	if err != nil {
		return errors.New(
			"a position here is the name of a snapshot; take one first, or reset to a moment instead")
	}

	_, err = c.client.SubscriptionAdminClient.Seek(ctx, &pubsubpb.SeekRequest{
		Subscription: c.subscriptionPath(name),
		Target:       &pubsubpb.SeekRequest_Snapshot{Snapshot: c.snapshotPath(snapshot)},
	})
	if notFound(err) {
		// One of the two is missing and the service does not say which.
		return fmt.Errorf("no subscription %q or snapshot %q in %s: %w",
			name, snapshot, c.config.project, err)
	}
	return err
}

// Snapshot is a restore point taken from one subscription.
//
// It is not a copy of the messages: it is that subscription's acknowledgement
// state at a moment, and the topic goes on holding what it needs to honour it.
// Which is why one has an expiry - seven days from creation - and why leaving
// them lying around costs storage on the topic rather than on the snapshot.
type Snapshot struct {
	Name string
	// Topic is what the subscription it was taken from reads. A snapshot can
	// only be sought to from a subscription on the same topic.
	Topic string
	// ExpiresAt is when the service will delete it, in epoch milliseconds.
	ExpiresAt int64
	Labels    map[string]string
}

// ListSnapshots is every restore point in the project.
func (c *Conn) ListSnapshots(ctx context.Context) ([]*Snapshot, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	listing := c.client.SubscriptionAdminClient.ListSnapshots(ctx,
		&pubsubpb.ListSnapshotsRequest{Project: c.projectPath()})

	snapshots := make([]*Snapshot, 0, 8)
	for {
		snapshot, err := listing.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := shortName(snapshot.GetName())
		if !c.matchesPrefix(name) {
			continue
		}
		snapshots = append(snapshots, &Snapshot{
			Name:      name,
			Topic:     topicNameOf(snapshot.GetTopic()),
			ExpiresAt: snapshot.GetExpireTime().AsTime().UnixMilli(),
			Labels:    snapshot.GetLabels(),
		})
	}
	sort.Slice(snapshots, func(a, b int) bool { return snapshots[a].Name < snapshots[b].Name })
	return snapshots, nil
}

// CreateSnapshot takes a restore point from one subscription.
//
// The snapshot belongs to the topic rather than to the subscription it came
// from: any subscription on the same topic can be sought to it, which is how a
// replay is set up for a second reader without disturbing the first.
func (c *Conn) CreateSnapshot(ctx context.Context, name, subscription string) error {
	if err := c.live(); err != nil {
		return err
	}
	snapshot, err := requiredName("snapshot", name)
	if err != nil {
		return err
	}
	from, err := requiredName("subscription", subscription)
	if err != nil {
		return err
	}

	_, err = c.client.SubscriptionAdminClient.CreateSnapshot(ctx, &pubsubpb.CreateSnapshotRequest{
		Name:         c.snapshotPath(snapshot),
		Subscription: c.subscriptionPath(from),
	})
	switch {
	case alreadyExists(err):
		return fmt.Errorf("a snapshot named %q already exists in %s", snapshot, c.config.project)
	case notFound(err):
		return fmt.Errorf("no subscription named %q in %s to snapshot", from, c.config.project)
	}
	return err
}

// RemoveSnapshot deletes a restore point.
//
// Worth doing rather than waiting for the expiry: while it exists the topic
// keeps every message the snapshot could restore, so an abandoned snapshot is
// storage nobody is paying attention to.
func (c *Conn) RemoveSnapshot(ctx context.Context, name string) error {
	if err := c.live(); err != nil {
		return err
	}
	snapshot, err := requiredName("snapshot", name)
	if err != nil {
		return err
	}
	err = c.client.SubscriptionAdminClient.DeleteSnapshot(ctx, &pubsubpb.DeleteSnapshotRequest{
		Snapshot: c.snapshotPath(snapshot),
	})
	if notFound(err) {
		return fmt.Errorf("no snapshot named %q in %s", snapshot, c.config.project)
	}
	return err
}

func (c *Conn) snapshotPath(name string) string { return c.projectPath() + "/snapshots/" + name }

// endpointName is what an error calls whatever answered, for the one message
// that has to distinguish an emulator from the service.
func (c *Conn) endpointName() string {
	if c.config.emulator != "" {
		return "the pub/sub emulator at " + strings.TrimSpace(c.config.emulator)
	}
	return "pub/sub"
}
