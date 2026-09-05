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

// Attribute keys this driver puts on a Destination.
//
// A contract between this package and frontend/src/mq/nsq, not part of the
// shared vocabulary. The two depth keys are the ones worth knowing about:
// they are the split behind the canonical Depth, and a board that showed only
// the total would leave a reader unable to tell a topic nothing has consumed
// from one that is paused.
const (
	AttrPaused       = "paused"
	AttrTopicDepth   = "topicDepth"
	AttrChannelDepth = "channelDepth"
	AttrBackendDepth = "backendDepth"
	AttrMessageCount = "messageCount"
	AttrMessageBytes = "messageBytes"
	AttrInFlight     = "inFlight"
	AttrDeferred     = "deferred"
	AttrRequeued     = "requeued"
	AttrTimedOut     = "timedOut"
	AttrEphemeral    = "ephemeral"
	AttrNodes        = "nodes"
	AttrChannels     = "channels"
)

// ephemeralSuffix marks a topic or channel that exists only while something is
// connected to it. nsqd deletes it when the last client goes.
const ephemeralSuffix = "#ephemeral"

/*
 * ListDestinations reads /stats from every nsqd and folds the answers into one
 * topic per name.
 *
 * The fold is the whole of what this driver has to get right about NSQ's
 * topology. A topic exists once per nsqd that was asked to carry it, and those
 * copies are independent queues rather than shards - so a name is one row,
 * and every figure on it is the sum of the daemons holding it.
 *
 * Depth is topic depth plus the depth of every channel under it, which is
 * larger than the number of messages published and is not a mistake: nsqd
 * copies each message into every channel, so a topic with two channels each
 * holding a hundred really is holding two hundred. The split is carried in
 * attributes so a board can show where the messages are - a topic depth above
 * zero means nothing has been copied out yet, which in practice means the
 * topic is paused or has no channels at all.
 */
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	// The filter's IncludeInternal has nothing to hide here. NSQ has no system
	// topics: nsqd's own figures are reported beside the topic list rather
	// than as topics, so every name in it is one somebody created.
	if err := c.live(); err != nil {
		return nil, err
	}

	perNode, err := c.statsOfEveryNode(ctx, nil)
	if err != nil {
		return nil, err
	}

	folded := map[string]*topicFold{}
	for index, stats := range perNode {
		for _, topic := range stats.Topics {
			fold, known := folded[topic.Name]
			if !known {
				fold = &topicFold{name: topic.Name}
				folded[topic.Name] = fold
			}
			fold.add(hostPort(c.nodes[index].address), topic)
		}
	}

	names := make([]string, 0, len(folded))
	for name := range folded {
		names = append(names, name)
	}
	sort.Strings(names)

	destinations := make([]*model.Destination, 0, len(names))
	for _, name := range names {
		destinations = append(destinations, folded[name].destination())
	}
	return destinations, nil
}

// DestinationDetail asks every nsqd about one topic.
//
// Filtered at the daemon rather than by walking the whole list: /stats takes a
// topic name, and a cluster with thousands of topics should not answer for all
// of them to describe one.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if ref.Name == "" {
		return nil, errors.New("a topic needs a name")
	}

	perNode, err := c.statsOfEveryNode(ctx, url.Values{"topic": {ref.Name}})
	if err != nil {
		return nil, err
	}

	fold := &topicFold{name: ref.Name}
	found := false
	for index, stats := range perNode {
		for _, topic := range stats.Topics {
			if topic.Name != ref.Name {
				continue
			}
			found = true
			fold.add(hostPort(c.nodes[index].address), topic)
		}
	}
	if !found {
		return nil, fmt.Errorf("no nsqd in this connection is carrying the topic %q", ref.Name)
	}
	return fold.destination(), nil
}

// statsOfEveryNode reads /stats from all of them at once.
//
// include_clients is off: the per-channel client list is the largest thing in
// the response and no destination or subscription figure is computed from it.
// The clients board asks for it explicitly.
func (c *Conn) statsOfEveryNode(ctx context.Context, query url.Values) ([]nsqdStats, error) {
	values := url.Values{"format": {"json"}, "include_clients": {"false"}}
	for key, entries := range query {
		values[key] = entries
	}
	return eachNode(ctx, c.nodes, func(ctx context.Context, n node) (nsqdStats, error) {
		var stats nsqdStats
		err := c.client.get(ctx, n.address, "/stats", values, &stats)
		return stats, err
	})
}

// topicFold is one topic's figures accumulated across the daemons holding it.
type topicFold struct {
	name  string
	nodes []string

	paused       bool
	topicDepth   int64
	channelDepth int64
	backendDepth int64
	messageCount uint64
	messageBytes uint64
	inFlight     int
	deferred     int
	requeued     uint64
	timedOut     uint64

	// channels is a set because the same channel exists on every daemon
	// carrying the topic: nsqd asks nsqlookupd which channels are registered
	// for a name and creates each one locally, so counting them per node
	// would multiply the subscriber count by the cluster size.
	channels map[string]struct{}
}

func (f *topicFold) add(address string, topic topicStats) {
	f.nodes = append(f.nodes, address)
	// Paused is per daemon and folds as "any". A topic paused on one nsqd and
	// not another is genuinely half-paused, and calling that "running" would
	// hide the reason half the messages have stopped moving.
	f.paused = f.paused || topic.Paused
	f.topicDepth += topic.Depth
	f.backendDepth += topic.BackendDepth
	f.messageCount += topic.MessageCount
	f.messageBytes += topic.MessageBytes

	if f.channels == nil {
		f.channels = map[string]struct{}{}
	}
	for _, channel := range topic.Channels {
		f.channels[channel.Name] = struct{}{}
		f.channelDepth += channel.Depth
		f.backendDepth += channel.BackendDepth
		f.inFlight += channel.InFlightCount
		f.deferred += channel.DeferredCount
		f.requeued += channel.RequeueCount
		f.timedOut += channel.TimeoutCount
	}
}

func (f *topicFold) destination() *model.Destination {
	channels := make([]string, 0, len(f.channels))
	for channel := range f.channels {
		channels = append(channels, channel)
	}
	sort.Strings(channels)

	return &model.Destination{
		Ref:         model.DestinationRef{Name: f.name},
		Partitions:  model.UnknownMetric,
		Subscribers: len(channels),
		Depth:       f.topicDepth + f.channelDepth,
		// nsqd counts messages since it started and reports no rate of any
		// kind. A figure derived from two samples taken here would be this
		// app's measurement rather than the broker's, and the cluster page
		// already samples for exactly that.
		RateIn:  model.UnknownMetric,
		RateOut: model.UnknownMetric,
		Attributes: map[string]string{
			AttrPaused:       strconv.FormatBool(f.paused),
			AttrTopicDepth:   strconv.FormatInt(f.topicDepth, 10),
			AttrChannelDepth: strconv.FormatInt(f.channelDepth, 10),
			AttrBackendDepth: strconv.FormatInt(f.backendDepth, 10),
			AttrMessageCount: strconv.FormatUint(f.messageCount, 10),
			AttrMessageBytes: strconv.FormatUint(f.messageBytes, 10),
			AttrInFlight:     strconv.Itoa(f.inFlight),
			AttrDeferred:     strconv.Itoa(f.deferred),
			AttrRequeued:     strconv.FormatUint(f.requeued, 10),
			AttrTimedOut:     strconv.FormatUint(f.timedOut, 10),
			AttrEphemeral:    strconv.FormatBool(strings.HasSuffix(f.name, ephemeralSuffix)),
			AttrNodes:        strings.Join(f.nodes, ","),
			AttrChannels:     strings.Join(channels, ","),
		},
	}
}
