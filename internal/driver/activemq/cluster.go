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

// The broker, and what it knows about its neighbours.
//
// This page is a broker page more than a cluster page, and the difference is
// the family's rather than the driver's. A JMS broker is a unit: destinations
// live on the one that owns them, clients connect to it, and clustering here
// is a bridge between brokers rather than a set of nodes sharing a namespace.
// Classic bridges with network connectors and Artemis with cluster
// connections, so what the list shows is this broker plus whatever it is
// bridged to - and on the ordinary single-broker deployment, one row.
//
// Neither reports a rate. Both keep cumulative counters, and dividing two
// samples of those would be this app's arithmetic presented as the broker's.

// Node and overview attribute keys, on top of the shared ones.
const (
	AttrUptime        = "uptime"
	AttrNodeID        = "nodeId"
	AttrPersistence   = "persistenceEnabled"
	AttrDataDirectory = "dataDirectory"
	AttrStorePercent  = "storePercent"
	AttrMemoryLimit   = "memoryLimit"
	AttrTotalMessages = "totalMessages"
	AttrTotalEnqueued = "totalEnqueued"
	AttrTotalDequeued = "totalDequeued"
	AttrConnections   = "connections"
	AttrConsumers     = "consumers"
	AttrProducers     = "producers"
	AttrAcceptors     = "acceptors"
	AttrClustered     = "clustered"
	AttrHAPolicy      = "haPolicy"
	AttrBackup        = "backup"
	AttrJournalType   = "journalType"
	AttrSecurity      = "securityEnabled"
	AttrTempPercent   = "tempPercent"
)

// classicBrokerAttributes is the read set for the broker itself.
var classicBrokerAttributes = []string{
	"BrokerName", "BrokerId", "BrokerVersion", "Uptime", "UptimeMillis",
	"Persistent", "DataDirectory", "StorePercentUsage", "TempPercentUsage",
	"MemoryPercentUsage", "MemoryLimit", "CurrentConnectionsCount",
	"TotalConsumerCount", "TotalProducerCount", "TotalMessageCount",
	"TotalEnqueueCount", "TotalDequeueCount", "TotalQueuesCount",
	"TotalTopicsCount", "Slave",
}

var artemisBrokerAttributes = []string{
	"Name", "NodeID", "Version", "Uptime", "UptimeMillis", "PersistenceEnabled",
	"JournalDirectory", "DiskStoreUsage", "MaxDiskUsage", "AddressMemoryUsagePercentage",
	"GlobalMaxSize", "ConnectionCount", "TotalConsumerCount", "TotalMessageCount",
	"TotalMessagesAdded", "TotalMessagesAcknowledged", "AddressCount", "QueueCount",
	"Clustered", "HAPolicy", "Backup", "Active", "JournalType", "SecurityEnabled",
	"Acceptors", "ClusterConnectionNames",
}

// ListNodes returns the broker, and the brokers it bridges to where it has any.
func (c *Conn) ListNodes(ctx context.Context) ([]*model.Node, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}
	nodes := []*model.Node{node}

	peers, err := c.bridgedBrokers(ctx)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, peers...)
	sort.SliceStable(nodes[1:], func(i, j int) bool { return nodes[i+1].Name < nodes[j+1].Name })
	return nodes, nil
}

// NodeDetail re-reads one node. Only the broker itself can answer in detail: a
// bridged peer is known by the connector that points at it and is not reachable
// through this connection's management plane.
func (c *Conn) NodeDetail(ctx context.Context, address string) (*model.Node, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}
	if address == "" || address == node.Address || address == node.Name {
		return node, nil
	}
	return nil, fmt.Errorf("%q is a bridged broker and answers on its own console", address)
}

// ClusterOverview is the header figures.
func (c *Conn) ClusterOverview(ctx context.Context) (*model.ClusterOverview, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	overview := &model.ClusterOverview{
		Name:         node.Cluster,
		TotalNodes:   len(nodes),
		OnlineNodes:  1,
		AvgDiskUsage: node.DiskUsage,
		Attributes:   map[string]string{},
	}
	if overview.Name == "" {
		overview.Name = node.Name
	}
	for key, value := range node.Attributes {
		overview.Attributes[key] = value
	}

	if destinations, err := strconv.Atoi(node.Attributes["destinationCount"]); err == nil {
		overview.Destinations = destinations
	} else {
		overview.Destinations = model.UnknownMetric
	}
	if subscriptions, err := c.ListSubscriptions(ctx); err == nil {
		overview.Subscriptions = len(subscriptions)
	} else {
		overview.Subscriptions = model.UnknownMetric
	}
	return overview, nil
}

func (c *Conn) brokerNode(ctx context.Context) (*model.Node, error) {
	attributes := classicBrokerAttributes
	if c.tiers.product == artemis {
		attributes = artemisBrokerAttributes
	}

	requests := make([]request, 0, len(attributes))
	for _, attribute := range attributes {
		requests = append(requests, readAttribute(c.names.brokerMBean(), attribute))
	}
	// Tolerant: the attribute sets differ between minor versions, and one
	// absent figure must not cost the page every other one.
	values, _, err := c.jolokia.batchTolerant(ctx, requests)
	if err != nil {
		return nil, err
	}
	read := attributeSet(attributes, values)

	if c.tiers.product == artemis {
		return c.artemisNode(read), nil
	}
	return c.classicNode(read), nil
}

func (c *Conn) classicNode(read map[string]json.RawMessage) *model.Node {
	attributes := map[string]string{AttrProduct: string(classic)}
	putString(attributes, AttrUptime, read["Uptime"])
	putString(attributes, AttrNodeID, read["BrokerId"])
	putBool(attributes, AttrPersistence, read["Persistent"])
	putString(attributes, AttrDataDirectory, read["DataDirectory"])
	putInt(attributes, AttrStorePercent, read["StorePercentUsage"])
	putInt(attributes, AttrTempPercent, read["TempPercentUsage"])
	putInt(attributes, AttrMemoryPercent, read["MemoryPercentUsage"])
	putInt(attributes, AttrMemoryLimit, read["MemoryLimit"])
	putInt(attributes, AttrConnections, read["CurrentConnectionsCount"])
	putInt(attributes, AttrConsumerCount, read["TotalConsumerCount"])
	putInt(attributes, AttrProducerCount, read["TotalProducerCount"])
	putInt(attributes, AttrTotalMessages, read["TotalMessageCount"])
	putInt(attributes, AttrTotalEnqueued, read["TotalEnqueueCount"])
	putInt(attributes, AttrTotalDequeued, read["TotalDequeueCount"])
	attributes["destinationCount"] = strconv.Itoa(
		intOr(read["TotalQueuesCount"], 0) + intOr(read["TotalTopicsCount"], 0))
	// A Classic secondary is passive: it holds the store lock's other side and
	// answers nothing until it takes over.
	putBool(attributes, AttrBackup, read["Slave"])

	name := stringOr(read["BrokerName"])
	return &model.Node{
		Name:       name,
		Address:    c.config.console,
		Version:    stringOr(read["BrokerVersion"]),
		Status:     model.NodeOnline,
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		DiskUsage:  intOr(read["StorePercentUsage"], model.UnknownMetric),
		Attributes: attributes,
	}
}

func (c *Conn) artemisNode(read map[string]json.RawMessage) *model.Node {
	attributes := map[string]string{AttrProduct: string(artemis)}
	putString(attributes, AttrUptime, read["Uptime"])
	putString(attributes, AttrNodeID, read["NodeID"])
	putBool(attributes, AttrPersistence, read["PersistenceEnabled"])
	putString(attributes, AttrDataDirectory, read["JournalDirectory"])
	putInt(attributes, AttrMemoryPercent, read["AddressMemoryUsagePercentage"])
	putInt(attributes, AttrMemoryLimit, read["GlobalMaxSize"])
	putInt(attributes, AttrConnections, read["ConnectionCount"])
	putInt(attributes, AttrConsumerCount, read["TotalConsumerCount"])
	putInt(attributes, AttrTotalMessages, read["TotalMessageCount"])
	putInt(attributes, AttrTotalEnqueued, read["TotalMessagesAdded"])
	putInt(attributes, AttrTotalDequeued, read["TotalMessagesAcknowledged"])
	putBool(attributes, AttrClustered, read["Clustered"])
	putString(attributes, AttrHAPolicy, read["HAPolicy"])
	putBool(attributes, AttrBackup, read["Backup"])
	putString(attributes, AttrJournalType, read["JournalType"])
	putBool(attributes, AttrSecurity, read["SecurityEnabled"])
	attributes["destinationCount"] = strconv.Itoa(intOr(read["AddressCount"], 0))
	if acceptors := acceptorNames(read["Acceptors"]); len(acceptors) > 0 {
		attributes[AttrAcceptors] = strings.Join(acceptors, ",")
	}

	// DiskStoreUsage is a fraction of MaxDiskUsage rather than a percentage of
	// the disk, so it has to be scaled - reported raw it reads as 0% on a
	// broker that is 16% full.
	disk := model.UnknownMetric
	var usage float64
	if raw := read["DiskStoreUsage"]; raw != nil {
		if err := json.Unmarshal(raw, &usage); err == nil {
			disk = int(usage * 100)
		}
	}

	status := model.NodeOnline
	var active bool
	if raw := read["Active"]; raw != nil {
		_ = json.Unmarshal(raw, &active)
		if !active {
			// A backup that has not taken over answers management calls and
			// serves no clients, which is neither online nor gone.
			status = model.NodeWarning
		}
	}

	return &model.Node{
		Name:       stringOr(read["Name"]),
		Address:    c.config.console,
		Version:    stringOr(read["Version"]),
		Status:     status,
		RateIn:     model.UnknownMetric,
		RateOut:    model.UnknownMetric,
		DiskUsage:  disk,
		Attributes: attributes,
	}
}

// bridgedBrokers lists what this broker forwards to.
//
// Classic's network connectors and Artemis's cluster connections are the same
// idea: a link to another broker, named and configured here. They are listed
// as nodes because that is what a reader is looking for on this page - what
// else is in the deployment - and marked as bridges, because this connection
// cannot say whether they are up.
func (c *Conn) bridgedBrokers(ctx context.Context) ([]*model.Node, error) {
	pattern := c.names.searchPattern("cluster-connections")
	if c.tiers.product == classic {
		pattern = fmt.Sprintf("%s:type=Broker,brokerName=%s,connector=networkConnectors,*",
			classicDomain, c.names.broker)
	}

	found, err := c.jolokia.search(ctx, pattern)
	if err != nil {
		return nil, err
	}

	nodes := make([]*model.Node, 0, len(found))
	for _, raw := range found {
		_, keys, err := parseObjectName(raw)
		if err != nil {
			continue
		}
		name := keys["name"]
		if name == "" {
			name = keys["networkConnectorName"]
		}
		if name == "" {
			continue
		}
		nodes = append(nodes, &model.Node{
			Name:    name,
			Cluster: "bridge",
			// Unknown rather than online: the link is declared here and the
			// broker at the other end answers on its own console, which this
			// connection has no way to reach.
			Status:     model.NodeUnknown,
			RateIn:     model.UnknownMetric,
			RateOut:    model.UnknownMetric,
			DiskUsage:  model.UnknownMetric,
			Attributes: map[string]string{AttrProduct: string(c.tiers.product), "bridge": "true"},
		})
	}
	return nodes, nil
}

// NodeConfig is the broker's effective settings.
//
// Everything the management tree reports about itself, which for Artemis is
// most of broker.xml and for Classic is the subset the BrokerView exposes.
// Filtered to scalars: the tree also carries destination lists and connector
// maps, which belong on their own pages rather than in a settings table.
func (c *Conn) NodeConfig(ctx context.Context, _ string) (map[string]string, error) {
	value, err := c.jolokia.call(ctx, request{Type: "read", MBean: c.names.brokerMBean()})
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(value, &all); err != nil {
		return nil, fmt.Errorf("the broker's attributes are not an object: %w", err)
	}

	config := make(map[string]string, len(all))
	for key, raw := range all {
		if scalar, ok := scalarOf(raw); ok {
			config[key] = scalar
		}
	}
	return config, nil
}

// DirectoryConfig is empty: there is no discovery tier here. Brokers find each
// other through connectors each one declares, so listing them again under
// another heading would be the cluster page twice.
func (c *Conn) DirectoryConfig(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

// Census is the broker's own running totals.
func (c *Conn) Census(ctx context.Context) (*model.BrokerCensus, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}

	census := &model.BrokerCensus{
		ClusterName: node.Name,
		Version:     node.Version,
		Total:       int64(atoiOr(node.Attributes[AttrTotalMessages], 0)),
		Connections: atoiOr(node.Attributes[AttrConnections], 0),
		Consumers:   atoiOr(node.Attributes[AttrConsumerCount], 0),
		Queues:      atoiOr(node.Attributes["destinationCount"], 0),
		// Ready and unacknowledged are not split at this level by either
		// broker: the distinction exists per destination and the broker-wide
		// counters do not carry it. Left at zero rather than duplicating the
		// total into one of them and inventing the other.
	}
	// Runtime version is the JVM's, which neither broker MBean reports.
	return census, nil
}

// scalarOf keeps the settings a table can show and drops the rest.
//
// Two filters, not one. The obvious one is a JSON object or array, which is a
// destination list or a connector map rather than a setting. The second is a
// JSON *string* whose content is itself a structure: Artemis reports
// AcceptorsAsJSON, ConnectorsAsJSON and a whole Status document that way, and
// a filter looking only at the outer type lets a page of JSON into a settings
// table. Those belong on the pages that draw them.
func scalarOf(raw json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", false
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return "", false
	}
	if trimmed[0] != '"' {
		return trimmed, true
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	if inner := strings.TrimSpace(text); inner != "" && (inner[0] == '{' || inner[0] == '[') {
		return "", false
	}
	return text, true
}

func atoiOr(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// acceptorNames pulls the names out of Artemis's acceptor listing.
//
// Not a list of strings, which is what it looks like from its name: each entry
// is [name, factory class, {params}], so reading it as strings yields nothing
// and the board shows an empty column on a broker with five acceptors.
func acceptorNames(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}
	var entries [][]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		var name string
		if err := json.Unmarshal(entry[0], &name); err == nil && name != "" {
			names = append(names, name)
		}
	}
	return names
}
