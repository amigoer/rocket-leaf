package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Creating, reconfiguring and deleting queues and topics.
 *
 * One entity kind decides which half of the API is called, and there is no way
 * to change it afterwards: a queue and a topic are different objects, so
 * turning one into the other means deleting it and losing whatever it held.
 * The spec carries the kind and this refuses an update that changes it, rather
 * than quietly creating a second entity beside the first.
 *
 * Three settings on a queue are fixed at creation and the service refuses them
 * in an update: sessions, duplicate detection and partitioning. They are sent
 * on a create and left out of an update, because the service's own refusal
 * names a proto field rather than the form row.
 */

// Bounds the service enforces, checked here so the message can name the field
// the form draws rather than the one the API answers with.
const (
	minLockDurationSec = 5
	maxLockDurationSec = 300
	minDeliveryCount   = 1
	maxDeliveryCount   = 2000
)

/*
 * CreateDestination declares a queue or a topic.
 *
 * Only what the spec carries is sent. Everything else takes the service's own
 * default, which for the settings that matter is a one-minute lock, ten
 * delivery attempts and a time to live of fourteen days.
 */
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("entity", spec.Ref.Name)
	if err != nil {
		return err
	}

	switch entityKind(spec.Attributes) {
	case EntityTopic:
		properties := &admin.TopicProperties{}
		if err := applyTopicSettings(properties, spec.Attributes, true); err != nil {
			return err
		}
		_, err = c.management.CreateTopic(ctx, name, &admin.CreateTopicOptions{Properties: properties})
	default:
		properties := &admin.QueueProperties{}
		if err := applyQueueSettings(properties, spec.Attributes, true); err != nil {
			return err
		}
		_, err = c.management.CreateQueue(ctx, name, &admin.CreateQueueOptions{Properties: properties})
	}

	if alreadyExists(err) {
		// The service's own message is "The messaging entity ... already
		// exists" with an internal tracking id attached, which is unhelpful in
		// a namespace where several teams create entities from a script. It is
		// also the answer when the name is taken by the *other* kind, which is
		// worth saying: queues and topics share one name space.
		return fmt.Errorf("a queue or topic named %q already exists in %s", name, c.config.namespace)
	}
	return err
}

/*
 * UpdateDestination changes an entity's settings.
 *
 * The whole object is sent, not a patch: the management API replaces an
 * entity's description with the document it is given, so the current one is
 * read first and the spec's values written over it. An attribute the spec
 * leaves out therefore keeps its stored value, which is what an edit form
 * expects - and is only true because of that read.
 */
func (c *Conn) UpdateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("entity", spec.Ref.Name)
	if err != nil {
		return err
	}

	// The kind is discovered rather than taken from the spec, because the two
	// halves of the API share one Atom path: sending a QueueDescription to a
	// topic's path would replace the topic with a queue and take every
	// subscription on it with the old object.
	described, err := c.describeEntity(ctx, name)
	if err != nil {
		return err
	}
	if wanted := entityKind(spec.Attributes); described.kind() != "" && wanted != described.kind() {
		return fmt.Errorf(
			"%q is a %s and cannot be turned into a %s; delete it and create the other, "+
				"which loses whatever it holds", name, described.kind(), wanted)
	}

	switch {
	case described.topic != nil:
		if err := applyTopicSettings(described.topic, spec.Attributes, false); err != nil {
			return err
		}
		_, err = c.management.UpdateTopic(ctx, name, *described.topic, nil)
		return err
	case described.queue != nil:
		if err := applyQueueSettings(described.queue, spec.Attributes, false); err != nil {
			return err
		}
		_, err = c.management.UpdateQueue(ctx, name, *described.queue, nil)
		return err
	}
	return fmt.Errorf("no queue or topic named %q in %s", name, c.config.namespace)
}

/*
 * RemoveDestination deletes a queue or a topic.
 *
 * Everything it holds goes with it, dead letters included: the
 * $DeadLetterQueue is a sub-entity of the thing being deleted rather than an
 * ordinary queue that survives it. Deleting a topic also deletes every
 * subscription on it, and with them whatever those had not delivered - which
 * is the opposite of Pub/Sub, where a subscription outlives its topic.
 *
 * The kind is discovered rather than taken from the caller, because a delete
 * confirmed by name should not depend on the page having remembered which of
 * the two it was - and because the wrong call would still succeed: both delete
 * the same Atom path, so DeleteQueue on a topic removes the topic.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("entity", ref.Name)
	if err != nil {
		return err
	}

	described, err := c.describeEntity(ctx, name)
	if err != nil {
		return err
	}
	switch described.kind() {
	case EntityTopic:
		_, err = c.management.DeleteTopic(ctx, name, nil)
	case EntityQueue:
		_, err = c.management.DeleteQueue(ctx, name, nil)
	default:
		return fmt.Errorf("no queue or topic named %q in %s", name, c.config.namespace)
	}
	if notFound(err) {
		// It went between the describe and the delete, which is what a second
		// operator clicking the same button looks like.
		return fmt.Errorf("no queue or topic named %q in %s", name, c.config.namespace)
	}
	return err
}

// entityKind reads which of the two an operation is about. A spec that says
// nothing means a queue, which is the ordinary case and the one a form that
// forgot the field would have meant.
func entityKind(attributes map[string]string) string {
	if strings.TrimSpace(attributes[AttrEntityType]) == EntityTopic {
		return EntityTopic
	}
	return EntityQueue
}

// applyQueueSettings writes the spec's values onto a queue description.
//
// creating decides the three settings the service refuses to change: sessions,
// duplicate detection and partitioning are fixed once the queue exists, so
// they are sent on a create and silently left out of an update rather than
// producing a failure the form cannot act on.
func applyQueueSettings(properties *admin.QueueProperties, attributes map[string]string, creating bool) error {
	if seconds, given, err := boundedSeconds(attributes, AttrLockDurationSec,
		"lock duration", minLockDurationSec, maxLockDurationSec); err != nil {
		return err
	} else if given {
		properties.LockDuration = pointer(isoDuration(seconds))
	}

	if seconds, given, err := positiveSeconds(attributes, AttrTTLSec, "time to live"); err != nil {
		return err
	} else if given {
		properties.DefaultMessageTimeToLive = pointer(isoDuration(seconds))
	}

	if seconds, given, err := positiveSeconds(attributes, AttrAutoDeleteOnIdleSec,
		"auto-delete on idle"); err != nil {
		return err
	} else if given {
		properties.AutoDeleteOnIdle = pointer(isoDuration(seconds))
	}

	if raw := strings.TrimSpace(attributes[AttrMaxDeliveryCount]); raw != "" {
		count, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the delivery limit has to be a number, not %q", raw)
		}
		if count < minDeliveryCount || count > maxDeliveryCount {
			return fmt.Errorf(
				"service bus dead-letters after between %d and %d delivery attempts; %d is outside that",
				minDeliveryCount, maxDeliveryCount, count)
		}
		properties.MaxDeliveryCount = pointer(int32(count))
	}

	if raw := strings.TrimSpace(attributes[AttrMaxSizeMB]); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the maximum size has to be a number of megabytes, not %q", raw)
		}
		properties.MaxSizeInMegabytes = pointer(int32(size))
	}

	if raw := strings.TrimSpace(attributes[AttrDeadLetterOnExpiry]); raw != "" {
		properties.DeadLetteringOnMessageExpiration = pointer(raw == "true")
	}
	// Forwarding is written whenever the spec mentions it at all, empty
	// included: an empty value is how it is removed, and the two cases have to
	// stay distinguishable from an attribute the spec never carried.
	if value, given := attributes[AttrForwardTo]; given {
		properties.ForwardTo = pointer(strings.TrimSpace(value))
	}
	if value, given := attributes[AttrForwardDeadLettersTo]; given {
		properties.ForwardDeadLetteredMessagesTo = pointer(strings.TrimSpace(value))
	}

	if !creating {
		return nil
	}
	if raw := strings.TrimSpace(attributes[AttrRequiresSession]); raw != "" {
		properties.RequiresSession = pointer(raw == "true")
	}
	if raw := strings.TrimSpace(attributes[AttrRequiresDuplicateDetect]); raw != "" {
		properties.RequiresDuplicateDetection = pointer(raw == "true")
	}
	if raw := strings.TrimSpace(attributes[AttrPartitioned]); raw != "" {
		properties.EnablePartitioning = pointer(raw == "true")
	}
	return nil
}

// applyTopicSettings writes the spec's values onto a topic description.
//
// Far fewer of them, and that is the family rather than a gap here: a topic
// holds no messages, so nothing about delivery belongs to it. The lock, the
// delivery limit and where dead letters go are all the subscription's.
func applyTopicSettings(properties *admin.TopicProperties, attributes map[string]string, creating bool) error {
	if seconds, given, err := positiveSeconds(attributes, AttrTTLSec, "time to live"); err != nil {
		return err
	} else if given {
		properties.DefaultMessageTimeToLive = pointer(isoDuration(seconds))
	}
	if seconds, given, err := positiveSeconds(attributes, AttrAutoDeleteOnIdleSec,
		"auto-delete on idle"); err != nil {
		return err
	} else if given {
		properties.AutoDeleteOnIdle = pointer(isoDuration(seconds))
	}
	if raw := strings.TrimSpace(attributes[AttrMaxSizeMB]); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("the maximum size has to be a number of megabytes, not %q", raw)
		}
		properties.MaxSizeInMegabytes = pointer(int32(size))
	}

	if !creating {
		return nil
	}
	if raw := strings.TrimSpace(attributes[AttrRequiresDuplicateDetect]); raw != "" {
		properties.RequiresDuplicateDetection = pointer(raw == "true")
	}
	if raw := strings.TrimSpace(attributes[AttrPartitioned]); raw != "" {
		properties.EnablePartitioning = pointer(raw == "true")
	}
	return nil
}

// boundedSeconds reads a timespan and refuses one the service would.
func boundedSeconds(attributes map[string]string, key, label string, low, high int) (int, bool, error) {
	seconds, given, err := positiveSeconds(attributes, key, label)
	if err != nil || !given {
		return 0, given, err
	}
	if seconds < low || seconds > high {
		return 0, false, fmt.Errorf(
			"service bus holds a %s between %ds and %ds; %ds is outside that", label, low, high, seconds)
	}
	return seconds, true, nil
}

func positiveSeconds(attributes map[string]string, key, label string) (int, bool, error) {
	raw := strings.TrimSpace(attributes[key])
	if raw == "" {
		return 0, false, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("the %s has to be a number of seconds, not %q", label, raw)
	}
	if seconds <= 0 {
		return 0, false, errors.New("the " + label + " has to be more than zero seconds")
	}
	return seconds, true, nil
}

func pointer[T any](value T) *T { return &value }
