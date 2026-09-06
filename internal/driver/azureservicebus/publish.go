package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// maxSendCount caps one console send.
//
// The cap is this driver's, not the service's: a send console is for producing
// a handful by hand rather than for load generation, and every message costs
// an operation Azure bills for.
const maxSendCount = 1000

/*
 * SendRequest is a send in Service Bus's own vocabulary.
 *
 * Deliberately not MessagePublisher's shape. That one takes a topic, tags,
 * keys and a delay level - RocketMQ's - and three of the four land differently
 * here. What a Service Bus message carries beside its body is a subject, a
 * correlation id, a session or partition key, a content type, and a table of
 * named application properties.
 *
 * That table and the subject are not decoration on this family: they are what
 * a subscription's rules select on. A SQL filter reads the application
 * properties by name and a correlation filter matches the subject, the
 * correlation id and the rest by equality - so a send meant to reach a
 * filtered subscription has to set them, and a console that could not would
 * make the routing page untestable from the app.
 */
type SendRequest struct {
	// Entity is a queue or a topic. A subscription cannot be sent to: it
	// receives what its topic copies into it.
	Entity string
	Body   string

	// Count sends the same message more than once, for filling an entity to
	// watch a consumer work through it.
	Count int

	// Subject is what Service Bus also calls the label, and what a correlation
	// filter matches under that name.
	Subject       string
	CorrelationID string
	ContentType   string
	// SessionID orders delivery within a session and is required on an entity
	// created with sessions on; PartitionKey only groups. Both are what a
	// reader groups by, which is what a RocketMQ key is for.
	SessionID    string
	PartitionKey string

	// Properties are the sender's own, and the only thing a SQL rule can
	// select on beyond the system fields above.
	Properties map[string]string

	// Delay holds the message back by scheduling it for later. Zero sends it
	// now. Anything else is a real scheduled message: it sits in the entity
	// with a state of its own until its time comes, which the messages page
	// shows and no consumer is offered.
	Delay time.Duration

	// TimeToLive overrides the entity's default for these messages only. Zero
	// leaves the entity's own in place.
	TimeToLive time.Duration
}

/*
 * SendResult is what the send did.
 *
 * There is no message id here, and its absence is the family rather than an
 * omission. Service Bus's MessageId is the sender's own field: nothing
 * assigns one, nothing indexes it, and setting one would turn on duplicate
 * detection's matching on an entity that has it - which is exactly what a
 * console filling a queue with the same body must not do.
 *
 * What does address a message is its sequence number, and the service reports
 * one only for a scheduled send: an immediate message is given its sequence on
 * arrival and the sender is never told. So the count is the whole answer for
 * an ordinary send, and the messages page is where the results are looked at.
 */
type SendResult struct {
	// Sent is how many the service accepted.
	Sent int
	// SequenceNumbers are the scheduled messages' handles, and they do address
	// something: cancelling a scheduled message takes one. Empty on an
	// immediate send.
	SequenceNumbers []int64
}

/*
 * Send publishes one message, or the same one several times.
 *
 * Where it goes depends on which kind of entity was named, and the difference
 * is worth knowing before pressing the button. A queue holds what is sent to
 * it whether or not anything is reading. A topic holds nothing: the message is
 * copied into every subscription whose rules let it through, and discarded on
 * the spot if none do - and the service reports success either way. So a send
 * to a topic with no subscription, or to one whose subscriptions all filter it
 * out, is accepted, acknowledged and thrown away.
 */
func (c *Conn) Send(ctx context.Context, request SendRequest) (*SendResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	entity, err := requiredName("queue or topic", request.Entity)
	if err != nil {
		return nil, err
	}
	// A Service Bus message may have an empty body, unlike Pub/Sub's, so
	// nothing is refused for being empty here.

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d messages at once", maxSendCount)
	}

	sender, err := c.data.NewSender(entity, nil)
	if err != nil {
		return nil, fmt.Errorf("opening a sender on %s: %w", entity, err)
	}
	defer func() { _ = sender.Close(context.WithoutCancel(ctx)) }()

	result := &SendResult{}
	// One at a time rather than fired together: a partial send has to report
	// how many were accepted before it stopped rather than an unordered pile
	// of futures, and a console sends a handful.
	for index := 0; index < count; index++ {
		message := messageOf(request)
		if request.Delay > 0 {
			sequences, err := sender.ScheduleMessages(ctx,
				[]*azservicebus.Message{message}, time.Now().Add(request.Delay), nil)
			if err != nil {
				return result, sendFailure(entity, c.config.namespace, result.Sent, count, err)
			}
			result.SequenceNumbers = append(result.SequenceNumbers, sequences...)
		} else if err := sender.SendMessage(ctx, message, nil); err != nil {
			return result, sendFailure(entity, c.config.namespace, result.Sent, count, err)
		}
		result.Sent++
	}
	return result, nil
}

// messageOf builds one message from the console's fields.
//
// No message id is set, deliberately. It is the sender's own field and
// nothing assigns one, but on an entity with duplicate detection on it is what
// the service matches by - so a console that stamped an id would either drop
// its own repeats silently or have to invent a unique one per message, and
// neither is what "send this five times" asked for.
func messageOf(request SendRequest) *azservicebus.Message {
	message := &azservicebus.Message{Body: []byte(request.Body)}
	if subject := strings.TrimSpace(request.Subject); subject != "" {
		message.Subject = &subject
	}
	if correlation := strings.TrimSpace(request.CorrelationID); correlation != "" {
		message.CorrelationID = &correlation
	}
	if contentType := strings.TrimSpace(request.ContentType); contentType != "" {
		message.ContentType = &contentType
	}
	if session := strings.TrimSpace(request.SessionID); session != "" {
		message.SessionID = &session
	}
	if partition := strings.TrimSpace(request.PartitionKey); partition != "" {
		message.PartitionKey = &partition
	}
	if request.TimeToLive > 0 {
		message.TimeToLive = &request.TimeToLive
	}
	if len(request.Properties) > 0 {
		message.ApplicationProperties = make(map[string]any, len(request.Properties))
		for name, value := range request.Properties {
			if strings.TrimSpace(name) == "" {
				continue
			}
			// Strings only, which is what a console can collect. A SQL rule
			// compares them as strings too unless the value is numeric, and
			// guessing at a type here would change which rules matched.
			message.ApplicationProperties[name] = value
		}
	}
	return message
}

func sendFailure(entity, namespace string, sent, wanted int, err error) error {
	if notFound(err) {
		return fmt.Errorf("no queue or topic named %q in %s", entity, namespace)
	}
	return fmt.Errorf("%s took %d of %d before failing: %w", entity, sent, wanted, err)
}

/*
 * SendMessage publishes through the canonical port.
 *
 * The port is RocketMQ's shape and three of its five arguments land
 * differently here, so each is handled deliberately rather than dropped:
 *
 *   - tags carries the subject, which Service Bus also calls the label. It is
 *     the one place a RocketMQ tag maps exactly: both are a single string on
 *     the message that routing decisions are made from.
 *   - keys carries the session id, which is the nearest thing to what a
 *     RocketMQ key is for - the value a reader groups by - and is the stronger
 *     of the two grouping fields, because it also orders delivery.
 *   - delayLevel is seconds, not a level. RocketMQ's levels are an index into
 *     a broker-side table; Service Bus schedules to a moment and ports.go
 *     fixes no unit. Seconds is what SQS, Pulsar and NSQ chose and what the
 *     console labels.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	if delayLevel < 0 {
		return "", errors.New("a delay cannot be negative")
	}
	_, err := c.Send(ctx, SendRequest{
		Entity:    topic,
		Body:      body,
		Count:     1,
		Subject:   tags,
		SessionID: keys,
		Delay:     time.Duration(delayLevel) * time.Second,
	})
	if err != nil {
		return "", err
	}
	// No id to return, and nothing else to invent one from: Service Bus
	// assigns an immediate message its sequence number on arrival and does not
	// report it to the sender. A scheduled one does come back with a handle,
	// which is what Send's result carries and this port has nowhere to put.
	return "", nil
}

// CancelScheduled unschedules messages that have not been enqueued yet.
//
// It rides on the same capability as the send rather than declaring one of its
// own: scheduling is what produced these sequence numbers, and being able to
// take back what has not gone out is part of what a delayed send means rather
// than a separate thing a page decides to offer.
func (c *Conn) CancelScheduled(ctx context.Context, entity string, sequences []int64) error {
	if err := c.live(); err != nil {
		return err
	}
	name, err := requiredName("queue or topic", entity)
	if err != nil {
		return err
	}
	if len(sequences) == 0 {
		return errors.New("no scheduled messages named")
	}

	sender, err := c.data.NewSender(name, nil)
	if err != nil {
		return fmt.Errorf("opening a sender on %s: %w", name, err)
	}
	defer func() { _ = sender.Close(context.WithoutCancel(ctx)) }()

	if err := sender.CancelScheduledMessages(ctx, sequences, nil); err != nil {
		if notFound(err) {
			return fmt.Errorf("no queue or topic named %q in %s", name, c.config.namespace)
		}
		return err
	}
	return nil
}
