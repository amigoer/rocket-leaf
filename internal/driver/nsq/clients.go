package nsq

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// Client attribute keys, on top of the shared ones.
const (
	AttrClientTopic   = "topic"
	AttrClientChannel = "channel"
	AttrReadyCount    = "readyCount"
	AttrFinishCount   = "finishCount"
	AttrUserAgent     = "userAgent"
	AttrSnappy        = "snappy"
	AttrClientNode    = "node"
	// AttrRole is which half of the picture a row is. The two are reported by
	// nsqd in different places and carry different figures, and a page that
	// mixed them would show a producer with a ready count of nothing and call
	// it stalled.
	AttrRole = "role"
	// AttrPublished is what a producer has published, per topic, since it
	// connected. Producers only: it is the whole of what one reports.
	AttrPublished = "published"
)

// The two roles a connected client can be in.
const (
	roleConsumer = "consumer"
	roleProducer = "producer"
)

// clientStates spells nsqd's connection state machine, which /stats reports as
// a number. Only one of them can appear in a channel's client list - a client
// is there because it subscribed - but the others are what the number means
// and a page showing a bare 3 would be showing nothing.
var clientStates = map[int]string{
	0: "init",
	1: "disconnected",
	2: "connected",
	3: "subscribed",
	4: "closing",
}

/*
 * ListClientConnections is everything holding a connection open to the
 * cluster, in the two places nsqd reports them.
 *
 * There is no single connection list. A consumer appears in the stats of the
 * channel it subscribed to and nowhere else, so this walks every topic and
 * channel on every daemon; a producer subscribes to nothing and appears
 * instead in the daemon's own producer list, with what it has published per
 * topic. Reading only the first was this driver's original mistake, and it
 * left the page asserting that producers could never be seen.
 *
 * One kind of client is still invisible, and no page can fix it: anything
 * publishing over HTTP. /pub is a request rather than a connection, so nsqd
 * has nothing to list once it has answered.
 *
 * The namespace argument is ignored. NSQ has no vhost, tenant or account for a
 * connection to belong to.
 */
func (c *Conn) ListClientConnections(ctx context.Context, _ string) ([]*model.ClientConnection, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	// The client lists are asked for explicitly and the runtime memory block
	// is refused. Both default the other way, and both are worth naming: the
	// per-channel client list is the largest thing in the response and this is
	// the one page that reads it, while the memory figures belong to the
	// cluster board and are dead weight here.
	values := url.Values{
		"format":          {"json"},
		"include_clients": {"true"},
		"include_mem":     {"false"},
	}
	perNode, err := eachNode(ctx, c.nodes, func(ctx context.Context, n node) (nsqdStats, error) {
		var stats nsqdStats
		err := c.client.get(ctx, n.address, "/stats", values, &stats)
		return stats, err
	})
	if err != nil {
		return nil, err
	}

	connections := make([]*model.ClientConnection, 0, 16)
	for index, stats := range perNode {
		daemon := hostPort(c.nodes[index].address)
		for _, topic := range stats.Topics {
			for _, channel := range topic.Channels {
				for _, client := range channel.Clients {
					connections = append(connections,
						describeConsumer(daemon, topic.Name, channel.Name, client))
				}
			}
		}
		for _, producer := range stats.Producers {
			connections = append(connections, describeProducer(daemon, producer))
		}
	}

	// Sorted, because the daemons answer concurrently and a client list that
	// reshuffled between refreshes would be unreadable.
	sort.Slice(connections, func(first, second int) bool {
		return connections[first].Name < connections[second].Name
	})
	return connections, nil
}

/*
 * ListClientChannels has nothing to report, and returns an empty list rather
 * than an error.
 *
 * An NSQ connection subscribes to exactly one channel and cannot multiplex:
 * there is no session layer beneath a connection the way an AMQP channel sits
 * inside one, so the topic and channel a client is reading are fields on the
 * connection above rather than rows of their own. The method exists because
 * ClientInspector is one interface; the clients board does not call it.
 */
func (c *Conn) ListClientChannels(_ context.Context, _ string) ([]*model.ClientChannel, error) {
	return []*model.ClientChannel{}, nil
}

// describeConsumer is a client that subscribed to a channel.
func describeConsumer(daemon, topic, channel string, client clientStats) *model.ClientConnection {
	connection := describeClient(daemon, client)
	connection.Attributes[AttrRole] = roleConsumer
	connection.Attributes[AttrClientTopic] = topic
	connection.Attributes[AttrClientChannel] = channel
	// What this consumer told nsqd it will accept. A zero here on a channel
	// with a backlog is the whole explanation for a consumer that is connected
	// and taking nothing.
	connection.Attributes[AttrReadyCount] = strconv.Itoa(client.ReadyCount)
	connection.Attributes[AttrFinishCount] = strconv.FormatUint(client.FinishCount, 10)
	// A consumer reads one channel and cannot multiplex, so there is never
	// more than one.
	connection.Channels = 1
	return connection
}

/*
 * describeProducer is a client holding a connection open to publish.
 *
 * It carries no topic of its own and no ready count: those are a
 * subscription's, and a producer has none. What it has instead is a count per
 * topic it has published to, which is the only thing on this page that says a
 * connection is doing anything at all.
 */
func describeProducer(daemon string, client clientStats) *model.ClientConnection {
	connection := describeClient(daemon, client)
	connection.Attributes[AttrRole] = roleProducer

	topics := make([]string, 0, len(client.PubCounts))
	published := make([]string, 0, len(client.PubCounts))
	var total uint64
	for _, count := range client.PubCounts {
		topics = append(topics, count.Topic)
		published = append(published, fmt.Sprintf("%s=%d", count.Topic, count.Count))
		total += count.Count
	}
	sort.Strings(topics)
	sort.Strings(published)

	connection.Attributes[AttrClientTopic] = strings.Join(topics, ",")
	connection.Attributes[AttrPublished] = strings.Join(published, ",")
	connection.Attributes[AttrMessageCount] = strconv.FormatUint(total, 10)
	return connection
}

// describeClient is what the two roles have in common: the socket, and who is
// on the other end of it.
func describeClient(daemon string, client clientStats) *model.ClientConnection {
	host, port := splitPeer(client.RemoteAddress)

	state := clientStates[client.State]
	if state == "" {
		state = strconv.Itoa(client.State)
	}

	return &model.ClientConnection{
		// The broker's own identifier form: who is connected, and to which
		// daemon. Both halves are needed - the same consumer process holds one
		// connection per nsqd it found.
		Name:       client.RemoteAddress + " -> " + daemon,
		ClientName: client.ClientID,
		Node:       daemon,
		PeerHost:   host,
		PeerPort:   port,
		// nsqd speaks one protocol and names its versions V1 and V2, which is
		// what the client reports here.
		Protocol:      "nsq " + client.Version,
		State:         state,
		TLS:           client.TLS,
		Cipher:        client.TLSCipherSuite,
		ConnectedAtMs: client.ConnectTS * 1000,
		Attributes: map[string]string{
			AttrInFlight:     strconv.Itoa(client.InFlightCount),
			AttrMessageCount: strconv.FormatUint(client.MessageCount, 10),
			AttrRequeued:     strconv.FormatUint(client.RequeueCount, 10),
			AttrUserAgent:    client.UserAgent,
			AttrHostname:     client.Hostname,
			AttrSnappy:       strconv.FormatBool(client.Snappy),
			AttrClientNode:   daemon,
		},
	}
}

// splitPeer takes a remote address apart. nsqd reports host:port, and an IPv6
// literal makes a naive split on the last colon wrong.
func splitPeer(address string) (string, int) {
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return address, 0
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return host, 0
	}
	return host, port
}
