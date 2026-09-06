package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Node attribute keys, on top of the shared ones in attributes.go.
const (
	AttrHostname         = "hostname"
	AttrBroadcastAddress = "broadcastAddress"
	AttrTCPPort          = "tcpPort"
	AttrHTTPPort         = "httpPort"
	AttrStartTime        = "startTime"
	AttrHealth           = "health"
	AttrTopicCount       = "topicCount"
	AttrChannelCount     = "channelCount"
	AttrClientCount      = "clientCount"
	AttrDepth            = "depth"
	AttrHeapInUse        = "heapInUseBytes"
	AttrHeapObjects      = "heapObjects"
	AttrGCRuns           = "gcTotalRuns"
	// AttrProducerCount and AttrDirectoryTopics are a directory node's: how
	// many nsqd have registered with it and how many topic names it knows.
	// They are what makes one nsqlookupd's answer different from another's,
	// which is the reason to run more than one.
	AttrProducerCount   = "producerCount"
	AttrDirectoryTopics = "directoryTopics"
)

// lookupdAbsent is why the directory board has nothing to draw, as an i18n key
// rather than a sentence. The renderer turns it into the user's language; an
// English frame around it would put the key itself on screen.
const lookupdAbsent = "mq.nsq.degraded.lookupdAbsent"

/*
 * ListNodes describes every nsqd this connection was pointed at.
 *
 * The list is the profile's, not the cluster's, and that is deliberate. There
 * is no nsqd that knows about the others: nsqlookupd knows which daemons have
 * registered with it, and those are not necessarily the ones this connection
 * speaks for - a daemon can run with no lookupd at all, and a lookupd can
 * still be advertising one that has gone. So the nodes here are exactly the
 * addresses whose figures every other page is a sum of, which is the only list
 * that makes those sums explicable.
 */
func (c *Conn) ListNodes(ctx context.Context) ([]*model.Node, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	perNode, err := c.statsOfEveryNode(ctx, nil)
	if err != nil {
		return nil, err
	}
	nodes := make([]*model.Node, 0, len(perNode))
	for index, stats := range perNode {
		nodes = append(nodes, describeNode(c.nodes[index], stats))
	}
	return nodes, nil
}

// NodeDetail re-reads one daemon. The address is host:port, as the list
// reports it.
func (c *Conn) NodeDetail(ctx context.Context, address string) (*model.Node, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	target, err := c.nodeAt(address)
	if err != nil {
		return nil, err
	}
	var stats nsqdStats
	if err := c.client.get(ctx, target.address, "/stats",
		url.Values{"format": {"json"}, "include_clients": {"false"}}, &stats); err != nil {
		return nil, fmt.Errorf("%s: %w", hostPort(target.address), err)
	}
	return describeNode(target, stats), nil
}

// ClusterOverview is what the cluster page's header shows.
func (c *Conn) ClusterOverview(ctx context.Context) (*model.ClusterOverview, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	perNode, err := c.statsOfEveryNode(ctx, nil)
	if err != nil {
		return nil, err
	}

	topics := map[string]struct{}{}
	channels := map[string]struct{}{}
	online, clients := 0, 0
	var depth int64
	for _, stats := range perNode {
		if healthy(stats) {
			online++
		}
		for _, topic := range stats.Topics {
			topics[topic.Name] = struct{}{}
			depth += topic.Depth
			for _, channel := range topic.Channels {
				channels[topic.Name+"\x00"+channel.Name] = struct{}{}
				depth += channel.Depth
				clients += channel.ClientCount
			}
		}
	}

	return &model.ClusterOverview{
		// NSQ has no cluster name. There is nothing to name one: the daemons
		// do not know about each other, and nsqlookupd is a registry rather
		// than a membership list.
		Name:          "",
		TotalNodes:    len(c.nodes),
		OnlineNodes:   online,
		Destinations:  len(topics),
		Subscriptions: len(channels),
		// nsqd reports no disk figure of any kind - not free space, not used,
		// not a percentage. A topic's overflow file is on whatever filesystem
		// --data-path points at, and nsqd never looks at it.
		AvgDiskUsage: model.UnknownMetric,
		Attributes: map[string]string{
			AttrDepth:       strconv.FormatInt(depth, 10),
			AttrClientCount: strconv.Itoa(clients),
		},
	}, nil
}

// describeNode turns one daemon's /info and /stats into a canonical node.
func describeNode(n node, stats nsqdStats) *model.Node {
	channels, clients := 0, 0
	var depth int64
	for _, topic := range stats.Topics {
		depth += topic.Depth
		channels += len(topic.Channels)
		for _, channel := range topic.Channels {
			depth += channel.Depth
			clients += channel.ClientCount
		}
	}

	status := model.NodeOnline
	if !healthy(stats) {
		// nsqd sets health to the error that broke it - a failed write to the
		// overflow file is the usual one - and keeps answering HTTP while it
		// is set. A node reporting a reachable port and a broken disk is
		// exactly the case a status of "online" would hide.
		status = model.NodeWarning
	}

	name := n.info.Hostname
	if name == "" {
		name = hostPort(n.address)
	}
	return &model.Node{
		Name:    name,
		Address: hostPort(n.address),
		// No cluster label: nsqd belongs to no cluster object, only to
		// whichever nsqlookupd it was told to register with.
		Cluster: "",
		Version: stats.Version,
		Status:  status,
		// nsqd counts messages since it started and reports no rate.
		RateIn:    model.UnknownMetric,
		RateOut:   model.UnknownMetric,
		DiskUsage: model.UnknownMetric,
		// Now, because the daemon just answered. nsqd reports when it started
		// and nothing about when it was last heard from, and putting its start
		// time here would date a healthy node to whenever it was deployed -
		// the start time is on the row as an attribute instead.
		LastSeen: time.Now().UTC().Format(time.RFC3339),
		Attributes: map[string]string{
			AttrHostname:         n.info.Hostname,
			AttrBroadcastAddress: n.info.BroadcastAddress,
			AttrTCPPort:          strconv.Itoa(n.info.TCPPort),
			AttrHTTPPort:         strconv.Itoa(n.info.HTTPPort),
			AttrStartTime:        strconv.FormatInt(stats.StartTime, 10),
			AttrHealth:           stats.Health,
			AttrTopicCount:       strconv.Itoa(len(stats.Topics)),
			AttrChannelCount:     strconv.Itoa(channels),
			AttrClientCount:      strconv.Itoa(clients),
			AttrDepth:            strconv.FormatInt(depth, 10),
			AttrHeapInUse:        strconv.FormatUint(stats.Memory.HeapInUseBytes, 10),
			AttrHeapObjects:      strconv.FormatUint(stats.Memory.HeapObjects, 10),
			AttrGCRuns:           strconv.FormatUint(stats.Memory.GCTotalRuns, 10),
		},
	}
}

// healthy reads nsqd's own verdict on itself. It is the string "OK" while the
// daemon is fine and the error that broke it otherwise.
func healthy(stats nsqdStats) bool { return stats.Health == "OK" }

/*
 * ListDirectoryNodes describes the nsqlookupd tier.
 *
 * A tier worth its own board rather than a second cluster page, because it
 * answers a different question. The nsqd list is where messages are; this is
 * what consumers are told when they ask. The two disagreeing - a lookupd
 * advertising a daemon that has gone, or not yet knowing about one that is
 * there - is the failure this board exists to make visible, and it is why the
 * producer and topic counts are per lookupd rather than summed.
 */
func (c *Conn) ListDirectoryNodes(ctx context.Context) ([]*model.Node, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	if len(c.config.lookupd) == 0 {
		return nil, errors.New("this connection names no nsqlookupd")
	}

	nodes := make([]*model.Node, len(c.config.lookupd))
	group, groupCtx := errgroup.WithContext(ctx)
	for index, address := range c.config.lookupd {
		group.Go(func() error {
			node, err := c.describeDirectoryNode(groupCtx, address)
			if err != nil {
				return fmt.Errorf("%s: %w", hostPort(address), err)
			}
			nodes[index] = node
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Conn) describeDirectoryNode(ctx context.Context, address string) (*model.Node, error) {
	var info lookupInfo
	if err := c.client.get(ctx, address, "/info", nil, &info); err != nil {
		return nil, err
	}
	var nodes lookupNodes
	if err := c.client.get(ctx, address, "/nodes", nil, &nodes); err != nil {
		return nil, err
	}
	var topics lookupTopics
	if err := c.client.get(ctx, address, "/topics", nil, &topics); err != nil {
		return nil, err
	}

	// The daemons this lookupd would hand a consumer, spelled the way it
	// spells them: whatever each nsqd broadcast about itself, which is not
	// necessarily an address this machine can reach. Reporting it as given is
	// the point - a broadcast address only the cluster can resolve is a
	// misconfiguration this is the one place to see.
	advertised := make([]string, 0, len(nodes.Producers))
	for _, producer := range nodes.Producers {
		advertised = append(advertised, joinHostPort(producer.BroadcastAddress, producer.HTTPPort))
	}
	sort.Strings(advertised)

	return &model.Node{
		Name:      hostPort(address),
		Address:   hostPort(address),
		Version:   info.Version,
		Status:    model.NodeOnline,
		RateIn:    model.UnknownMetric,
		RateOut:   model.UnknownMetric,
		DiskUsage: model.UnknownMetric,
		Attributes: map[string]string{
			AttrProducerCount:   strconv.Itoa(len(nodes.Producers)),
			AttrDirectoryTopics: strconv.Itoa(len(topics.Topics)),
			AttrNodes:           strings.Join(advertised, ","),
		},
	}, nil
}

/*
 * NodeConfig reads what one nsqd is actually running with.
 *
 * Small, and honestly so: /info reports the handful of settings a client's
 * behaviour depends on, and /config exposes exactly one more - the nsqlookupd
 * addresses, which is the setting worth checking first when a consumer cannot
 * find a topic that plainly exists. Everything else nsqd was started with is a
 * flag it does not report.
 */
func (c *Conn) NodeConfig(ctx context.Context, address string) (map[string]string, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	target, err := c.nodeAt(address)
	if err != nil {
		return nil, err
	}

	var info nsqdInfo
	if err := c.client.get(ctx, target.address, "/info", nil, &info); err != nil {
		return nil, fmt.Errorf("%s: %w", hostPort(target.address), err)
	}

	config := map[string]string{
		"version":                   info.Version,
		"hostname":                  info.Hostname,
		"broadcast_address":         info.BroadcastAddress,
		"tcp_port":                  strconv.Itoa(info.TCPPort),
		"http_port":                 strconv.Itoa(info.HTTPPort),
		"start_time":                time.Unix(info.StartTime, 0).UTC().Format(time.RFC3339),
		"max_heartbeat_interval":    time.Duration(info.MaxHeartbeatInterval).String(),
		"max_output_buffer_size":    strconv.FormatInt(info.MaxOutputBufferSize, 10),
		"max_output_buffer_timeout": time.Duration(info.MaxOutputBufferTimeout).String(),
		"max_deflate_level":         strconv.Itoa(info.MaxDeflateLevel),
	}

	var lookupd []string
	if err := c.client.get(ctx, target.address, "/config/nsqlookupd_tcp_addresses",
		nil, &lookupd); err != nil {
		return nil, fmt.Errorf("%s: %w", hostPort(target.address), err)
	}
	config["nsqlookupd_tcp_addresses"] = strings.Join(lookupd, ", ")
	return config, nil
}

// DirectoryConfig is what the nsqlookupd tier is running with.
//
// One line per address rather than a merged document, because they are
// separate processes that can be on different versions - and a tier running
// two versions is worth seeing, since the registry format is what they share.
func (c *Conn) DirectoryConfig(ctx context.Context) (map[string]string, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	config := map[string]string{}
	for _, address := range c.config.lookupd {
		var info lookupInfo
		if err := c.client.get(ctx, address, "/info", nil, &info); err != nil {
			return nil, fmt.Errorf("%s: %w", hostPort(address), err)
		}
		config[hostPort(address)] = info.Version
	}
	return config, nil
}
