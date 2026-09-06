package solace

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Who is connected, and what they are doing.
 *
 * SEMP lists a Message VPN's clients with everything worth knowing about each
 * one, so this is one request. Two things about the list are worth saying out
 * loud because neither is obvious from the field names.
 *
 * The broker's own machinery appears here. A client whose name starts with "#"
 * is the broker talking to itself - the internal message bus, the REST
 * listener's own session, the MQTT bridge - and they are kept rather than
 * filtered, because a page that hid them would be hiding real connections
 * holding real resources. They are marked instead.
 *
 * There is no channel layer. A Solace client has flows, one per endpoint it is
 * bound to, and a flow is not a channel: it carries no prefetch of its own, it
 * is not something an operator closes, and it belongs to the endpoint as much
 * as to the client - which is why the queues page reports the bound count and
 * this one does not pretend to a multiplexing layer the protocol has not got.
 */

// clientRow is the shape the client collection answers with.
type clientRow struct {
	ClientName        string  `json:"clientName"`
	ClientAddress     string  `json:"clientAddress"`
	ClientUsername    string  `json:"clientUsername"`
	MsgVpnName        string  `json:"msgVpnName"`
	Description       string  `json:"description"`
	Platform          string  `json:"platform"`
	SoftwareVersion   string  `json:"softwareVersion"`
	Uptime            int64   `json:"uptime"`
	VirtualRouter     string  `json:"virtualRouter"`
	ClientProfileName string  `json:"clientProfileName"`
	ACLProfileName    string  `json:"aclProfileName"`
	SlowSubscriber    bool    `json:"slowSubscriber"`
	TLSDowngraded     bool    `json:"tlsDowngradedToPlainText"`
	DataRxByteCount   int64   `json:"dataRxByteCount"`
	DataTxByteCount   int64   `json:"dataTxByteCount"`
	RxByteRate        float64 `json:"rxByteRate"`
	TxByteRate        float64 `json:"txByteRate"`
	BindSuccessCount  int64   `json:"bindSuccessCount"`
}

const clientFields = "clientName,clientAddress,clientUsername,msgVpnName,description,platform," +
	"softwareVersion,uptime,virtualRouter,clientProfileName,aclProfileName,slowSubscriber," +
	"tlsDowngradedToPlainText,dataRxByteCount,dataTxByteCount,rxByteRate,txByteRate," +
	"bindSuccessCount"

// ListClientConnections lists what is connected to this Message VPN.
func (c *Conn) ListClientConnections(ctx context.Context, _ string) ([]*model.ClientConnection, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	// The namespace argument is ignored: a connection is one Message VPN, and
	// a listing that took another one would be reading outside its own scope.
	rows, err := listMonitor[clientRow](ctx, c.semp,
		"/msgVpns/"+segment(c.vpn)+"/clients?select="+clientFields)
	if err != nil {
		return nil, err
	}

	clients := make([]*model.ClientConnection, 0, len(rows))
	for _, row := range rows {
		host, port := splitAddress(row.ClientAddress)
		clients = append(clients, &model.ClientConnection{
			// The client name is the key rather than the address: one host can
			// hold twenty connections and the broker addresses each of them by
			// the name it chose or was given.
			Name:       row.ClientName,
			ClientName: row.ClientName,
			Namespace:  row.MsgVpnName,
			User:       row.ClientUsername,
			Node:       row.VirtualRouter,
			PeerHost:   host,
			PeerPort:   port,
			// The broker reports no protocol on a client, and there is no
			// honest way to derive one: an SMF session, an MQTT session and a
			// REST request all appear here with the same fields. The platform
			// string is what a reader gets instead, on the attributes.
			Protocol: "",
			State:    clientState(row),
			// Flows rather than channels, and they are not the same thing -
			// see the file comment. The count is the successful binds, which
			// is what the broker keeps.
			Channels:     int(row.BindSuccessCount),
			TLS:          false,
			RecvBytes:    row.DataRxByteCount,
			SendBytes:    row.DataTxByteCount,
			RecvByteRate: row.RxByteRate,
			SendByteRate: row.TxByteRate,
			Attributes: map[string]string{
				AttrClientPlatform: row.Platform,
				AttrClientVersion:  row.SoftwareVersion,
				AttrClientProfile:  row.ClientProfileName,
				AttrACLProfile:     row.ACLProfileName,
				AttrDescription:    row.Description,
				AttrUptimeSec:      strconv.FormatInt(row.Uptime, 10),
				AttrSlowSubscriber: strconv.FormatBool(row.SlowSubscriber),
				AttrTLSDowngraded:  strconv.FormatBool(row.TLSDowngraded),
				AttrInternal:       strconv.FormatBool(internalClient(row.ClientName)),
			},
		})
	}

	sort.Slice(clients, func(i, j int) bool { return clients[i].Name < clients[j].Name })
	return clients, nil
}

// Client attribute keys this driver fills in.
//
// A contract between this package and frontend/src/mq/solace/clients.ts, not
// part of the shared vocabulary.
const (
	AttrClientPlatform = "platform"
	AttrClientVersion  = "softwareVersion"
	AttrClientProfile  = "clientProfile"
	AttrACLProfile     = "aclProfile"
	AttrDescription    = "description"
	AttrUptimeSec      = "uptimeSec"
	AttrSlowSubscriber = "slowSubscriber"
	AttrTLSDowngraded  = "tlsDowngraded"
	// AttrInternal marks the broker talking to itself. Those connections are
	// listed rather than hidden - they hold real resources - and a reader
	// counting applications needs to be able to leave them out.
	AttrInternal = "internal"
)

/*
 * ListClientChannels is empty, and deliberately.
 *
 * A channel is AMQP 0-9-1's multiplexing layer and this protocol has not got
 * one. The nearest thing is a flow, and a flow is not a channel: it exists per
 * endpoint the client is bound to rather than per session, it carries no
 * prefetch of its own, and it is not something an operator closes. It is
 * reported as a bind count on the connection and as the bound count on the
 * queue, which is where it belongs.
 */
func (c *Conn) ListClientChannels(_ context.Context, _ string) ([]*model.ClientChannel, error) {
	return nil, nil
}

// clientState is the one word the broker's fields add up to.
//
// There is no state field on a client: it is in the list, so it is connected.
// What can be said beyond that is whether the broker has marked it as falling
// behind, which is the one condition worth seeing at a glance.
func clientState(row clientRow) string {
	if row.SlowSubscriber {
		return "slow"
	}
	return "connected"
}

// internalClient reports the broker's own machinery. Solace reserves a leading
// "#" for its own objects, and its internal sessions are named that way.
func internalClient(name string) bool {
	return strings.HasPrefix(name, "#")
}

// splitAddress takes the client's "host:port" apart. An address without a port
// keeps the whole string as the host rather than losing it.
func splitAddress(address string) (string, int) {
	colon := strings.LastIndexByte(address, ':')
	if colon < 0 {
		return address, 0
	}
	port, err := strconv.Atoi(address[colon+1:])
	if err != nil {
		return address, 0
	}
	return address[:colon], port
}
