package googlepubsub

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pubsub "cloud.google.com/go/pubsub/v2"
)

// maxSendCount caps one console send.
//
// The cap is this driver's, not the service's: a send console is for producing
// a handful by hand rather than for load generation, and every message costs a
// request Google bills for.
const maxSendCount = 1000

// PublishRequest is a send in Pub/Sub's own vocabulary.
//
// Deliberately not MessagePublisher's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary, of which a Pub/Sub message
// has only the topic. There is no tag and no delay anywhere in the service:
// what a message carries beside its body is a table of string attributes, and
// what decides its ordering is a key rather than a queue.
type PublishRequest struct {
	Topic string
	Body  string

	// Count sends the same body more than once, for filling a subscription to
	// watch a consumer work through it.
	Count int

	// Attributes are the publisher's own, sent as Pub/Sub message attributes.
	// They are also the only thing a subscription filter can select on, so a
	// send meant for a filtered subscription has to set them.
	Attributes map[string]string

	// OrderingKey groups messages that must be delivered in order relative to
	// each other. It only has an effect on a subscription created with message
	// ordering on; elsewhere it is carried and ignored.
	OrderingKey string
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
 * The one thing worth knowing before pressing the button is where the message
 * goes, which is not "the topic". A topic stores nothing: the publish is fanned
 * out to whatever subscriptions exist at that instant and discarded if none
 * do, and the service reports success either way. So a send to a topic nothing
 * subscribes to is accepted, acknowledged and thrown away - which is why the
 * topics board marks that state and this call reports how many subscriptions
 * were attached when it ran.
 */
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	topic, err := requiredName("topic", request.Topic)
	if err != nil {
		return nil, err
	}
	// Pub/Sub refuses a message with neither data nor attributes, and its own
	// refusal names no field at all.
	if request.Body == "" && len(request.Attributes) == 0 {
		return nil, errors.New(
			"a pub/sub message needs a body or at least one attribute; this one has neither")
	}

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d messages at once", maxSendCount)
	}

	publisher := c.client.Publisher(c.topicPath(topic))
	// The client refuses an ordering key unless the publisher was told to
	// expect one, and the refusal names an internal flag rather than the field
	// the console draws.
	ordering := strings.TrimSpace(request.OrderingKey)
	publisher.EnableMessageOrdering = ordering != ""
	defer publisher.Stop()

	attributes := attributesOf(request.Attributes)
	result := &PublishResult{}
	// One at a time rather than fired together: the client batches for itself,
	// and a partial send has to report how many were accepted before it
	// stopped rather than an unordered pile of futures.
	for index := 0; index < count; index++ {
		pending := publisher.Publish(ctx, &pubsub.Message{
			Data:        []byte(request.Body),
			Attributes:  attributes,
			OrderingKey: ordering,
		})
		id, err := pending.Get(ctx)
		if err != nil {
			if notFound(err) {
				return result, fmt.Errorf("no topic named %q in %s", topic, c.config.project)
			}
			return result, fmt.Errorf("%s took %d of %d before failing: %w",
				topic, result.Sent, count, err)
		}
		result.Sent++
		if result.MessageID == "" {
			result.MessageID = id
		}
	}
	return result, nil
}

// attributesOf turns the console's key/value table into message attributes.
//
// Strings only, which is all Pub/Sub carries: there is no typed attribute
// anywhere in the service, so nothing has to be guessed at.
func attributesOf(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	built := make(map[string]string, len(attributes))
	for name, value := range attributes {
		if strings.TrimSpace(name) == "" {
			continue
		}
		built[name] = value
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
 *   - tags is refused. A Pub/Sub message has a body and a table of named
 *     attributes and nothing else, so a tag would be silently discarded and
 *     the send reported as having carried it.
 *   - keys carries the ordering key, which is the nearest thing Pub/Sub has to
 *     what a RocketMQ key is for: the value a reader groups by.
 *   - delayLevel is refused outright rather than converted. RocketMQ's levels
 *     are an index into a broker-side table and SQS takes real seconds;
 *     Pub/Sub has neither. A message is delivered as soon as there is a
 *     subscription to deliver it to, and a console that accepted a delay would
 *     report holding a message back that went out immediately.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	if strings.TrimSpace(tags) != "" {
		return "", errors.New(
			"a pub/sub message has a body and named attributes and no tag; send it as an attribute")
	}
	if delayLevel != 0 {
		return "", errors.New(
			"pub/sub cannot hold a message back; it is delivered as soon as a subscription " +
				"exists to deliver it to")
	}
	result, err := c.Publish(ctx, PublishRequest{
		Topic:       topic,
		Body:        body,
		Count:       1,
		OrderingKey: keys,
	})
	if result == nil {
		return "", err
	}
	return result.MessageID, err
}
