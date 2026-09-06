package ibmmq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

/*
 * Sending, which goes to a queue and only to a queue.
 *
 * The messaging interface has no topic resource at any version of the API: it
 * carries messages to and from queues, and that is the whole of it. Publishing
 * needs an MQ client, so this console does not offer it - a queue name is not
 * a topic string, and a send that quietly went to a queue named after a topic
 * would be worse than no button.
 *
 * It also carries character data and nothing else. A body sent as anything
 * else is refused with HTTP 415 before it reaches the queue manager, so a
 * binary payload has to come from a real client too.
 */

// PublishRequest is one send, as the console collects it.
//
// Deliberately not MessagePublisher's signature. That one is RocketMQ's - a
// topic, tags, keys and a delay level - and an MQ message has none of those
// four. What it has instead is a descriptor: a correlation identifier that
// matches a reply to its request, a persistence that decides whether the
// message survives a restart, an expiry, and whatever properties the sending
// application chose to attach.
type PublishRequest struct {
	// Queue is where the message goes. A topic name here is refused rather
	// than resolved: the interface cannot publish.
	Queue string
	Body  string

	// ContentType is what the message is declared as. It reaches the message
	// descriptor's format field, and it must be a character type: the server
	// refuses anything else outright.
	ContentType string

	// CorrelationID is 48 hexadecimal characters or empty. Empty is the
	// ordinary case and leaves the field at MQ's own zeroes.
	CorrelationID string

	// Persistent decides whether the queue manager writes the message to its
	// log. A non-persistent message is faster and is gone if the queue manager
	// restarts, which is the difference worth a switch rather than a default.
	Persistent bool

	// ExpirySeconds discards the message if nothing has read it by then. Zero
	// means unlimited, which is MQ's own default.
	ExpirySeconds int

	// Properties are attached to the message under their own names. They are
	// what an application reads back as message properties.
	Properties map[string]string

	// Count sends the same body more than once. One when left at zero.
	Count int
}

// PublishResult is what the send did.
type PublishResult struct {
	Sent int
	// MessageID is the first message's, as the queue manager assigned it. It
	// is the handle the browse lists and the message page opens, so it is
	// worth handing straight back.
	MessageID string
}

// maxSendCount caps one console send. Each copy is its own HTTP request, so a
// larger number is a long wait rather than a bigger batch.
const maxSendCount = 100

/*
 * SendMessage is the canonical send, and three of its five arguments have no
 * counterpart here.
 *
 * They are refused rather than ignored. A tag and a key are RocketMQ's, and a
 * delay level is a scheduled send IBM MQ has no equivalent of at all - the
 * queue manager takes a message and it is readable immediately. A console that
 * accepted them and dropped them would be offering three controls that do
 * nothing.
 */
func (c *Conn) SendMessage(ctx context.Context, topic, tags, keys, body string, delayLevel int) (string, error) {
	if strings.TrimSpace(tags) != "" || strings.TrimSpace(keys) != "" {
		return "", errors.New("an ibm mq message has no tag and no key: " +
			"what it carries instead is a correlation id and its own properties")
	}
	if delayLevel > 0 {
		return "", errors.New("ibm mq has no scheduled send: a message the queue manager " +
			"accepts is readable immediately")
	}

	result, err := c.Publish(ctx, PublishRequest{Queue: topic, Body: body})
	if err != nil {
		return "", err
	}
	return result.MessageID, nil
}

// Publish sends one body, or the same body several times, to one queue.
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	queue := strings.TrimSpace(request.Queue)
	if queue == "" {
		return nil, errors.New("no queue name given")
	}

	// A topic here would be a send to a destination that cannot take one, and
	// the queue manager's own refusal names an object rather than the reason.
	kind, err := c.kindOf(ctx, queue)
	if err != nil {
		return nil, err
	}
	if kind == KindTopic {
		return nil, fmt.Errorf("%q is a topic, and the messaging interface can only send to a "+
			"queue: publishing needs an mq client", queue)
	}

	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("%d copies is more than one send should be: the interface takes "+
			"one message per request, so this would be %d of them", count, count)
	}

	headers, err := descriptorHeaders(request)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "text/plain;charset=utf-8"
	}

	path := fmt.Sprintf("/qmgr/%s/queue/%s/message", c.qmgr, url.PathEscape(queue))
	result := &PublishResult{}
	for sent := 0; sent < count; sent++ {
		response, err := c.rest.messagingPost(ctx, path, contentType, []byte(request.Body), headers)
		if err != nil {
			// The count so far is reported with the failure: a send of fifty
			// that stopped at thirty put thirty messages on the queue, and a
			// caller that read only the error would not know.
			return result, err
		}
		result.Sent++
		if result.MessageID == "" {
			result.MessageID = response.Get(headerMessageID)
		}
	}
	return result, nil
}

// The request headers the message descriptor is set through. They are the same
// names the browse reads back, in the other direction.
const (
	requestCorrelationID = "ibm-mq-md-correlationId"
	requestPersistence   = "ibm-mq-md-persistence"
	requestExpiry        = "ibm-mq-md-expiry"
	requestUserPrefix    = "ibm-mq-usr-"
)

// descriptorHeaders turns the request into the headers mqweb reads.
func descriptorHeaders(request PublishRequest) (map[string]string, error) {
	headers := map[string]string{}

	if correlation := strings.TrimSpace(request.CorrelationID); correlation != "" {
		if err := validHexIdentifier(correlation); err != nil {
			return nil, err
		}
		headers[requestCorrelationID] = correlation
	}
	if request.Persistent {
		headers[requestPersistence] = "persistent"
	} else {
		headers[requestPersistence] = "nonPersistent"
	}
	if request.ExpirySeconds > 0 {
		// Tenths of a second, which is MQ's own unit for expiry and the sort
		// of thing that is quietly wrong by a factor of ten otherwise.
		headers[requestExpiry] = fmt.Sprintf("%d", request.ExpirySeconds*10)
	}
	for name, value := range request.Properties {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		headers[requestUserPrefix+name] = value
	}
	return headers, nil
}

// validHexIdentifier applies MQ's rule for a correlation identifier: 24 bytes,
// spelled as 48 hexadecimal characters. It is checked here because the
// server's own refusal names a hex string rather than the field it came from.
func validHexIdentifier(id string) error {
	if len(id) != 48 {
		return fmt.Errorf("a correlation id is 48 hexadecimal characters; %q is %d", id, len(id))
	}
	for _, letter := range id {
		switch {
		case letter >= '0' && letter <= '9',
			letter >= 'a' && letter <= 'f',
			letter >= 'A' && letter <= 'F':
		default:
			return fmt.Errorf("a correlation id is hexadecimal; %q contains %q", id, letter)
		}
	}
	return nil
}
