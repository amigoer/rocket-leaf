package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// The operations that change what a topic holds, and the two that this family
// has no way to perform.
var (
	errNoMove = errors.New(
		"nothing drains one nsq topic into another; a message enters a topic by being published to it")
	errNoDrop = errors.New(
		"nsqd empties a queue whole and cannot discard a bounded batch from the head")
	errNoRebalance = errors.New(
		"an nsq topic lives on the daemon it was created on; nothing redistributes one")
)

/*
 * PurgeQueue discards everything the topic is holding, on every daemon that
 * carries it.
 *
 * Emptying the topic is not enough, and that is the whole reason this method
 * is longer than one call. nsqd copies each message into every channel as it
 * arrives and /topic/empty touches only the topic's own queue, so on a topic
 * with any channel at all - which is every topic anything is consuming - the
 * call answers 200 and the depth on screen does not move. Confirmed against
 * 1.3.0: three messages published to a topic with one channel leave the topic
 * at depth 0 and the channel at 3, and /topic/empty changes neither.
 *
 * So the purge is the topic and then each of its channels, and the channel
 * list is read per daemon rather than assumed: a channel exists on the nsqd
 * that created it, and asking one daemon to empty a channel another holds is
 * a 404 rather than a no-op.
 */
func (c *Conn) PurgeQueue(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	if ref.Name == "" {
		return errors.New("a topic needs a name")
	}

	query := url.Values{"topic": {ref.Name}}
	perNode, err := c.statsOfEveryNode(ctx, query)
	if err != nil {
		return err
	}

	carried := false
	group, groupCtx := errgroup.WithContext(ctx)
	for index, stats := range perNode {
		for _, topic := range stats.Topics {
			if topic.Name != ref.Name {
				continue
			}
			carried = true
			address := c.nodes[index].address
			group.Go(func() error {
				if err := c.client.post(groupCtx, address, "/topic/empty", query, nil, nil); err != nil {
					return fmt.Errorf("%s: %w", hostPort(address), err)
				}
				for _, channel := range topic.Channels {
					values := url.Values{"topic": {ref.Name}, "channel": {channel.Name}}
					if err := c.client.post(groupCtx, address, "/channel/empty", values, nil, nil); err != nil {
						return fmt.Errorf("%s channel %s: %w", hostPort(address), channel.Name, err)
					}
				}
				return nil
			})
		}
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if !carried {
		return fmt.Errorf("no nsqd in this connection is carrying the topic %q", ref.Name)
	}
	return nil
}

// MoveMessages is not offered. See errNoMove.
func (c *Conn) MoveMessages(_ context.Context, _ model.MoveRequest) (int, error) {
	return 0, errNoMove
}

// DropMessages is not offered. See errNoDrop.
func (c *Conn) DropMessages(_ context.Context, _ model.DestinationRef, _ int) (int, error) {
	return 0, errNoDrop
}

// RebalanceQueues is not offered. See errNoRebalance.
func (c *Conn) RebalanceQueues(_ context.Context) error { return errNoRebalance }

// SetTopicPaused stops or resumes delivery into a topic's channels.
//
// Not a canonical capability, because no other family here has the gesture:
// pausing is neither a purge nor a delete, and publishing carries on while it
// is in force - what stops is the copy into each channel, so the messages pile
// up in the topic rather than being refused or dropped. That is why the topics
// board draws it as a state rather than as an action with a confirmation.
//
// Applied on every daemon carrying the topic. A topic paused on one nsqd and
// running on another is a real state and a confusing one, so this driver never
// creates it on purpose.
func (c *Conn) SetTopicPaused(ctx context.Context, name string, paused bool) error {
	if err := c.live(); err != nil {
		return err
	}
	if name == "" {
		return errors.New("a topic needs a name")
	}
	path := "/topic/unpause"
	if paused {
		path = "/topic/pause"
	}
	return c.onEveryCarrier(ctx, path, url.Values{"topic": {name}},
		fmt.Sprintf("no nsqd in this connection is carrying the topic %q", name))
}

// notCarriedError is what a management call reports when no daemon in the
// connection had the object at all.
//
// A type rather than a message, because the two failures lead opposite ways: a
// delete that found nothing should still sweep the discovery tier, and one
// that failed on a daemon must not - a topic still there and no longer
// registered is worse than a topic still there.
type notCarriedError struct{ what string }

func (e *notCarriedError) Error() string { return e.what }

func notCarried(err error) bool {
	var missing *notCarriedError
	return errors.As(err, &missing)
}

// onEveryCarrier runs one management call on every daemon that has the object,
// and reports missing when none of them did.
//
// The 404 is ordinary rather than a failure: a topic placed on two of four
// daemons is a normal cluster, and treating the other two as errors would make
// every management call on it fail.
func (c *Conn) onEveryCarrier(ctx context.Context, path string, query url.Values, missing string) error {
	applied := make([]bool, len(c.nodes))
	group, groupCtx := errgroup.WithContext(ctx)
	for index, n := range c.nodes {
		group.Go(func() error {
			err := c.client.post(groupCtx, n.address, path, query, nil, nil)
			switch {
			case err == nil:
				applied[index] = true
				return nil
			case notFound(err):
				return nil
			default:
				return fmt.Errorf("%s: %w", hostPort(n.address), err)
			}
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	for _, done := range applied {
		if done {
			return nil
		}
	}
	return &notCarriedError{what: missing}
}
