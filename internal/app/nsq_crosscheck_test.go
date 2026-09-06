package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every NSQ board against the raw HTTP API.
 *
 * The other live tests compare one code path against another and can only show
 * that the two agree. This compares what the app computes against the
 * daemons' own answers, fetched by a client that shares no code with the
 * driver - its own request, its own structs, its own arithmetic.
 *
 * It matters more here than the shape of the driver suggests. Almost every
 * figure this family shows is a sum the driver performs: a topic's depth is
 * its own queue plus every channel's, on every daemon carrying it; a channel's
 * backlog is the same sum one level down; the overview counts distinct objects
 * across daemons that each know only their own. Every one of those is
 * arithmetic that can be subtly wrong while staying plausible, and the driver
 * testing itself would produce the same wrong number twice.
 *
 * The seed is required rather than optional. Comparing zero against zero would
 * pass whatever the driver did.
 *
 * Everything compared exactly is a seeded object, because the cluster is
 * shared: the driver package's own live tests run against it at the same time
 * and create and delete topics of their own. Nothing publishes to or consumes
 * from MQS.SEED.*, so those figures hold still. The one comparison that cannot
 * be narrowed that way is the cluster overview, which counts everything -
 * that one waits for the cluster to stop moving first.
 */

const (
	crossNSQD1    = "http://127.0.0.1:4151"
	crossNSQD2    = "http://127.0.0.1:4153"
	crossLookupd1 = "http://127.0.0.1:4161"
)

// The seeded objects, as scripts/e2e-nsq-seed.sh builds them.
const (
	nsqSeedOrders = "MQS.SEED.orders"
	nsqSeedAudit  = "MQS.SEED.audit"
	nsqSeedPaused = "MQS.SEED.paused"
	nsqSeedEvents = "MQS.SEED.events"
)

/*
 * nsqProbe reads a daemon without going through the driver.
 *
 * Deliberately its own structs rather than a call into internal/driver: a
 * crosscheck that reused the driver's client and its folds would compare the
 * driver against itself, which is what these tests exist not to do. The
 * Accept header is here for the same reason the driver has one - without it
 * nsqd answers in the pre-1.0 envelope - and that is a fact about nsqd rather
 * than about the driver, so repeating it is correct.
 */
type nsqProbe struct{ address string }

type probeStats struct {
	Health string `json:"health"`
	// Producers are the clients holding a connection open to publish. They are
	// reported here rather than under a channel, because a producer subscribes
	// to nothing - and reading only the channels is exactly the mistake this
	// crosscheck caught in the driver.
	Producers []struct {
		RemoteAddress string `json:"remote_address"`
	} `json:"producers"`
	Topics []struct {
		Name         string `json:"topic_name"`
		Depth        int64  `json:"depth"`
		MessageCount uint64 `json:"message_count"`
		Paused       bool   `json:"paused"`
		Channels     []struct {
			Name          string `json:"channel_name"`
			Depth         int64  `json:"depth"`
			InFlightCount int    `json:"in_flight_count"`
			DeferredCount int    `json:"deferred_count"`
			ClientCount   int    `json:"client_count"`
			Paused        bool   `json:"paused"`
			Clients       []struct {
				RemoteAddress string `json:"remote_address"`
				ReadyCount    int    `json:"ready_count"`
			} `json:"clients"`
		} `json:"channels"`
	} `json:"topics"`
}

func (p nsqProbe) stats(t *testing.T) probeStats {
	t.Helper()
	var stats probeStats
	p.get(t, "/stats", url.Values{"format": {"json"}}, &stats)
	return stats
}

func (p nsqProbe) get(t *testing.T, path string, query url.Values, out any) {
	t.Helper()
	endpoint := p.address + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Accept", "application/vnd.nsq; version=1.0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s answered %d", endpoint, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		t.Fatalf("decoding %s: %v", endpoint, err)
	}
}

// requireSeed skips locally and fails in CI when the environment holds nothing
// worth comparing. Zero against zero passes whatever the driver did.
func requireNSQSeed(t *testing.T, stats probeStats) {
	t.Helper()
	for _, topic := range stats.Topics {
		if topic.Name == nsqSeedOrders {
			return
		}
	}
	e2e.Missing(t, "the cluster holds no seeded topics; run `npm run e2e:nsq:seed` "+
		"- comparing zero against zero would pass whatever the driver did")
}

// depthOf adds a topic up the way the boards say it is added up: the topic's
// own queue plus every channel's, across every daemon carrying it.
func nsqDepthOf(name string, all []probeStats) (topicDepth, channelDepth int64, channels []string) {
	seen := map[string]struct{}{}
	for _, stats := range all {
		for _, topic := range stats.Topics {
			if topic.Name != name {
				continue
			}
			topicDepth += topic.Depth
			for _, channel := range topic.Channels {
				channelDepth += channel.Depth
				seen[channel.Name] = struct{}{}
			}
		}
	}
	for channel := range seen {
		channels = append(channels, channel)
	}
	sort.Strings(channels)
	return topicDepth, channelDepth, channels
}

/*
 * The topics board, figure by figure.
 *
 * The seed is built for this: MQS.SEED.orders is on both daemons with a
 * different count on each, so a driver reading one and calling it the cluster
 * gets a number that is plausible and wrong. MQS.SEED.paused is the other
 * half - the only state in NSQ where a topic depth is not zero.
 */
func TestCrossCheckNSQTopics(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq crosscheck", true))

	raw := []probeStats{
		nsqProbe{crossNSQD1}.stats(t),
		nsqProbe{crossNSQD2}.stats(t),
	}
	requireNSQSeed(t, raw[0])

	destinations, err := stack.destinations.List(nsqContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	byName := map[string]*model.Destination{}
	for _, entry := range destinations {
		byName[entry.Ref.Name] = entry
	}

	for _, name := range []string{nsqSeedOrders, nsqSeedAudit, nsqSeedPaused, nsqSeedEvents} {
		entry := byName[name]
		if entry == nil {
			t.Errorf("%s is on the cluster and missing from the app's listing", name)
			continue
		}
		topicDepth, channelDepth, channels := nsqDepthOf(name, raw)

		if entry.Depth != topicDepth+channelDepth {
			t.Errorf("%s: the app shows a depth of %d; the daemons hold %d in the topic and %d in its channels",
				name, entry.Depth, topicDepth, channelDepth)
		}
		if entry.Attribute("topicDepth") != fmt.Sprint(topicDepth) {
			t.Errorf("%s: topicDepth = %q, the daemons say %d",
				name, entry.Attribute("topicDepth"), topicDepth)
		}
		if entry.Attribute("channelDepth") != fmt.Sprint(channelDepth) {
			t.Errorf("%s: channelDepth = %q, the daemons say %d",
				name, entry.Attribute("channelDepth"), channelDepth)
		}
		if entry.Subscribers != len(channels) {
			t.Errorf("%s: the app counts %d channels, the daemons have %v",
				name, entry.Subscribers, channels)
		}
		if got := entry.Attribute("channels"); got != strings.Join(channels, ",") {
			t.Errorf("%s: channels = %q, the daemons have %v", name, got, channels)
		}
	}

	// The seed puts orders on both daemons and audit on one. A driver that
	// read the first daemon and stopped would agree with every figure above
	// and still be wrong about this.
	if nodes := byName[nsqSeedOrders].Attribute("nodes"); !strings.Contains(nodes, ",") {
		t.Errorf("%s names %q; the seed puts it on both daemons", nsqSeedOrders, nodes)
	}
	if nodes := byName[nsqSeedAudit].Attribute("nodes"); strings.Contains(nodes, ",") {
		t.Errorf("%s names %q; the seed puts it on one daemon", nsqSeedAudit, nodes)
	}

	// The paused topic is the only one holding messages in its own queue,
	// because nothing is being copied into its channel.
	paused := byName[nsqSeedPaused]
	if paused.Attribute("paused") != "true" {
		t.Errorf("%s is paused on the daemon and the app says %q",
			nsqSeedPaused, paused.Attribute("paused"))
	}
	if paused.Attribute("topicDepth") == "0" {
		t.Errorf("%s reports nothing held; the pause is what makes it the one topic that does",
			nsqSeedPaused)
	}
}

/*
 * The channels board, against the same daemons.
 *
 * A channel is identified by its topic as well as its own name, and the seed
 * has "analytics" under two topics precisely so a driver that keyed on the
 * name alone folds two channels into one row with a backlog belonging to
 * neither.
 */
func TestCrossCheckNSQChannels(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq crosscheck channels", true))

	raw := []probeStats{
		nsqProbe{crossNSQD1}.stats(t),
		nsqProbe{crossNSQD2}.stats(t),
	}
	requireNSQSeed(t, raw[0])

	// The daemons' own answer, folded independently of the driver.
	type channelFigures struct {
		depth    int64
		inFlight int
		deferred int
		clients  int
		paused   bool
	}
	wanted := map[string]*channelFigures{}
	for _, stats := range raw {
		for _, topic := range stats.Topics {
			for _, channel := range topic.Channels {
				key := topic.Name + "/" + channel.Name
				figures := wanted[key]
				if figures == nil {
					figures = &channelFigures{}
					wanted[key] = figures
				}
				figures.depth += channel.Depth
				figures.inFlight += channel.InFlightCount
				figures.deferred += channel.DeferredCount
				figures.clients += channel.ClientCount
				figures.paused = figures.paused || channel.Paused
			}
		}
	}

	subscriptions, err := stack.consumers.List(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	shown := map[string]*model.Subscription{}
	for _, entry := range subscriptions {
		shown[entry.Ref.Namespace+"/"+entry.Ref.Name] = entry
	}

	for key, figures := range wanted {
		// Seeded channels only. The driver package's live tests run against
		// the same cluster and create channels of their own, so a comparison
		// over everything would be racing them.
		if !strings.HasPrefix(key, "MQS.SEED.") {
			continue
		}
		entry := shown[key]
		if entry == nil {
			t.Errorf("%s is on the cluster and missing from the app's listing", key)
			continue
		}
		if entry.Backlog != figures.depth {
			t.Errorf("%s: backlog = %d, the daemons hold %d", key, entry.Backlog, figures.depth)
		}
		if entry.Members != figures.clients {
			t.Errorf("%s: %d consumers, the daemons report %d", key, entry.Members, figures.clients)
		}
		if entry.Attribute("inFlight") != fmt.Sprint(figures.inFlight) {
			t.Errorf("%s: inFlight = %q, the daemons say %d",
				key, entry.Attribute("inFlight"), figures.inFlight)
		}
		if entry.Attribute("deferred") != fmt.Sprint(figures.deferred) {
			t.Errorf("%s: deferred = %q, the daemons say %d",
				key, entry.Attribute("deferred"), figures.deferred)
		}
		if entry.Attribute("paused") != fmt.Sprint(figures.paused) {
			t.Errorf("%s: paused = %q, the daemons say %v",
				key, entry.Attribute("paused"), figures.paused)
		}
	}
	// The seed's point: the same channel name under two topics has to be two
	// rows with two backlogs.
	orders, audit := shown[nsqSeedOrders+"/analytics"], shown[nsqSeedAudit+"/analytics"]
	if orders == nil || audit == nil {
		t.Fatal("the seed's two analytics channels did not both reach the listing")
	}
	if orders.Backlog == audit.Backlog {
		t.Errorf("both analytics channels report a backlog of %d; the seed gives them different ones",
			orders.Backlog)
	}
	// The paused one is the seed's audit channel, and its status has to say
	// so rather than reporting a channel with a consumer as healthy.
	if audit.Status != model.SubscriptionWarning {
		t.Errorf("%s/analytics is paused on the daemon and the app calls it %q",
			nsqSeedAudit, audit.Status)
	}
}

/*
 * The cluster board, against both tiers.
 *
 * The overview counts distinct objects across daemons that each know only
 * their own, so the arithmetic is the driver's and a topic on two daemons is
 * the case that catches a double count.
 */
func TestCrossCheckNSQCluster(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq crosscheck cluster", true))

	raw := []probeStats{
		nsqProbe{crossNSQD1}.stats(t),
		nsqProbe{crossNSQD2}.stats(t),
	}
	requireNSQSeed(t, raw[0])

	/*
	 * Read while the cluster is still.
	 *
	 * The overview counts everything, and the driver package's live tests are
	 * creating and deleting topics on the same cluster while this runs - so a
	 * single comparison would be racing them. This takes the daemons' own
	 * count either side of the app's call and only compares when the two agree
	 * that nothing moved in between. What it is checking is worth the wait: a
	 * topic carried by two daemons has to count once, and a driver adding up
	 * per-daemon totals would double it.
	 */
	overview, nodes := stillOverview(t, stack, connID)
	if len(nodes) != 2 || overview.TotalNodes != 2 || overview.OnlineNodes != 2 {
		t.Fatalf("the app lists %d daemons and calls %d of %d healthy; the profile names 2 and both answer",
			len(nodes), overview.OnlineNodes, overview.TotalNodes)
	}
	// nsqd reports no disk figure of any kind, so an average is a measurement
	// nobody took rather than a zero.
	if overview.AvgDiskUsage != model.UnknownMetric {
		t.Errorf("the app reports %d%% disk; nsqd reports no disk figure", overview.AvgDiskUsage)
	}
	// Matched by address rather than by position: the driver answers the
	// daemons concurrently, and a row landing in the wrong slot is exactly the
	// bug an index-aligned comparison would not see.
	byAddress := map[string]probeStats{
		"127.0.0.1:4151": raw[0],
		"127.0.0.1:4153": raw[1],
	}
	for _, node := range nodes {
		stats, known := byAddress[node.Address]
		if !known {
			t.Errorf("the app lists a daemon at %s that the profile does not name", node.Address)
			continue
		}
		if node.Attribute("health") != stats.Health {
			t.Errorf("%s: health = %q, the daemon says %q",
				node.Address, node.Attribute("health"), stats.Health)
		}
	}

	// The discovery tier, read from nsqlookupd rather than from any nsqd. Its
	// producer list is what a consumer is told, and the driver reports it as
	// given rather than as the addresses this connection uses.
	var registry struct {
		Producers []struct {
			BroadcastAddress string `json:"broadcast_address"`
			HTTPPort         int    `json:"http_port"`
		} `json:"producers"`
	}
	nsqProbe{crossLookupd1}.get(t, "/nodes", nil, &registry)

	directory, err := stack.cluster.DirectoryNodes(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("directory nodes: %v", err)
	}
	if len(directory) != 2 {
		t.Fatalf("the app lists %d nsqlookupd, the profile names 2", len(directory))
	}
	advertised := make([]string, 0, len(registry.Producers))
	for _, producer := range registry.Producers {
		advertised = append(advertised, fmt.Sprintf("%s:%d", producer.BroadcastAddress, producer.HTTPPort))
	}
	sort.Strings(advertised)
	if got := directory[0].Attribute("nodes"); got != strings.Join(advertised, ",") {
		t.Errorf("the app says %s advertises %q; nsqlookupd's own answer is %v",
			directory[0].Address, got, advertised)
	}
	if directory[0].Attribute("producerCount") != fmt.Sprint(len(registry.Producers)) {
		t.Errorf("the app counts %s registered daemons, nsqlookupd reports %d",
			directory[0].Attribute("producerCount"), len(registry.Producers))
	}
}

/*
 * The clients board, against the same stats the driver walks.
 *
 * The compose file keeps one consumer attached to a topic nothing publishes
 * to, so there is always a row - and its ready count is the figure the page
 * exists for.
 */
func TestCrossCheckNSQClients(t *testing.T) {
	requireLiveNSQ(t)
	stack := newNSQStack(t)
	connID := stack.dial(t, liveNSQProfile("nsq crosscheck clients", true))

	type wantedClient struct {
		role  string
		ready int
		topic string
		chans string
	}
	// The seed deletes the seeded topics before remaking them, which
	// disconnects the compose consumer. It resubscribes on its own, but not
	// within the moment a suite started straight after the seed would look, so
	// this reads until it is back rather than once.
	wanted := map[string]wantedClient{}
	deadline := time.Now().Add(30 * time.Second)
	for {
		wanted = map[string]wantedClient{}
		attached := false
		for _, address := range []string{crossNSQD1, crossNSQD2} {
			stats := nsqProbe{address}.stats(t)
			for _, topic := range stats.Topics {
				for _, channel := range topic.Channels {
					for _, client := range channel.Clients {
						wanted[client.RemoteAddress] = wantedClient{
							role:  "consumer",
							ready: client.ReadyCount,
							topic: topic.Name,
							chans: channel.Name,
						}
						attached = true
					}
				}
			}
			for _, producer := range stats.Producers {
				wanted[producer.RemoteAddress] = wantedClient{role: "producer"}
			}
		}
		if attached || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(wanted) == 0 {
		e2e.Missing(t, "no consumer is attached; the compose file keeps one on %s, "+
			"which the seed has to create first", nsqSeedEvents)
	}

	clients, err := stack.nsq.Connections(nsqContext(t), connID)
	if err != nil {
		t.Fatalf("connections: %v", err)
	}
	if len(clients) != len(wanted) {
		t.Errorf("the app lists %d clients, the daemons report %d", len(clients), len(wanted))
	}
	for _, client := range clients {
		peer := fmt.Sprintf("%s:%d", client.PeerHost, client.PeerPort)
		expected, known := wanted[peer]
		if !known {
			t.Errorf("the app lists %s, which no daemon reports", peer)
			continue
		}
		if client.Attribute("role") != expected.role {
			t.Errorf("%s: the app calls it a %s, the daemon reports it as a %s",
				peer, client.Attribute("role"), expected.role)
		}
		// A producer has none of what follows, and must not: a ready count on
		// one would read as a stalled consumer.
		if expected.role != "consumer" {
			continue
		}
		if client.Attribute("readyCount") != fmt.Sprint(expected.ready) {
			t.Errorf("%s: ready = %q, the daemon says %d",
				peer, client.Attribute("readyCount"), expected.ready)
		}
		if client.Attribute("topic") != expected.topic || client.Attribute("channel") != expected.chans {
			t.Errorf("%s: the app says %s/%s, the daemon says %s/%s", peer,
				client.Attribute("topic"), client.Attribute("channel"),
				expected.topic, expected.chans)
		}
	}
}

// distinctObjects counts topic names and topic/channel pairs across the
// cluster, which is what the overview reports and what a driver adding up
// per-daemon counts would get wrong.
func distinctObjects(all []probeStats) (topics, channels int) {
	topicNames := map[string]struct{}{}
	channelNames := map[string]struct{}{}
	for _, stats := range all {
		for _, topic := range stats.Topics {
			topicNames[topic.Name] = struct{}{}
			for _, channel := range topic.Channels {
				channelNames[topic.Name+"/"+channel.Name] = struct{}{}
			}
		}
	}
	return len(topicNames), len(channelNames)
}

// stillOverview reads the overview and compares it against the daemons' own
// counts, retrying until nothing changed across the call.
//
// The wait is the shared cluster rather than the driver: another package's
// live tests are creating and deleting topics on it, and a total taken while
// they do is not wrong so much as about a different cluster.
func stillOverview(t *testing.T, stack *nsqStack, connID int) (*model.ClusterOverview, []*model.Node) {
	t.Helper()
	for attempt := range 20 {
		before := []probeStats{nsqProbe{crossNSQD1}.stats(t), nsqProbe{crossNSQD2}.stats(t)}
		overview, nodes, err := stack.cluster.Overview(nsqContext(t), connID)
		if err != nil {
			t.Fatalf("overview: %v", err)
		}
		after := []probeStats{nsqProbe{crossNSQD1}.stats(t), nsqProbe{crossNSQD2}.stats(t)}

		beforeTopics, beforeChannels := distinctObjects(before)
		afterTopics, afterChannels := distinctObjects(after)
		if beforeTopics != afterTopics || beforeChannels != afterChannels {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if overview.Destinations != beforeTopics {
			t.Errorf("the app counts %d topics, the daemons hold %d distinct names (attempt %d)",
				overview.Destinations, beforeTopics, attempt)
		}
		if overview.Subscriptions != beforeChannels {
			t.Errorf("the app counts %d channels, the daemons hold %d distinct ones (attempt %d)",
				overview.Subscriptions, beforeChannels, attempt)
		}
		return overview, nodes
	}
	t.Fatal("the cluster never stopped changing long enough to compare a total against it")
	return nil, nil
}
