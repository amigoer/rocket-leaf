package activemq

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// Connections, which the two products expose through unrelated shapes again.
//
// Artemis answers with JSON from an operation on the broker: one call returns
// every connection with its protocol, its user and its session count. Classic
// registers an MBean per connection under the connector that accepted it, so
// the list is a search and a read per result - and the connector's name is
// where the protocol comes from, because the connection itself does not say.

// Connection attribute keys, on top of the shared ones.
const (
	AttrSessions   = "sessions"
	AttrConnector  = "connector"
	AttrRemoteAddr = "remoteAddress"
	AttrCreated    = "created"
	AttrBlocked    = "blocked"
	AttrSlowClient = "slow"
)

// ListClientConnections lists what is holding a socket open on the broker.
func (c *Conn) ListClientConnections(ctx context.Context, _ string) ([]*model.ClientConnection, error) {
	if c.tiers.product == artemis {
		return c.artemisConnections(ctx)
	}
	return c.classicConnections(ctx)
}

// ListClientChannels is empty on both.
//
// A channel is AMQP 0-9-1's multiplexing layer and neither product has it. The
// nearest thing is a JMS session, and a session is not a channel: it carries no
// prefetch of its own, cannot be flow-controlled independently, and is not
// something an operator closes. It is reported as a count on the connection
// instead, which is where it belongs.
func (c *Conn) ListClientChannels(_ context.Context, _ string) ([]*model.ClientChannel, error) {
	return nil, nil
}

// CloseClientConnection disconnects one client.
//
// The name is the broker's own connection id, which is why the list carries it
// as the key rather than as a label: an address is not unique - one host can
// hold twenty connections - and closing by address would take all of them.
//
// The reason is dropped, and there is nowhere to put it: neither product's
// close operation carries one, so a message typed here would never reach the
// client. Saying so beats collecting it and discarding it silently.
func (c *Conn) CloseClientConnection(ctx context.Context, name, _ string) error {
	if c.tiers.product == artemis {
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"closeConnectionWithID(java.lang.String)", name))
		return err
	}
	// Classic has no broker-level close: the operation lives on the
	// connection's own MBean, which the list already found.
	mbean, err := c.classicConnectionMBean(ctx, name)
	if err != nil {
		return err
	}
	_, err = c.jolokia.call(ctx, execOperation(mbean, "stop()"))
	return err
}

// CloseUserConnections evicts every connection one identity holds.
//
// Artemis has the operation; Classic does not, so this walks its own listing
// and closes each match. That is not the same thing under concurrency - a
// connection opened between the list and the close survives - and it is still
// better than refusing, because an application with five instances is exactly
// the case this exists for.
func (c *Conn) CloseUserConnections(ctx context.Context, username, _ string) error {
	if username == "" {
		return fmt.Errorf("closing every connection needs a user to close them for")
	}
	if c.tiers.product == artemis {
		_, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(),
			"closeConnectionsForUser(java.lang.String)", username))
		return err
	}

	connections, err := c.classicConnections(ctx)
	if err != nil {
		return err
	}
	var failed error
	for _, connection := range connections {
		if connection.User != username {
			continue
		}
		if err := c.CloseClientConnection(ctx, connection.Name, ""); err != nil && failed == nil {
			failed = err
		}
	}
	return failed
}

// artemisConnection is one entry of listConnectionsAsJSON.
//
// Its field names are Artemis's own and are not the canonical model's, which
// is the usual story here - the mapping below is the whole reason this type
// exists rather than a map.
type artemisConnection struct {
	ConnectionID string `json:"connectionID"`
	ClientID     string `json:"clientID"`
	Users        string `json:"users"`
	// clientAddress is the field this version answers with. remoteAddress is
	// kept because older ones used that name and an empty peer column is
	// worse than reading both.
	ClientAddress string `json:"clientAddress"`
	RemoteAddress string `json:"remoteAddress"`
	SessionCount  int    `json:"sessionCount"`
	CreationTime  int64  `json:"creationTime"`
	// Implementation is the connection's Java class, and the only thing in
	// the answer that says what the client is speaking - there is no protocol
	// field. ActiveMQProtonRemotingConnection is AMQP, and so on.
	Implementation string `json:"implementation"`
}

// artemisProtocols maps the connection class onto the protocol's own name.
//
// Read off the implementation because Artemis's connection listing carries no
// protocol field, and "ActiveMQProtonRemotingConnection" in a column headed
// Protocol is the class name leaking into the UI.
var artemisProtocols = map[string]string{
	"ActiveMQProtonRemotingConnection": "AMQP",
	"RemotingConnectionImpl":           "CORE",
	"StompConnection":                  "STOMP",
	"MQTTConnection":                   "MQTT",
	"OpenWireConnection":               "OPENWIRE",
}

func protocolOf(implementation string) string {
	if name, known := artemisProtocols[implementation]; known {
		return name
	}
	return implementation
}

func (c *Conn) artemisConnections(ctx context.Context) ([]*model.ClientConnection, error) {
	value, err := c.jolokia.call(ctx, execOperation(c.names.brokerMBean(), "listConnectionsAsJSON()"))
	if err != nil {
		return nil, err
	}
	// A JSON string inside a JSON response, which is how every Artemis
	// *AsJSON operation answers - it has to be unwrapped before it parses.
	var encoded string
	if err := json.Unmarshal(value, &encoded); err != nil {
		return nil, fmt.Errorf("the connection list is not a json string: %w", err)
	}
	var entries []artemisConnection
	if err := json.Unmarshal([]byte(encoded), &entries); err != nil {
		return nil, fmt.Errorf("the connection list did not parse: %w", err)
	}

	connections := make([]*model.ClientConnection, 0, len(entries))
	for _, entry := range entries {
		address := entry.ClientAddress
		if address == "" {
			address = entry.RemoteAddress
		}
		host, port := splitHostPort(address)
		attributes := map[string]string{
			AttrProduct:  string(artemis),
			AttrSessions: strconv.Itoa(entry.SessionCount),
		}
		if entry.CreationTime > 0 {
			attributes[AttrCreated] = timeOf(entry.CreationTime)
		}
		if address != "" {
			attributes[AttrRemoteAddr] = address
		}

		connections = append(connections, &model.ClientConnection{
			Name:       entry.ConnectionID,
			ClientName: entry.ClientID,
			User:       entry.Users,
			PeerHost:   host,
			PeerPort:   port,
			Protocol:   protocolOf(entry.Implementation),
			State:      "running",
			Channels:   entry.SessionCount,
			Attributes: attributes,
		})
	}
	sortConnections(connections)
	return connections, nil
}

// classicConnectionAttributes is the read set for one connection MBean.
var classicConnectionAttributes = []string{
	"RemoteAddress", "UserName", "ClientId", "Active", "Blocked", "Slow",
	"Connected", "SessionCount", "DispatchQueueSize",
}

func (c *Conn) classicConnections(ctx context.Context) ([]*model.ClientConnection, error) {
	found, err := c.classicConnectionMBeans(ctx)
	if err != nil {
		return nil, err
	}

	requests := make([]request, 0, len(found)*len(classicConnectionAttributes))
	for _, mbean := range found {
		for _, attribute := range classicConnectionAttributes {
			requests = append(requests, readAttribute(mbean, attribute))
		}
	}
	values, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}

	connections := make([]*model.ClientConnection, 0, len(found))
	for i, mbean := range found {
		read := attributeSet(classicConnectionAttributes, values[i*len(classicConnectionAttributes):])
		_, keys, err := parseObjectName(mbean)
		if err != nil {
			continue
		}
		name := keys["connectionName"]
		if name == "" {
			continue
		}

		host, port := splitHostPort(stringOr(read["RemoteAddress"]))
		attributes := map[string]string{
			AttrProduct: string(classic),
			// The connector that accepted it, which is where the protocol
			// comes from: a Classic connection MBean does not say what it
			// speaks, and the connector it arrived on does.
			AttrConnector:  keys["connectorName"],
			AttrRemoteAddr: stringOr(read["RemoteAddress"]),
		}
		putInt(attributes, AttrSessions, read["SessionCount"])
		putBool(attributes, AttrBlocked, read["Blocked"])
		putBool(attributes, AttrSlowClient, read["Slow"])
		putString(attributes, AttrClientID, read["ClientId"])

		state := "running"
		if attributes[AttrBlocked] == "true" {
			state = "blocked"
		}

		connections = append(connections, &model.ClientConnection{
			Name:       name,
			ClientName: stringOr(read["ClientId"]),
			User:       stringOr(read["UserName"]),
			PeerHost:   host,
			PeerPort:   port,
			Protocol:   keys["connectorName"],
			State:      state,
			Channels:   intOr(read["SessionCount"], 0),
			Attributes: attributes,
		})
	}
	sortConnections(connections)
	return connections, nil
}

// classicConnectionMBeans searches every connector for its open connections.
func (c *Conn) classicConnectionMBeans(ctx context.Context) ([]string, error) {
	found, err := c.jolokia.search(ctx, fmt.Sprintf(
		"%s:type=Broker,brokerName=%s,connector=clientConnectors,connectorName=*,connectionViewType=*,connectionName=*",
		classicDomain, c.names.broker))
	if err != nil {
		return nil, err
	}

	// The same connection is registered once per view type - by address and by
	// client id - so a plain listing shows every client twice.
	seen := make(map[string]bool, len(found))
	unique := make([]string, 0, len(found))
	for _, raw := range found {
		_, keys, err := parseObjectName(raw)
		if err != nil || keys["connectionName"] == "" {
			continue
		}
		if keys["connectionViewType"] != "clientId" && keys["connectionViewType"] != "remoteAddress" {
			continue
		}
		// Prefer the remoteAddress view, which is the one whose connectionName
		// is the peer rather than an application-chosen string that may repeat.
		if keys["connectionViewType"] != "remoteAddress" {
			continue
		}
		if seen[keys["connectionName"]] {
			continue
		}
		seen[keys["connectionName"]] = true
		unique = append(unique, raw)
	}
	sort.Strings(unique)
	return unique, nil
}

func (c *Conn) classicConnectionMBean(ctx context.Context, name string) (string, error) {
	found, err := c.classicConnectionMBeans(ctx)
	if err != nil {
		return "", err
	}
	for _, raw := range found {
		_, keys, err := parseObjectName(raw)
		if err != nil {
			continue
		}
		if keys["connectionName"] == name {
			return raw, nil
		}
	}
	return "", fmt.Errorf("no open connection named %q", name)
}

// splitHostPort takes apart the address forms both brokers use, which are not
// quite host:port: Artemis reports "/127.0.0.1:52134" and Classic
// "tcp://127.0.0.1:52134".
func splitHostPort(address string) (string, int) {
	trimmed := address
	if index := strings.Index(trimmed, "://"); index >= 0 {
		trimmed = trimmed[index+3:]
	}
	trimmed = strings.TrimPrefix(trimmed, "/")

	colon := strings.LastIndex(trimmed, ":")
	if colon < 0 {
		return trimmed, 0
	}
	port, err := strconv.Atoi(trimmed[colon+1:])
	if err != nil {
		return trimmed, 0
	}
	return trimmed[:colon], port
}

func sortConnections(connections []*model.ClientConnection) {
	sort.SliceStable(connections, func(i, j int) bool {
		return connections[i].Name < connections[j].Name
	})
}
