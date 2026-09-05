package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxSendCount caps one console send.
//
// The cap is this driver's, not nsqd's: /mpub takes as many messages as fit in
// the request, and a send console is for producing a handful by hand rather
// than for load generation.
const maxSendCount = 1000

// PublishRequest is a send in NSQ's own vocabulary.
//
// Short, because an NSQ message is bytes and nothing else. There is no key, no
// header table and no property map - what a producer wants a consumer to know
// goes in the body, and there is nowhere else to put it.
type PublishRequest struct {
	Topic string
	Body  string

	// Count sends the same body more than once, for filling a channel to
	// watch a consumer work through it.
	Count int

	// Delay holds the message back from the channels. nsqd caps it at its
	// --max-req-timeout, one hour by default, and refuses anything longer with
	// INVALID_DEFER - a limit this driver cannot read, because /info does not
	// report it.
	Delay time.Duration

	// Node is the nsqd to publish through, as host:port. Empty means the
	// first in the connection. It matters more here than the field's size
	// suggests: the message is held by the daemon that took it, and a consumer
	// that never finds that daemon never sees it.
	Node string
}

// PublishResult is what the send did, and where.
type PublishResult struct {
	Sent int
	// Node is the daemon that took the messages, so a page can say where they
	// went rather than only that they went.
	Node string
}

/*
 * Publish sends one body, or the same body several times.
 *
 * Two shapes underneath, and the choice is not an optimisation. /mpub takes a
 * batch and is the right call for a repeat, but it silently ignores a defer
 * parameter - confirmed against 1.3.0, where an mpub with defer=1000 answers
 * OK and the messages are delivered immediately. So a delayed send is one /pub
 * per message, however many were asked for.
 *
 * A body containing a newline cannot go through /mpub either: newline is the
 * separator, so one message would arrive as several. A repeat of such a body
 * therefore also goes one at a time.
 */
func (c *Conn) Publish(ctx context.Context, request PublishRequest) (*PublishResult, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Topic) == "" {
		return nil, errors.New("a publish needs a topic")
	}
	if request.Body == "" {
		// nsqd answers MSG_EMPTY, which is correct and says nothing about
		// which field was blank.
		return nil, errors.New("nsqd refuses an empty message body")
	}
	count := request.Count
	if count <= 0 {
		count = 1
	}
	if count > maxSendCount {
		return nil, fmt.Errorf("this console sends at most %d messages at once", maxSendCount)
	}

	target, err := c.publishTarget(request.Node)
	if err != nil {
		return nil, err
	}

	query := url.Values{"topic": {request.Topic}}
	if request.Delay > 0 {
		query.Set("defer", strconv.FormatInt(request.Delay.Milliseconds(), 10))
	}

	if request.Delay > 0 || strings.Contains(request.Body, "\n") {
		for sent := range count {
			if err := c.client.post(ctx, target.address, "/pub", query,
				[]byte(request.Body), nil); err != nil {
				// The count is what already reached the daemon, which
				// separates a send that did nothing from one that stopped
				// partway.
				return &PublishResult{Sent: sent, Node: hostPort(target.address)},
					fmt.Errorf("%s: %w", hostPort(target.address), err)
			}
		}
		return &PublishResult{Sent: count, Node: hostPort(target.address)}, nil
	}

	bodies := make([]string, count)
	for index := range bodies {
		bodies[index] = request.Body
	}
	path := "/pub"
	if count > 1 {
		path = "/mpub"
	}
	if err := c.client.post(ctx, target.address, path, query,
		[]byte(strings.Join(bodies, "\n")), nil); err != nil {
		return nil, fmt.Errorf("%s: %w", hostPort(target.address), err)
	}
	return &PublishResult{Sent: count, Node: hostPort(target.address)}, nil
}

/*
 * SendMessage publishes through the canonical port.
 *
 * The port is RocketMQ's shape and three of its five arguments have nowhere to
 * go on this family, so each is handled deliberately rather than dropped:
 *
 *   - tags and keys are refused rather than ignored. An NSQ message is bytes:
 *     there is no key, no header table and no property map anywhere in the
 *     protocol, so a value put in either field would be silently discarded and
 *     the send reported as having carried it.
 *   - delayLevel is seconds, not a level. RocketMQ's levels are an index into
 *     a broker-side table; nsqd takes a duration and ports.go fixes no unit.
 *     Seconds is what Pulsar's driver chose, what the send console labels, and
 *     what the tests pin.
 *
 * The id it returns is empty, and that is the honest answer rather than a
 * placeholder: nsqd assigns an id when the message is queued and answers the
 * publish with the word OK. Nothing is ever handed back that could be used to
 * look the message up - which is the same fact that keeps this driver from
 * declaring a message-by-id capability at all.
 */
func (c *Conn) SendMessage(
	ctx context.Context, topic, tags, keys, body string, delayLevel int,
) (string, error) {
	if strings.TrimSpace(tags) != "" || strings.TrimSpace(keys) != "" {
		return "", errors.New(
			"an nsq message is bytes and carries no tag or key; put them in the body")
	}
	if delayLevel < 0 {
		return "", errors.New("a delay cannot be negative")
	}
	_, err := c.Publish(ctx, PublishRequest{
		Topic: topic,
		Body:  body,
		Count: 1,
		Delay: time.Duration(delayLevel) * time.Second,
	})
	return "", err
}

// publishTarget picks the daemon a send goes through.
//
// Named rather than chosen for the caller, because the choice is visible: the
// message is held by the nsqd that took it, and a consumer connected to a
// different one sees it only if it also finds this daemon through nsqlookupd.
func (c *Conn) publishTarget(name string) (node, error) {
	if strings.TrimSpace(name) == "" {
		return c.nodes[0], nil
	}
	wanted := hostPort(normaliseAddress(name))
	for _, candidate := range c.nodes {
		if hostPort(candidate.address) == wanted {
			return candidate, nil
		}
	}
	return node{}, fmt.Errorf("%q is not one of this connection's nsqd", name)
}

// Nodes lists the daemons a send can be addressed to, in the order the profile
// names them. The send console offers them; nothing else needs the list.
func (c *Conn) Nodes() []string {
	names := make([]string, 0, len(c.nodes))
	for _, n := range c.nodes {
		names = append(names, hostPort(n.address))
	}
	return names
}
