package solace

import (
	"context"
	"strconv"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * The broker, and the Message VPN this connection is reading.
 *
 * This page is a broker page more than a cluster page, and that is the
 * family's doing rather than the driver's. A Solace deployment scales by
 * adding brokers that mesh with each other, and a redundancy pair is not two
 * nodes: the two appliances share one virtual router, only one of them is
 * active, and the standby answers nothing. So there is one row here, and a
 * list of nodes would be a list of one however it were assembled.
 *
 * # Two units in one object
 *
 * A Message VPN reports msgSpoolUsage in bytes and maxMsgSpoolUsage in
 * megabytes, on the same object, with names that differ by three letters. A
 * percentage computed without scaling is out by a factor of a million and
 * reads as a broker that is using nothing, which is exactly the figure an
 * operator would be looking at this page to check.
 */

// Overview and node attribute keys, on top of the shared ones.
const (
	AttrVersion       = "version"
	AttrRedundancy    = "redundancyEnabled"
	AttrSpoolMaxMb    = "spoolMaxMb"
	AttrSpoolUsedByte = "spoolUsedBytes"
	AttrSpoolMsgCount = "spoolMsgCount"
	AttrClientCount   = "clientCount"
	AttrEndpointCount = "topicEndpointCount"
	AttrQueueCount    = "queueCount"
	AttrVpnState      = "msgVpnState"
	AttrMsgVPN        = "msgVpn"
)

// brokerStats is the broker's own half of the page, read from the SEMP root.
type brokerStats struct {
	Version           string  `json:"version"`
	RxMsgRate         float64 `json:"rxMsgRate"`
	TxMsgRate         float64 `json:"txMsgRate"`
	MaxMsgSpoolUsage  int64   `json:"guaranteedMsgingMaxMsgSpoolUsage"`
	RedundancyEnabled bool    `json:"serviceRedundancyEnabled"`
}

// vpnStats is the Message VPN's half.
type vpnStats struct {
	MsgVpnName     string  `json:"msgVpnName"`
	State          string  `json:"state"`
	Enabled        bool    `json:"enabled"`
	MsgSpoolUsage  int64   `json:"msgSpoolUsage"`
	MaxMsgSpool    int64   `json:"maxMsgSpoolUsage"`
	MsgSpoolMsgCnt int64   `json:"msgSpoolMsgCount"`
	RxMsgRate      float64 `json:"rxMsgRate"`
	TxMsgRate      float64 `json:"txMsgRate"`
	MaxConnections int64   `json:"maxConnectionCount"`
}

// ListNodes returns the broker, and only the broker.
//
// One row, and it is not a shortcut. A redundancy pair shares one virtual
// router and only one half is ever active; a mesh of brokers is reached by
// connecting to each of them, and SEMP on this one describes this one.
func (c *Conn) ListNodes(ctx context.Context) ([]*model.Node, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}
	node.ID = 1
	return []*model.Node{node}, nil
}

// NodeDetail re-reads the broker. Any address answers, because there is only
// one node and the caller got its address from this driver.
func (c *Conn) NodeDetail(ctx context.Context, _ string) (*model.Node, error) {
	node, err := c.brokerNode(ctx)
	if err != nil {
		return nil, err
	}
	node.ID = 1
	return node, nil
}

// ClusterOverview is the header figures.
func (c *Conn) ClusterOverview(ctx context.Context) (*model.ClusterOverview, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	var (
		broker    brokerStats
		vpn       vpnStats
		queues    int
		endpoints int
		clients   int
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return c.semp.monitorGet(groupCtx, "?select="+brokerFields, &broker) })
	group.Go(func() error {
		return c.semp.monitorGet(groupCtx,
			"/msgVpns/"+segment(c.vpn)+"?select="+vpnFields, &vpn)
	})
	group.Go(func() error {
		count, err := c.vpnCollectionCount(groupCtx, c.vpn, "queues")
		queues = count
		return err
	})
	group.Go(func() error {
		count, err := c.vpnCollectionCount(groupCtx, c.vpn, "topicEndpoints")
		endpoints = count
		return err
	})
	group.Go(func() error {
		count, err := c.vpnCollectionCount(groupCtx, c.vpn, "clients")
		clients = count
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return &model.ClusterOverview{
		Name:        c.config.semp,
		TotalNodes:  1,
		OnlineNodes: 1,
		// Queues, because those are the endpoints an operator creates and
		// names. Topic endpoints are counted separately below.
		Destinations: queues,
		// Topic endpoints, which are the closest thing this family keeps to a
		// subscription the broker owns: one is a durable subscription to
		// exactly one topic, and its name is that topic. A Solace broker has
		// no consumer group anywhere.
		Subscriptions: endpoints,
		AvgDiskUsage:  spoolPercent(vpn.MsgSpoolUsage, vpn.MaxMsgSpool),
		Attributes: map[string]string{
			AttrMsgVPN:        c.vpn,
			AttrVpnState:      vpnStateOf(vpn),
			AttrVersion:       broker.Version,
			AttrRedundancy:    strconv.FormatBool(broker.RedundancyEnabled),
			AttrSpoolUsedByte: strconv.FormatInt(vpn.MsgSpoolUsage, 10),
			AttrSpoolMaxMb:    strconv.FormatInt(vpn.MaxMsgSpool, 10),
			AttrSpoolMsgCount: strconv.FormatInt(vpn.MsgSpoolMsgCnt, 10),
			AttrClientCount:   strconv.Itoa(clients),
			AttrQueueCount:    strconv.Itoa(queues),
			AttrEndpointCount: strconv.Itoa(endpoints),
		},
	}, nil
}

const brokerFields = "version,rxMsgRate,txMsgRate,guaranteedMsgingMaxMsgSpoolUsage," +
	"serviceRedundancyEnabled"

const vpnFields = "msgVpnName,state,enabled,msgSpoolUsage,maxMsgSpoolUsage,msgSpoolMsgCount," +
	"rxMsgRate,txMsgRate,maxConnectionCount"

// brokerNode is the one row this page has.
func (c *Conn) brokerNode(ctx context.Context) (*model.Node, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	var broker brokerStats
	if err := c.semp.monitorGet(ctx, "?select="+brokerFields, &broker); err != nil {
		return nil, err
	}
	var vpn vpnStats
	if err := c.semp.monitorGet(ctx,
		"/msgVpns/"+segment(c.vpn)+"?select="+vpnFields, &vpn); err != nil {
		return nil, err
	}

	status := model.NodeOnline
	if !vpn.Enabled || vpn.State != "up" {
		// The broker is answering and the Message VPN this connection reads is
		// not serving, which is neither healthy nor gone.
		status = model.NodeWarning
	}

	return &model.Node{
		// SEMP v2 reports no name for the broker anywhere - not at the root,
		// not in the about resources - so the address a profile dialled is the
		// only honest identifier. Inventing one from the hostname would be
		// printing this machine's DNS rather than the broker's name.
		Name:      c.config.semp,
		Address:   c.config.semp,
		Cluster:   c.vpn,
		Version:   broker.Version,
		Status:    status,
		RateIn:    int(broker.RxMsgRate),
		RateOut:   int(broker.TxMsgRate),
		DiskUsage: spoolPercent(vpn.MsgSpoolUsage, vpn.MaxMsgSpool),
		Attributes: map[string]string{
			AttrMsgVPN:        c.vpn,
			AttrVpnState:      vpnStateOf(vpn),
			AttrVersion:       broker.Version,
			AttrRedundancy:    strconv.FormatBool(broker.RedundancyEnabled),
			AttrSpoolUsedByte: strconv.FormatInt(vpn.MsgSpoolUsage, 10),
			AttrSpoolMaxMb:    strconv.FormatInt(vpn.MaxMsgSpool, 10),
			AttrSpoolMsgCount: strconv.FormatInt(vpn.MsgSpoolMsgCnt, 10),
			// The broker's own spool cap, which is the ceiling every Message
			// VPN's share is taken out of.
			"brokerSpoolMaxMb": strconv.FormatInt(broker.MaxMsgSpoolUsage, 10),
			"maxConnections":   strconv.FormatInt(vpn.MaxConnections, 10),
		},
	}, nil
}

func vpnStateOf(vpn vpnStats) string {
	if !vpn.Enabled {
		return "disabled"
	}
	if vpn.State == "" {
		return "unknown"
	}
	return vpn.State
}

/*
 * spoolPercent scales the two figures onto each other.
 *
 * usedBytes is what the Message VPN reports as msgSpoolUsage and it is in
 * bytes; maxMb is maxMsgSpoolUsage on the same object and it is in megabytes.
 * That is the broker's own pair of units, measured rather than assumed - a
 * Message VPN holding 1572 bytes across its queues reports msgSpoolUsage 1572
 * beside maxMsgSpoolUsage 1500 - and dividing them unscaled reports a full
 * broker as empty.
 */
func spoolPercent(usedBytes, maxMb int64) int {
	if maxMb <= 0 {
		return model.UnknownMetric
	}
	maxBytes := maxMb * 1024 * 1024
	percent := (usedBytes * 100) / maxBytes
	if percent > 100 {
		return 100
	}
	return int(percent)
}
