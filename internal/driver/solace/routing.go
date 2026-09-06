package solace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Routing, which on this family is what makes a queue receive anything at all.
 *
 * A Solace publisher names a topic and never a queue. What decides where the
 * message lands is the set of subscriptions the broker matches it against, and
 * a durable endpoint gets its share two ways:
 *
 *   - a queue carries topic subscriptions, added and removed independently of
 *     the queue itself. Several to a queue, wildcards allowed, and the queue
 *     works perfectly well with none - it just never receives anything except
 *     what is sent to it by name.
 *   - a topic endpoint has no subscriptions at all, because its name is its
 *     subscription. It takes what is published to the topic it is called after
 *     and there is nothing else to configure.
 *
 * That is a topology rather than a setting on the reader, which is what puts
 * it on the same page RabbitMQ's exchanges and bindings and Service Bus's
 * rules are on. The mapping onto that page is close enough to be worth stating
 * exactly:
 *
 *   - an exchange is a topic endpoint. It is the object whose whole
 *     configuration is where messages come from, and unlike a queue there is
 *     nothing to decide about it beyond that.
 *   - a binding is a topic subscription. Its source is the topic pattern, its
 *     destination is the queue, and its routing key is that same pattern -
 *     because that is what the column means: the thing a message is matched
 *     against to decide whether it takes this edge.
 *   - the binding's properties key is the subscription topic, which is
 *     genuinely the handle: a subscription has no name and is deleted by the
 *     topic it subscribes to.
 *
 * One thing does not map and is refused rather than approximated: a topic
 * endpoint has no exchange type. There is no direct, fanout, topic or headers
 * here - the matching is the broker's own and is not configurable.
 */

// Binding argument keys this driver fills in.
//
// A contract between this package and frontend/src/mq/solace/routing.ts, not
// part of the shared vocabulary.
const (
	// ArgWildcard says whether this subscription matches more than one topic.
	// "*" matches one level and ">" matches the rest, and the difference
	// between a subscription that takes one topic and one that takes a whole
	// tree is the thing most worth seeing on this page.
	ArgWildcard = "wildcard"
	// ArgQueueDepth is what the destination is holding, so a subscription that
	// is quietly filling a queue nobody drains is visible here rather than
	// only on the queues page.
	ArgQueueDepth = "queueDepth"
)

// Topic endpoint attribute keys, on top of the shared destination ones.
const (
	AttrEndpointTopic = "endpointTopic"
)

// topicEndpointDetail is the shape the topic endpoint collection answers with
// for this page.
type topicEndpointDetail struct {
	TopicEndpointName string  `json:"topicEndpointName"`
	AccessType        string  `json:"accessType"`
	Permission        string  `json:"permission"`
	Owner             string  `json:"owner"`
	Durable           bool    `json:"durable"`
	IngressEnabled    bool    `json:"ingressEnabled"`
	EgressEnabled     bool    `json:"egressEnabled"`
	DeadMsgQueue      string  `json:"deadMsgQueue"`
	MaxRedelivery     int     `json:"maxRedeliveryCount"`
	MsgSpoolUsage     int64   `json:"msgSpoolUsage"`
	RxMsgRate         float64 `json:"rxMsgRate"`
	TxMsgRate         float64 `json:"txMsgRate"`
}

const topicEndpointFields = "topicEndpointName,accessType,permission,owner,durable," +
	"ingressEnabled,egressEnabled,deadMsgQueue,maxRedeliveryCount,msgSpoolUsage,rxMsgRate,txMsgRate"

/*
 * ListExchanges is the Message VPN's topic endpoints.
 *
 * Only topic endpoints, and that is the mapping rather than a filter. A queue
 * is named by whoever made it and can receive by name; a topic endpoint's name
 * is the topic it takes, which makes it the one endpoint here whose entire
 * definition is where its messages come from.
 */
func (c *Conn) ListExchanges(ctx context.Context, _ string) ([]*model.Destination, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	rows, err := listMonitor[topicEndpointDetail](ctx, c.semp,
		"/msgVpns/"+segment(c.vpn)+"/topicEndpoints?select="+topicEndpointFields)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*model.Destination, 0, len(rows))
	for _, row := range rows {
		endpoints = append(endpoints, &model.Destination{
			Ref:         model.DestinationRef{Namespace: c.vpn, Name: row.TopicEndpointName},
			Partitions:  model.UnknownMetric,
			Subscribers: model.UnknownMetric,
			Depth:       model.UnknownMetric,
			RateIn:      int(row.RxMsgRate),
			RateOut:     int(row.TxMsgRate),
			Attributes: map[string]string{
				// The topic is the name, and it is repeated under its own key
				// so a board can label the column for what it means rather
				// than for where it came from.
				AttrEndpointTopic: row.TopicEndpointName,
				AttrAccessType:    row.AccessType,
				AttrPermission:    row.Permission,
				AttrOwner:         row.Owner,
				AttrDurable:       strconv.FormatBool(row.Durable),
				AttrIngress:       strconv.FormatBool(row.IngressEnabled),
				AttrEgress:        strconv.FormatBool(row.EgressEnabled),
				AttrDeadMsgQueue:  row.DeadMsgQueue,
				AttrMaxRedelivery: strconv.Itoa(row.MaxRedelivery),
				AttrSpoolUsage:    strconv.FormatInt(row.MsgSpoolUsage, 10),
			},
		})
	}

	if err := c.fillEndpointDepths(ctx, endpoints); err != nil {
		return nil, err
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Ref.Name < endpoints[j].Ref.Name
	})
	return endpoints, nil
}

// fillEndpointDepths reads what each topic endpoint is holding, for the reason
// the queue listing does: SEMP puts no depth on the object.
func (c *Conn) fillEndpointDepths(ctx context.Context, endpoints []*model.Destination) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(countConcurrency)
	for _, endpoint := range endpoints {
		group.Go(func() error {
			path := monitorAPI + "/msgVpns/" + segment(c.vpn) + "/topicEndpoints/" +
				segment(endpoint.Ref.Name) + "/msgs?count=1"
			_, meta, err := c.semp.do(ctx, http.MethodGet, path, nil)
			if err != nil {
				if notFound(err) {
					return nil
				}
				return err
			}
			endpoint.Depth = int64(meta.Count)
			return nil
		})
	}
	return group.Wait()
}

// subscriptionRow is one topic subscription on one queue.
type subscriptionRow struct {
	QueueName         string `json:"queueName"`
	SubscriptionTopic string `json:"subscriptionTopic"`
}

/*
 * ListBindings is every topic subscription on every queue.
 *
 * Read from the configuration half rather than the monitored one: a
 * subscription is a declaration, it exists whether or not anything has ever
 * matched it, and that is exactly the state this page has to be able to show -
 * a subscription that matches nothing looks identical to a working one from
 * every other page in the app.
 *
 * The queue depth travels with the binding, because the pair is the answer to
 * the question people come here with: which subscription is filling that
 * queue.
 */
func (c *Conn) ListBindings(ctx context.Context, _ string) ([]*model.Binding, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	queues, err := c.ListDestinations(ctx, model.DestinationFilter{})
	if err != nil {
		return nil, err
	}
	depths := make(map[string]int64, len(queues))
	for _, queue := range queues {
		depths[queue.Ref.Name] = queue.Depth
	}

	// One call per queue rather than one for the Message VPN, because SEMP
	// offers no collection of every subscription in a VPN: they live under the
	// queue that carries them and nowhere else.
	found := make([][]subscriptionRow, len(queues))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(countConcurrency)
	for index, queue := range queues {
		group.Go(func() error {
			rows, listErr := listConfig[subscriptionRow](groupCtx, c.semp,
				"/msgVpns/"+segment(c.vpn)+"/queues/"+segment(queue.Ref.Name)+
					"/subscriptions?select=queueName,subscriptionTopic")
			if listErr != nil {
				if notFound(listErr) {
					// The queue went between the listing and this read, which
					// is ordinary during a refresh.
					return nil
				}
				return listErr
			}
			found[index] = rows
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	bindings := make([]*model.Binding, 0, len(queues))
	for index, rows := range found {
		for _, row := range rows {
			depth := depths[queues[index].Ref.Name]
			bindings = append(bindings, &model.Binding{
				Namespace: c.vpn,
				// The topic pattern is the source: it is where the messages
				// come from, and there is no object between it and the queue.
				Source:          row.SubscriptionTopic,
				Destination:     row.QueueName,
				DestinationKind: SourceQueue,
				RoutingKey:      row.SubscriptionTopic,
				// A subscription has no name and is deleted by its topic, so
				// the topic is genuinely the handle.
				PropertiesKey: row.SubscriptionTopic,
				Arguments: map[string]string{
					ArgWildcard:   strconv.FormatBool(hasWildcard(row.SubscriptionTopic)),
					ArgQueueDepth: strconv.FormatInt(depth, 10),
				},
			})
		}
	}

	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Destination != bindings[j].Destination {
			return bindings[i].Destination < bindings[j].Destination
		}
		return bindings[i].Source < bindings[j].Source
	})
	for index, binding := range bindings {
		binding.ID = index + 1
	}
	return bindings, nil
}

/*
 * DeclareExchange creates a topic endpoint.
 *
 * A type is refused rather than ignored: there is no direct, fanout, topic or
 * headers here, and what decides where a message goes is the broker's own
 * topic matching against the endpoint's name.
 */
func (c *Conn) DeclareExchange(ctx context.Context, spec model.ExchangeSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	if kind := strings.TrimSpace(spec.Type); kind != "" {
		return fmt.Errorf(
			"a topic endpoint has no exchange type; %q means nothing here, and what "+
				"decides where a message goes is the topic it is named after", kind)
	}
	if spec.Transient || spec.AutoDelete {
		// A non-durable topic endpoint exists, and it is created by a client
		// binding to one rather than by an administrator: it lives for as long
		// as that client's flow does. There is nothing for this to make.
		return errors.New(
			"a topic endpoint made here is durable; a non-durable one is created by the " +
				"client that binds to it and goes when that client does")
	}

	name := strings.TrimSpace(spec.Name)
	if err := validQueueName(name); err != nil {
		return err
	}
	err := c.semp.configSend(ctx, http.MethodPost,
		"/msgVpns/"+segment(c.vpn)+"/topicEndpoints", map[string]any{
			"topicEndpointName": name,
			"accessType":        "exclusive",
			"permission":        "consume",
			"ingressEnabled":    true,
			"egressEnabled":     true,
		})
	if alreadyExists(err) {
		return fmt.Errorf("%s already has a topic endpoint named %s", c.vpn, name)
	}
	return err
}

// RemoveExchange deletes a topic endpoint, and whatever it was holding.
func (c *Conn) RemoveExchange(ctx context.Context, _, name string) error {
	if err := c.live(); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("no name given")
	}
	err := c.semp.configSend(ctx, http.MethodDelete,
		"/msgVpns/"+segment(c.vpn)+"/topicEndpoints/"+segment(trimmed), nil)
	if notFound(err) {
		return fmt.Errorf("%s has no topic endpoint named %s", c.vpn, trimmed)
	}
	return err
}

/*
 * DeclareBinding adds a topic subscription to a queue.
 *
 * The routing key is the subscription and the source is ignored, which reads
 * oddly against RabbitMQ and is right here: there is no exchange between the
 * topic and the queue, so the two fields are the same thing and the driver
 * takes whichever the caller filled in.
 *
 * Nothing already spooled moves. A subscription added now attracts what is
 * published from now on, which is worth knowing before somebody adds one and
 * waits for a backlog to appear.
 */
func (c *Conn) DeclareBinding(ctx context.Context, binding model.Binding) error {
	if err := c.live(); err != nil {
		return err
	}
	queue := strings.TrimSpace(binding.Destination)
	if queue == "" {
		return errors.New("no queue named for the subscription")
	}
	topic := subscriptionTopicOf(binding)
	if topic == "" {
		return errors.New("no topic given to subscribe to")
	}

	err := c.semp.configSend(ctx, http.MethodPost,
		"/msgVpns/"+segment(c.vpn)+"/queues/"+segment(queue)+"/subscriptions",
		map[string]any{"subscriptionTopic": topic})
	switch {
	case alreadyExists(err):
		return fmt.Errorf("%s already subscribes to %s", queue, topic)
	case notFound(err):
		return fmt.Errorf("%s has no queue named %s", c.vpn, queue)
	default:
		return err
	}
}

// RemoveBinding drops a topic subscription from a queue.
func (c *Conn) RemoveBinding(ctx context.Context, binding model.Binding) error {
	if err := c.live(); err != nil {
		return err
	}
	queue := strings.TrimSpace(binding.Destination)
	if queue == "" {
		return errors.New("no queue named for the subscription")
	}
	topic := subscriptionTopicOf(binding)
	if topic == "" {
		return errors.New("no topic given to unsubscribe from")
	}

	err := c.semp.configSend(ctx, http.MethodDelete,
		"/msgVpns/"+segment(c.vpn)+"/queues/"+segment(queue)+"/subscriptions/"+segment(topic), nil)
	if notFound(err) {
		return fmt.Errorf("%s does not subscribe to %s", queue, topic)
	}
	return err
}

// subscriptionTopicOf takes the topic out of whichever field the caller used.
//
// Three of them mean the same thing here - the properties key is the handle, the
// routing key is what a message is matched against, and the source is where it
// comes from - because with no exchange between the topic and the queue there
// is only one string to carry.
func subscriptionTopicOf(binding model.Binding) string {
	for _, candidate := range []string{binding.PropertiesKey, binding.RoutingKey, binding.Source} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// hasWildcard reports whether a subscription matches more than one topic.
//
// The two characters are positional rather than free: "*" stands for one whole
// level and ">" for the rest of them, and neither means anything in the middle
// of a level. That is close enough to a glob to be misread, which is why the
// page marks a wildcard subscription rather than leaving a reader to spot it.
func hasWildcard(topic string) bool {
	return strings.ContainsAny(topic, "*>")
}
