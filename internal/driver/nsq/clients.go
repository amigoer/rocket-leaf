package nsq

import (
	"context"
	"net"
	"net/url"
	"sort"
	"strconv"

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
 * ListClientConnections is every consumer connected to the cluster.
 *
 * There is no connection list in NSQ. A client appears in the stats of the
 * channel it subscribed to and nowhere else, so this walks every topic and
 * channel on every daemon and collects them - which also means a connection
 * that has not subscribed to anything yet is invisible, and a producer is
 * invisible always. Both are worth knowing rather than being papered over: the
 * page is who is consuming, not who is connected.
 *
 * The namespace argument is ignored. NSQ has no vhost, tenant or account for a
 * connection to belong to.
 */
func (c *Conn) ListClientConnections(ctx context.Context, _ string) ([]*model.ClientConnection, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	// include_clients is on here and off everywhere else: the per-channel
	// client list is the largest thing in the response, and this is the one
	// page that needs it.
	perNode, err := eachNode(ctx, c.nodes, func(ctx context.Context, n node) (nsqdStats, error) {
		var stats nsqdStats
		err := c.client.get(ctx, n.address, "/stats", url.Values{"format": {"json"}}, &stats)
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
						describeClient(daemon, topic.Name, channel.Name, client))
				}
			}
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

func describeClient(daemon, topic, channel string, client clientStats) *model.ClientConnection {
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
		Protocol: "nsq " + client.Version,
		State:    state,
		// A connection reads one channel and cannot multiplex, so there is
		// never more than one.
		Channels:      1,
		TLS:           client.TLS,
		Cipher:        client.TLSCipherSuite,
		ConnectedAtMs: client.ConnectTS * 1000,
		Attributes: map[string]string{
			AttrClientTopic:   topic,
			AttrClientChannel: channel,
			// What this consumer told nsqd it will accept. A zero here on a
			// channel with a backlog is the whole explanation for a consumer
			// that is connected and taking nothing.
			AttrReadyCount:   strconv.Itoa(client.ReadyCount),
			AttrInFlight:     strconv.Itoa(client.InFlightCount),
			AttrMessageCount: strconv.FormatUint(client.MessageCount, 10),
			AttrFinishCount:  strconv.FormatUint(client.FinishCount, 10),
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
