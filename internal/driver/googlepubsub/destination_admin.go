package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating, reconfiguring and deleting topics.
//
// There is very little on a topic to configure, and that is the family rather
// than a gap here: everything about delivery - the ack deadline, the retry
// policy, where dead letters go, what is filtered out - belongs to the
// subscription. What a topic owns is how long a published message stays
// available for a subscription to seek back into, and a set of labels.

// Retention bounds Pub/Sub enforces on a topic. Ten minutes to
// thirty-one days, and the service refuses anything outside with
// INVALID_ARGUMENT naming the field rather than the value.
const (
	minTopicRetention = 10 * time.Minute
	maxTopicRetention = 31 * 24 * time.Hour
)

/*
 * CreateDestination declares a topic.
 *
 * Almost nothing is settable, so almost nothing is sent: a topic with no
 * retention keeps published messages only until every subscription has
 * acknowledged them, which is the default and what most topics want.
 */
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("topic", spec.Ref.Name)
	if err != nil {
		return err
	}

	topic := &pubsubpb.Topic{Name: c.topicPath(name), Labels: labelsOf(spec.Attributes)}
	retention, given, err := retentionOf(spec.Attributes)
	if err != nil {
		return err
	}
	if given {
		topic.MessageRetentionDuration = durationpb.New(retention)
	}

	_, err = c.client.TopicAdminClient.CreateTopic(ctx, topic)
	if alreadyExists(err) {
		// The service's own message is "Topic already exists" with no name in
		// it, which is unhelpful in a project where several teams create
		// topics from a script.
		return fmt.Errorf("a topic named %q already exists in %s", name, c.config.project)
	}
	return err
}

/*
 * UpdateDestination changes a topic's settings.
 *
 * Only what the spec carries is sent, and the update mask is built from that:
 * UpdateTopic replaces exactly the fields the mask names and leaves the rest
 * alone, so an omitted key means "keep it" rather than "reset it". The one
 * consequence worth knowing is that labels are replaced wholesale - a form
 * editing them has to send the complete set, because sending one label with
 * the mask removes the others.
 *
 * The emulator serves this call for retention and refuses it for labels,
 * answering INVALID_ARGUMENT and saying labels is not a known Topic field.
 * That is the emulator rather than the API, and the live test pins it so the
 * gap is recorded rather than discovered by a user.
 */
func (c *Conn) UpdateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("topic", spec.Ref.Name)
	if err != nil {
		return err
	}

	topic := &pubsubpb.Topic{Name: c.topicPath(name)}
	var paths []string

	retention, given, err := retentionOf(spec.Attributes)
	if err != nil {
		return err
	}
	if given {
		topic.MessageRetentionDuration = durationpb.New(retention)
		paths = append(paths, "message_retention_duration")
	}
	if labels := labelsOf(spec.Attributes); labels != nil {
		topic.Labels = labels
		paths = append(paths, "labels")
	}
	if len(paths) == 0 {
		return errors.New("nothing to change")
	}

	_, err = c.client.TopicAdminClient.UpdateTopic(ctx, &pubsubpb.UpdateTopicRequest{
		Topic:      topic,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
	})
	if notFound(err) {
		return fmt.Errorf("no topic named %q in %s", name, c.config.project)
	}
	return err
}

/*
 * RemoveDestination deletes a topic.
 *
 * What it does not delete is the subscriptions on it. They survive, keep
 * whatever they had not delivered, and report their topic as _deleted-topic_
 * from then on - which is a subscription that can never receive another
 * message and goes on being billed for what it holds. The subscriptions board
 * is where those show up; this call does not clean them up, because deleting
 * somebody else's subscription is not what "delete this topic" asked for.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("topic", ref.Name)
	if err != nil {
		return err
	}
	err = c.client.TopicAdminClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{
		Topic: c.topicPath(name),
	})
	if notFound(err) {
		return fmt.Errorf("no topic named %q in %s", name, c.config.project)
	}
	return err
}

// retentionOf reads the retention a spec asks for.
//
// The bounds are checked here rather than left to the service, which answers
// INVALID_ARGUMENT naming message_retention_duration - a field the form does
// not draw under that name and whose limits it does not state.
func retentionOf(attributes map[string]string) (time.Duration, bool, error) {
	raw := strings.TrimSpace(attributes[AttrRetentionSec])
	if raw == "" {
		return 0, false, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("the retention has to be a number of seconds, not %q", raw)
	}
	retention := time.Duration(seconds) * time.Second
	if retention < minTopicRetention || retention > maxTopicRetention {
		return 0, false, fmt.Errorf(
			"pub/sub keeps a topic's messages for between %v and %v; %v is outside that",
			minTopicRetention, maxTopicRetention, retention)
	}
	return retention, true, nil
}

// labelsOf collects the label attributes off a spec.
//
// Nil rather than an empty map when the spec carries none, because the two
// mean different things to an update: nil leaves labels alone and an empty map
// removes every one of them.
func labelsOf(attributes map[string]string) map[string]string {
	var labels map[string]string
	for key, value := range attributes {
		if !strings.HasPrefix(key, AttrLabelPrefix) {
			continue
		}
		if labels == nil {
			labels = map[string]string{}
		}
		labels[strings.TrimPrefix(key, AttrLabelPrefix)] = value
	}
	return labels
}
