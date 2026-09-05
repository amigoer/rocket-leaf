package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// Channels, which are this family's consumer groups.
//
// A channel belongs to a topic and every channel under a topic receives a copy
// of every message, so a channel is what a consumer group is elsewhere: the
// unit that has a backlog, that consumers attach to, and that survives them.
// Its depth is that backlog, and it is the only lag NSQ has - there is no
// offset behind it, which is why this driver declares no way to move one.
//
// The canonical ref carries a namespace and a name, and the topic goes in the
// namespace: a channel called "analytics" under two topics is two channels
// with nothing in common, and a page that keyed on the name alone would fold
// them into one row.

var errNoSubscriptionUpdate = errors.New(
	"an nsq channel has no configuration to change; its name and its topic are the whole of it")

// ListSubscriptions enumerates every channel on the cluster.
func (c *Conn) ListSubscriptions(ctx context.Context) ([]*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	perNode, err := c.statsOfEveryNode(ctx, nil)
	if err != nil {
		return nil, err
	}
	return c.foldChannels(perNode, ""), nil
}

// SubscriptionDetail re-reads one channel, asking the daemons for its topic
// rather than for everything they hold.
func (c *Conn) SubscriptionDetail(ctx context.Context, ref model.SubscriptionRef) (*model.Subscription, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if ref.Namespace == "" || ref.Name == "" {
		return nil, errors.New("a channel is identified by its topic and its own name")
	}

	perNode, err := c.statsOfEveryNode(ctx, url.Values{"topic": {ref.Namespace}})
	if err != nil {
		return nil, err
	}
	for _, subscription := range c.foldChannels(perNode, ref.Namespace) {
		if subscription.Ref.Name == ref.Name {
			return subscription, nil
		}
	}
	return nil, fmt.Errorf("no nsqd in this connection is carrying the channel %q on %q",
		ref.Name, ref.Namespace)
}

// CreateSubscription declares a channel on every nsqd carrying its topic.
//
// What it inherits depends on what was there before it, and the difference is
// worth knowing: a channel added to a topic that already has one starts at
// nothing, because the copies were made into the existing channels as the
// messages arrived. A topic with no channel at all holds its messages in its
// own queue instead, and the first channel created drains that queue into
// itself. Either way there is no position to start from - only what nsqd
// happens to still be holding.
func (c *Conn) CreateSubscription(ctx context.Context, spec model.SubscriptionSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	if spec.Ref.Namespace == "" || spec.Ref.Name == "" {
		return errors.New("a channel needs a topic and a name")
	}
	query := url.Values{"topic": {spec.Ref.Namespace}, "channel": {spec.Ref.Name}}
	return c.onEveryCarrier(ctx, "/channel/create", query,
		fmt.Sprintf("no nsqd in this connection is carrying the topic %q", spec.Ref.Namespace))
}

// UpdateSubscription is not offered. See errNoSubscriptionUpdate.
func (c *Conn) UpdateSubscription(_ context.Context, _ model.SubscriptionSpec) error {
	return errNoSubscriptionUpdate
}

// RemoveSubscription deletes a channel and everything it is holding.
//
// Both tiers, in the order a topic delete uses and for the same two reasons.
// nsqlookupd keeps its own registry, and a channel it still lists is one every
// nsqd that later carries the topic recreates for itself - so the registry has
// to be swept. And nsqd goes first, because its registration is asynchronous:
// sweeping the registry before deleting at nsqd loses the race with a register
// still in flight from the create, and the channel comes back.
func (c *Conn) RemoveSubscription(ctx context.Context, ref model.SubscriptionRef) error {
	if err := c.live(); err != nil {
		return err
	}
	if ref.Namespace == "" || ref.Name == "" {
		return errors.New("a channel is identified by its topic and its own name")
	}

	query := url.Values{"topic": {ref.Namespace}, "channel": {ref.Name}}
	removed := c.onEveryCarrier(ctx, "/channel/delete", query,
		fmt.Sprintf("no nsqd in this connection was carrying the channel %q on %q",
			ref.Name, ref.Namespace))
	if err := c.forgetAtLookupd(ctx, "/channel/delete", query); err != nil {
		return err
	}
	return removed
}

// EmptyChannel discards a channel's backlog without touching its topic or the
// other channels under it.
//
// Not the canonical purge, which empties a destination: this empties one
// consumer group's copy. A family where those are the same thing needs only
// the purge; here they are as different as resetting one group's offset is
// from deleting a topic.
func (c *Conn) EmptyChannel(ctx context.Context, topic, channel string) error {
	if err := c.live(); err != nil {
		return err
	}
	if topic == "" || channel == "" {
		return errors.New("a channel is identified by its topic and its own name")
	}
	query := url.Values{"topic": {topic}, "channel": {channel}}
	return c.onEveryCarrier(ctx, "/channel/empty", query,
		fmt.Sprintf("no nsqd in this connection is carrying the channel %q on %q", channel, topic))
}

// SetChannelPaused stops or resumes delivery to one channel's consumers.
//
// Distinct from pausing the topic, which stops the copy into every channel:
// this leaves the other channels running and lets this one's backlog build.
func (c *Conn) SetChannelPaused(ctx context.Context, topic, channel string, paused bool) error {
	if err := c.live(); err != nil {
		return err
	}
	if topic == "" || channel == "" {
		return errors.New("a channel is identified by its topic and its own name")
	}
	path := "/channel/unpause"
	if paused {
		path = "/channel/pause"
	}
	query := url.Values{"topic": {topic}, "channel": {channel}}
	return c.onEveryCarrier(ctx, path, query,
		fmt.Sprintf("no nsqd in this connection is carrying the channel %q on %q", channel, topic))
}

// channelFold is one channel's figures accumulated across the daemons holding
// it. The topic is part of its identity rather than a label: two topics with a
// channel of the same name have nothing to do with each other.
type channelFold struct {
	topic string
	name  string
	nodes []string

	paused       bool
	depth        int64
	backendDepth int64
	inFlight     int
	deferred     int
	clients      int
	messageCount uint64
	requeued     uint64
	timedOut     uint64
}

// foldChannels turns per-daemon stats into one row per (topic, channel).
// A non-empty topic narrows the fold to that topic's channels.
func (c *Conn) foldChannels(perNode []nsqdStats, topic string) []*model.Subscription {
	folded := map[string]*channelFold{}
	for index, stats := range perNode {
		address := hostPort(c.nodes[index].address)
		for _, entry := range stats.Topics {
			if topic != "" && entry.Name != topic {
				continue
			}
			for _, channel := range entry.Channels {
				key := entry.Name + "\x00" + channel.Name
				fold, known := folded[key]
				if !known {
					fold = &channelFold{topic: entry.Name, name: channel.Name}
					folded[key] = fold
				}
				fold.add(address, channel)
			}
		}
	}

	keys := make([]string, 0, len(folded))
	for key := range folded {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	subscriptions := make([]*model.Subscription, 0, len(keys))
	for _, key := range keys {
		subscriptions = append(subscriptions, folded[key].subscription())
	}
	return subscriptions
}

func (f *channelFold) add(address string, channel channelStats) {
	f.nodes = append(f.nodes, address)
	// Paused folds as "any", the way a topic's does: a channel paused on one
	// daemon and running on another really has stopped for a share of its
	// consumers, and calling that running would hide why.
	f.paused = f.paused || channel.Paused
	f.depth += channel.Depth
	f.backendDepth += channel.BackendDepth
	f.inFlight += channel.InFlightCount
	f.deferred += channel.DeferredCount
	f.clients += channel.ClientCount
	f.messageCount += channel.MessageCount
	f.requeued += channel.RequeueCount
	f.timedOut += channel.TimeoutCount
}

func (f *channelFold) subscription() *model.Subscription {
	return &model.Subscription{
		Ref:    model.SubscriptionRef{Namespace: f.topic, Name: f.name},
		Status: f.status(),
		// Consumers connected right now. A channel with none is not broken -
		// it is what makes a channel durable - so the status says offline
		// rather than the count being an error.
		Members: f.clients,
		// One, always. A channel belongs to exactly one topic, unlike a
		// consumer group elsewhere, which can subscribe to several.
		Destinations: 1,
		Backlog:      f.depth,
		// nsqd counts messages since it started and reports no rate.
		RateOut: model.UnknownMetric,
		Attributes: map[string]string{
			AttrTopic:        f.topic,
			AttrPaused:       strconv.FormatBool(f.paused),
			AttrInFlight:     strconv.Itoa(f.inFlight),
			AttrDeferred:     strconv.Itoa(f.deferred),
			AttrRequeued:     strconv.FormatUint(f.requeued, 10),
			AttrTimedOut:     strconv.FormatUint(f.timedOut, 10),
			AttrMessageCount: strconv.FormatUint(f.messageCount, 10),
			AttrBackendDepth: strconv.FormatInt(f.backendDepth, 10),
			AttrEphemeral:    strconv.FormatBool(strings.HasSuffix(f.name, ephemeralSuffix)),
			AttrNodes:        strings.Join(f.nodes, ","),
		},
	}
}

// status is the channel's health as the canonical page reads it.
//
// Paused outranks having consumers, because it is the state that explains a
// backlog nobody is working through: the consumers are connected, they are
// asking for messages, and nsqd is not sending any.
func (f *channelFold) status() model.SubscriptionStatus {
	switch {
	case f.paused:
		return model.SubscriptionWarning
	case f.clients == 0:
		return model.SubscriptionOffline
	default:
		return model.SubscriptionOnline
	}
}
