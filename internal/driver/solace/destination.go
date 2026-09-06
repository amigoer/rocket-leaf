package solace

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Queues, which are the only destinations on this page.
 *
 * A Message VPN has two kinds of endpoint and only one of them belongs here. A
 * queue is named by whoever creates it and receives either by name or through
 * the topic subscriptions attached to it; a topic endpoint's name is its
 * routing, and there is nothing to decide about it beyond the topic it is
 * bound to. So topic endpoints are on the routing page beside the
 * subscriptions, and this page is queues.
 *
 * # The depth is not a field
 *
 * This is the one thing about SEMP that has to be got right, and the obvious
 * answer is wrong. A queue reports spooledMsgCount, which reads exactly like a
 * current depth and is not one: it is a statistic counting every message ever
 * spooled, the action API's clearStats sets it to zero on a queue holding a
 * quarter of a million messages, and a queue drained to empty keeps reporting
 * its high-water mark. Nothing else on the object carries the depth either.
 *
 * What does carry it is the message collection's own meta.count, which SEMP
 * fills with how many are there rather than how many it returned - so one
 * request asking for a single message answers the question. The same is true
 * of the bound consumers: there is no bindCount on the queue, and the flow
 * collection's count is the number attached right now.
 *
 * That makes a listing two extra requests per queue, which is the price of the
 * figures being true. They are run together with a bounded amount of
 * concurrency rather than one after another, because the alternative on a
 * broker with a hundred queues is a page that takes a hundred round trips.
 */

// Attribute keys this driver writes into model.Destination.Attributes.
//
// A contract between this package and frontend/src/mq/solace/destinations.ts,
// not part of the shared vocabulary. Another family's "accessType" means
// whatever that family's driver decided it means.
const (
	AttrAccessType  = "accessType"
	AttrPermission  = "permission"
	AttrOwner       = "owner"
	AttrDurable     = "durable"
	AttrIngress     = "ingressEnabled"
	AttrEgress      = "egressEnabled"
	AttrSpoolUsage  = "spoolUsageBytes"
	AttrMaxSpool    = "maxSpoolUsageMb"
	AttrMaxMsgSize  = "maxMsgSizeBytes"
	AttrPartitions  = "partitionCount"
	AttrVirtualRtr  = "virtualRouter"
	AttrByManagemnt = "createdByManagement"

	// AttrDeadMsgQueue is how a queue says where its undelivered messages go.
	// It is read on the dead-letter page too: this pointer is the whole of
	// what makes an ordinary queue a dead message queue here.
	AttrDeadMsgQueue  = "deadMsgQueue"
	AttrMaxRedelivery = "maxRedeliveryCount"
	AttrRespectTTL    = "respectTtlEnabled"
	AttrMaxTTL        = "maxTtlSec"
	AttrRespectDmq    = "respectDmqEligibleEnabled"

	AttrRedelivered = "redeliveredMsgCount"
	AttrUnacked     = "txUnackedMsgCount"
	AttrToDmqTTL    = "ttlExpiredToDmqMsgCount"
	AttrToDmqRetry  = "maxRedeliveryToDmqMsgCount"
	// AttrSpooledTotal is spooledMsgCount, kept under a name that says what it
	// is. It is a lifetime counter rather than the depth, and putting it on
	// the detail panel under its own name is what stops the next reader
	// reaching for it as one.
	AttrSpooledTotal = "spooledMsgCountTotal"
)

// queueFields is the fixed read set for the listing.
//
// Fixed rather than a bare read: a queue's monitored description is a hundred
// fields, most of them per-failure-mode discard counters, and this page reads
// twenty. The saving is on every row of every refresh.
const queueFields = "queueName,accessType,permission,owner,durable,ingressEnabled," +
	"egressEnabled,msgSpoolUsage,maxMsgSpoolUsage,maxMsgSize,partitionCount," +
	"deadMsgQueue,maxRedeliveryCount,respectTtlEnabled,maxTtl,respectDmqEligibleEnabled," +
	"rxMsgRate,txMsgRate,redeliveredMsgCount,txUnackedMsgCount,spooledMsgCount," +
	"maxTtlExpiredToDmqMsgCount,maxRedeliveryExceededToDmqMsgCount,virtualRouter," +
	"createdByManagement"

// queueRow is the shape the queue collection answers with.
type queueRow struct {
	QueueName        string `json:"queueName"`
	AccessType       string `json:"accessType"`
	Permission       string `json:"permission"`
	Owner            string `json:"owner"`
	Durable          bool   `json:"durable"`
	IngressEnabled   bool   `json:"ingressEnabled"`
	EgressEnabled    bool   `json:"egressEnabled"`
	MsgSpoolUsage    int64  `json:"msgSpoolUsage"`
	MaxMsgSpoolUsage int64  `json:"maxMsgSpoolUsage"`
	MaxMsgSize       int64  `json:"maxMsgSize"`
	PartitionCount   int    `json:"partitionCount"`

	DeadMsgQueue       string `json:"deadMsgQueue"`
	MaxRedeliveryCount int    `json:"maxRedeliveryCount"`
	RespectTTLEnabled  bool   `json:"respectTtlEnabled"`
	MaxTTL             int64  `json:"maxTtl"`
	RespectDmqEligible bool   `json:"respectDmqEligibleEnabled"`

	RxMsgRate           float64 `json:"rxMsgRate"`
	TxMsgRate           float64 `json:"txMsgRate"`
	RedeliveredMsgCount int64   `json:"redeliveredMsgCount"`
	TxUnackedMsgCount   int64   `json:"txUnackedMsgCount"`
	SpooledMsgCount     int64   `json:"spooledMsgCount"`

	TTLExpiredToDmq     int64  `json:"maxTtlExpiredToDmqMsgCount"`
	RedeliveryToDmq     int64  `json:"maxRedeliveryExceededToDmqMsgCount"`
	VirtualRouter       string `json:"virtualRouter"`
	CreatedByManagement bool   `json:"createdByManagement"`
}

// countConcurrency is how many of the per-queue counts run at once.
//
// Bounded rather than unbounded: a listing on a broker with several hundred
// queues would otherwise open several hundred sockets to it at the same
// moment, which is a management API being used as a load generator.
const countConcurrency = 8

// ListDestinations returns the Message VPN's queues.
func (c *Conn) ListDestinations(ctx context.Context, _ model.DestinationFilter) ([]*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	// The filter's namespace is ignored: a connection is one Message VPN, and
	// a listing that took another one would be reading outside its own scope.
	rows, err := listMonitor[queueRow](ctx, c.semp,
		"/msgVpns/"+segment(c.vpn)+"/queues?select="+queueFields)
	if err != nil {
		return nil, err
	}

	destinations := make([]*model.Destination, 0, len(rows))
	for index, row := range rows {
		destination := c.destinationOf(row)
		destination.ID = index + 1
		destinations = append(destinations, destination)
	}
	if err := c.fillCounts(ctx, destinations); err != nil {
		return nil, err
	}

	sort.Slice(destinations, func(i, j int) bool {
		return destinations[i].Ref.Name < destinations[j].Ref.Name
	})
	for index, destination := range destinations {
		destination.ID = index + 1
	}
	return destinations, nil
}

// DestinationDetail re-reads one queue.
func (c *Conn) DestinationDetail(ctx context.Context, ref model.DestinationRef) (*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	var row queueRow
	path := "/msgVpns/" + segment(c.vpn) + "/queues/" + segment(ref.Name) + "?select=" + queueFields
	if err := c.semp.monitorGet(ctx, path, &row); err != nil {
		if notFound(err) {
			return nil, fmt.Errorf("%s has no queue named %s", c.vpn, ref.Name)
		}
		return nil, err
	}

	destination := c.destinationOf(row)
	destination.ID = 1
	if err := c.fillCounts(ctx, []*model.Destination{destination}); err != nil {
		return nil, err
	}
	return destination, nil
}

/*
 * fillCounts reads the two figures the queue object does not carry.
 *
 * Both are collection counts rather than fields, and both are read with
 * count=1 for the same reason: SEMP's meta.count is how many exist, not how
 * many came back, so asking for one message answers "how many are spooled"
 * without transferring the rest.
 *
 * A queue that has gone between the listing and this read is left with its
 * counts unknown rather than failing the page: the listing is a snapshot and a
 * deletion in the middle of one is ordinary.
 */
func (c *Conn) fillCounts(ctx context.Context, destinations []*model.Destination) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(countConcurrency)

	for _, destination := range destinations {
		group.Go(func() error {
			depth, err := c.collectionCount(ctx, destination.Ref.Name, "msgs")
			if err != nil {
				return err
			}
			destination.Depth = depth
			return nil
		})
		group.Go(func() error {
			bound, err := c.collectionCount(ctx, destination.Ref.Name, "txFlows")
			if err != nil {
				return err
			}
			destination.Subscribers = int(bound)
			return nil
		})
	}
	return group.Wait()
}

// collectionCount is how many entries a queue's sub-collection holds.
//
// A queue that has gone between the listing and this read leaves the figure
// unknown rather than failing the page. That is not a rare case: a listing
// walks every queue and then reads two figures off each of them, so it is
// racing anybody who deletes one - and SEMP reports that deletion three
// different ways depending on which sub-collection was asked for, which is
// what vanished exists to flatten.
func (c *Conn) collectionCount(ctx context.Context, queue, collection string) (int64, error) {
	path := monitorAPI + "/msgVpns/" + segment(c.vpn) + "/queues/" + segment(queue) +
		"/" + collection + "?count=1"
	_, meta, err := c.semp.do(ctx, "GET", path, nil)
	if err != nil {
		if vanished(err) {
			return model.UnknownMetric, nil
		}
		return 0, err
	}
	return int64(meta.Count), nil
}

func (c *Conn) destinationOf(row queueRow) *model.Destination {
	return &model.Destination{
		Ref: model.DestinationRef{Namespace: c.vpn, Name: row.QueueName},
		// A Solace queue is not divided into parts a reader browses. A
		// partitioned queue exists in 10.x and its partitions are a scaling
		// unit for consumers rather than an addressable read range, so the
		// count is an attribute and this stays unknown.
		Partitions:  model.UnknownMetric,
		Subscribers: model.UnknownMetric,
		Depth:       model.UnknownMetric,
		RateIn:      int(row.RxMsgRate),
		RateOut:     int(row.TxMsgRate),
		Attributes: map[string]string{
			AttrAccessType:  row.AccessType,
			AttrPermission:  row.Permission,
			AttrOwner:       row.Owner,
			AttrDurable:     strconv.FormatBool(row.Durable),
			AttrIngress:     strconv.FormatBool(row.IngressEnabled),
			AttrEgress:      strconv.FormatBool(row.EgressEnabled),
			AttrSpoolUsage:  strconv.FormatInt(row.MsgSpoolUsage, 10),
			AttrMaxSpool:    strconv.FormatInt(row.MaxMsgSpoolUsage, 10),
			AttrMaxMsgSize:  strconv.FormatInt(row.MaxMsgSize, 10),
			AttrPartitions:  strconv.Itoa(row.PartitionCount),
			AttrVirtualRtr:  row.VirtualRouter,
			AttrByManagemnt: strconv.FormatBool(row.CreatedByManagement),

			AttrDeadMsgQueue:  row.DeadMsgQueue,
			AttrMaxRedelivery: strconv.Itoa(row.MaxRedeliveryCount),
			AttrRespectTTL:    strconv.FormatBool(row.RespectTTLEnabled),
			AttrMaxTTL:        strconv.FormatInt(row.MaxTTL, 10),
			AttrRespectDmq:    strconv.FormatBool(row.RespectDmqEligible),

			AttrRedelivered:  strconv.FormatInt(row.RedeliveredMsgCount, 10),
			AttrUnacked:      strconv.FormatInt(row.TxUnackedMsgCount, 10),
			AttrToDmqTTL:     strconv.FormatInt(row.TTLExpiredToDmq, 10),
			AttrToDmqRetry:   strconv.FormatInt(row.RedeliveryToDmq, 10),
			AttrSpooledTotal: strconv.FormatInt(row.SpooledMsgCount, 10),
		},
	}
}
