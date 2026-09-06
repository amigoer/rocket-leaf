package solace

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

/*
 * Sending, which goes through a different port from everything else here.
 *
 * SEMP manages the broker and carries no message data at any version, so a
 * send is the REST messaging interface: an HTTP POST whose path is where the
 * message goes and whose body is the message. That interface is probed when
 * the connection opens and this capability is degraded with a reason when it
 * did not answer - see conn.go.
 *
 * Unlike IBM MQ's messaging interface, this one has a topic resource as well
 * as a queue one, so the console can publish rather than only enqueue. The two
 * are genuinely different gestures and the form says which: /QUEUE/<name> puts
 * a message on one endpoint by name, and /TOPIC/<topic> hands it to the
 * broker's matching, where it lands on every queue whose subscriptions match
 * and on nothing at all when none do.
 */

// Where a send goes. The two are different paths on the interface and
// different things to do, so the console picks rather than guessing from the
// name.
const (
	TargetQueue = "queue"
	TargetTopic = "topic"
)

// Delivery modes the interface takes, spelled as it spells them. A bad value
// is refused by the broker with a message naming all three, which is what
// these are quoted from.
const (
	DeliveryPersistent    = "persistent"
	DeliveryNonPersistent = "non-persistent"
	DeliveryDirect        = "direct"
)

// The headers the interface reads. Every one of these was confirmed against a
// broker: Solace-Partition-Key looks as though it should be here and is not
// read at all, so it is not offered.
const (
	headerDeliveryMode = "Solace-Delivery-Mode"
	headerTTL          = "Solace-Time-To-Live-In-ms"
	headerDMQEligible  = "Solace-DMQ-Eligible"
	headerCorrelation  = "Solace-Correlation-ID"
	headerReplyTo      = "Solace-Reply-To-Destination"
	headerUserProperty = "Solace-User-Property-"
)

// PublishRequest is one send, as the console collects it.
//
// Deliberately not MessagePublisher's signature. That one is RocketMQ's - a
// topic, tags, keys and a delay level - and a Solace message has none of those
// four. What it has instead is a delivery mode that decides whether the broker
// writes it to the spool, a time to live, and the flag that decides whether it
// is moved or discarded when it is given up on.
type PublishRequest struct {
	// Target is TargetQueue or TargetTopic. It picks the path, and the two are
	// different things: a queue send names one endpoint, a topic send is
	// matched against every subscription in the Message VPN.
	Target string
	// Destination is the queue name or the topic.
	Destination string
	Body        string

	// ContentType is what the message is declared as. Unlike IBM MQ's
	// interface this one takes anything, including binary.
	ContentType string

	// DeliveryMode decides whether the broker writes the message to its spool.
	// Empty is persistent, which is the safe default and the one a console
	// user almost always means.
	DeliveryMode string

	// TimeToLiveMs discards the message if nothing has taken it by then. Zero
	// is the broker's own unlimited.
	TimeToLiveMs int

	// DMQEligible decides whether a message given up on is moved to the
	// queue's dead message queue or discarded. Off is the broker's default and
	// is why a queue configured to dead-letter can still quietly discard.
	DMQEligible bool

	CorrelationID string
	// ReplyTo is a destination spelled the interface's way - /QUEUE/name or a
	// bare topic - and is passed through as given.
	ReplyTo string

	// Properties are attached under their own names, prefixed the way the
	// interface expects, and are what a receiving application reads back.
	Properties map[string]string

	// Count sends the same body more than once. One when left at zero.
	Count int
}

// PublishResult is what the send did.
//
// There is no message id, and its absence is the interface rather than an
// omission: the broker answers a successful send with an empty body and no
// identifier of any kind. The id a browse lists is the queue's own sequence
// number, assigned when the message is spooled and never reported to the
// publisher - so anything here would be this app reading the queue afterwards
// and guessing which of the new messages was its own.
type PublishResult struct {
	Sent int
}

// maxSendCount caps one console send. Each copy is its own HTTP request, so a
// larger number is a long wait rather than a bigger batch.
const maxSendCount = 100

/*
 * SendMessage is the canonical send, and three of its five arguments have no
 * counterpart here.
 *
 * They are refused rather than ignored. A tag and a key are RocketMQ's, and a
 * delay level is a scheduled send - Solace has one, as a queue's delivery
 * delay, but it is a setting on the endpoint rather than something a publisher
 * chooses per message, so a console that accepted the argument would be
 * offering a control that changes nothing.
 *
 * The empty string it returns is honest: the interface reports no identifier
 * for a message it took.
 */
func (c *Conn) SendMessage(ctx context.Context, topic, tags, keys, body string, delayLevel int) (string, error) {
	if strings.TrimSpace(tags) != "" || strings.TrimSpace(keys) != "" {
		return "", errors.New("a solace message has no tag and no key: " +
			"what it carries instead is a correlation id and its own user properties")
	}
	if delayLevel > 0 {
		return "", errors.New("a solace publisher cannot delay one message: " +
			"a delivery delay is a setting on the queue, applied to everything it takes")
	}

	// A topic, because that is what the canonical argument is called and what
	// a caller with only this signature means by it.
	_, err := c.Publish(ctx, PublishRequest{
		Target: TargetTopic, Destination: topic, Body: body,
	})
	return "", err
}

// Publish sends one body, or the same body several times.
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if c.rest == nil {
		return nil, errors.New("this connection has no rest messaging interface; " +
			"semp manages the broker and carries no messages")
	}

	destination := strings.TrimSpace(request.Destination)
	if destination == "" {
		return nil, errors.New("no destination given")
	}
	path, err := sendPath(request.Target, destination)
	if err != nil {
		return nil, err
	}

	mode, err := deliveryMode(request.DeliveryMode)
	if err != nil {
		return nil, err
	}
	if request.TimeToLiveMs < 0 {
		return nil, errors.New("a time to live cannot be negative")
	}

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d copies at once", maxSendCount)
	}

	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}

	headers := map[string]string{
		headerDeliveryMode: mode,
		// Always sent, because the broker's own default is false and a queue
		// configured to dead-letter then discards quietly. Saying it either
		// way makes the console's switch mean what it says.
		headerDMQEligible: strconv.FormatBool(request.DMQEligible),
	}
	if request.TimeToLiveMs > 0 {
		headers[headerTTL] = strconv.Itoa(request.TimeToLiveMs)
	}
	if value := strings.TrimSpace(request.CorrelationID); value != "" {
		headers[headerCorrelation] = value
	}
	if value := strings.TrimSpace(request.ReplyTo); value != "" {
		headers[headerReplyTo] = value
	}
	for name, value := range request.Properties {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			headers[headerUserProperty+trimmed] = value
		}
	}

	body := []byte(request.Body)
	sent := 0
	for range count {
		if err := c.rest.post(ctx, path, contentType, body, headers); err != nil {
			// The count is what already reached the broker, which is
			// meaningful on a failure halfway through a batch.
			return &PublishResult{Sent: sent}, err
		}
		sent++
	}
	return &PublishResult{Sent: sent}, nil
}

// sendPath is where the message goes, escaped.
//
// A topic is escaped the same way a queue name is, and it has to be: a topic
// is written with slashes as its level separator, and leaving them raw would
// make "orders/eu/created" three path segments the interface refuses.
func sendPath(target, destination string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case TargetQueue:
		return "/QUEUE/" + segment(destination), nil
	case TargetTopic, "":
		return "/TOPIC/" + segment(destination), nil
	default:
		return "", fmt.Errorf("a send goes to a queue or a topic, not to %q", target)
	}
}

// deliveryMode settles which of the three the broker is being asked for.
//
// Checked here rather than left to the broker because the broker's refusal is
// an XML document quoting all three spellings, which reaches a console user as
// a wall of markup rather than as "that is not a delivery mode".
func deliveryMode(mode string) (string, error) {
	switch trimmed := strings.TrimSpace(strings.ToLower(mode)); trimmed {
	case "":
		return DeliveryPersistent, nil
	case DeliveryPersistent, DeliveryNonPersistent, DeliveryDirect:
		return trimmed, nil
	default:
		return "", fmt.Errorf("a delivery mode is %s, %s or %s, not %q",
			DeliveryPersistent, DeliveryNonPersistent, DeliveryDirect, mode)
	}
}
