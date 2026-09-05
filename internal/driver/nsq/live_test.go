package nsq

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live cluster, as tests/e2e/nsq/compose.yaml publishes it.
//
// Two nsqd and two nsqlookupd, and both halves matter. A topic lives on the
// daemon it was created on, so only a second nsqd can prove a cluster figure
// is a sum rather than one node's answer.
const (
	liveNSQD1    = "http://127.0.0.1:4151"
	liveNSQD2    = "http://127.0.0.1:4153"
	liveLookupd1 = "http://127.0.0.1:4161"
	liveLookupd2 = "http://127.0.0.1:4163"
)

func requireCluster(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.NSQ,
		Name:   "the nsq cluster",
		Start:  "npm run e2e:nsq:up",
		Probe:  e2e.HTTPGet(liveNSQD1 + "/ping"),
	})
}

// liveProfile is the cluster as a user would configure it: both nsqd in the
// address field, both nsqlookupd in the discovery one.
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "nsq e2e",
		Kind:       model.KindNSQ,
		Endpoints:  liveNSQD1 + "," + liveNSQD2,
		TimeoutSec: 10,
		Options:    map[string]string{OptionLookupd: liveLookupd1 + "," + liveLookupd2},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireCluster(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Opening has to reach every address in the profile, not the first one that
// answers: a cluster is the set, and a connection that opened against half of
// it would report depths that are quietly short.
func TestLiveOpenReachesEveryDaemon(t *testing.T) {
	conn := liveConn(t)

	if len(conn.nodes) != 2 {
		t.Fatalf("opened against %d nsqd, want 2", len(conn.nodes))
	}
	for _, node := range conn.nodes {
		if node.info.Version == "" {
			t.Errorf("%s reported no version", node.address)
		}
		if node.info.TCPPort == 0 {
			t.Errorf("%s reported no tcp port, so it did not answer as an nsqd", node.address)
		}
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

/*
 * The two daemons answer /info at adjacent ports and neither refuses the
 * other's field, so putting one address in the wrong row is the likeliest way
 * to fill this form in wrong. It has to fail at open with a sentence naming
 * the swap - the alternative is a connection that opens and then reports a
 * cluster with no topics, which reads as an empty broker rather than as a
 * misconfiguration.
 */
func TestLiveOpenRefusesTheDaemonsSwappedOver(t *testing.T) {
	requireCluster(t)

	t.Run("an nsqlookupd in the nsqd field", func(t *testing.T) {
		profile := liveProfile()
		profile.Endpoints = liveLookupd1
		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened against an nsqlookupd as though it were an nsqd")
		}
		if !strings.Contains(err.Error(), "lookupd field") {
			t.Errorf("error does not say where the address belongs: %v", err)
		}
	})

	t.Run("an nsqd in the lookupd field", func(t *testing.T) {
		profile := liveProfile()
		profile.Options[OptionLookupd] = liveNSQD1
		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened against an nsqd as though it were an nsqlookupd")
		}
		if !strings.Contains(err.Error(), "nsqd field") {
			t.Errorf("error does not say where the address belongs: %v", err)
		}
	})
}

// A daemon that is not there has to fail the whole open rather than leave a
// connection speaking for a smaller cluster than the profile named.
func TestLiveOpenFailsWhenOneDaemonIsAbsent(t *testing.T) {
	requireCluster(t)

	profile := liveProfile()
	profile.Endpoints = liveNSQD1 + ",http://127.0.0.1:4159"
	profile.TimeoutSec = 2
	if _, err := open(liveContext(t), profile); err == nil {
		t.Fatal("opened against a cluster with an unreachable nsqd in it")
	}
}

// Close is called on disconnect and again on shutdown, so the second call has
// to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireCluster(t)

	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answered a ping")
	}
}

// rawPost drives the cluster without going through the driver, so a test can
// arrange a state the driver is not being asked to produce.
func rawPost(t *testing.T, address, path string, query url.Values, body io.Reader) {
	t.Helper()
	endpoint := address + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("building %s: %v", endpoint, err)
	}
	request.Header.Set("Accept", "application/vnd.nsq; version=1.0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s answered %d: %s", endpoint, response.StatusCode, payload)
	}
}

// rawTopicNames reads one daemon's topic list straight out of its own API.
func rawTopicNames(t *testing.T, address string) []string {
	t.Helper()
	response, err := http.Get(address + "/stats?format=json")
	if err != nil {
		t.Fatalf("stats on %s: %v", address, err)
	}
	defer func() { _ = response.Body.Close() }()

	var stats struct {
		Topics []struct {
			Name string `json:"topic_name"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatalf("decoding stats on %s: %v", address, err)
	}
	names := make([]string, 0, len(stats.Topics))
	for _, topic := range stats.Topics {
		names = append(names, topic.Name)
	}
	return names
}

// rawLookupdTopics is nsqlookupd's own registry, which is a different list
// from any nsqd's and the one a delete is most likely to leave behind.
func rawLookupdTopics(t *testing.T, address string) []string {
	t.Helper()
	response, err := http.Get(address + "/topics")
	if err != nil {
		t.Fatalf("topics on %s: %v", address, err)
	}
	defer func() { _ = response.Body.Close() }()

	var payload struct {
		Topics []string `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decoding topics on %s: %v", address, err)
	}
	return payload.Topics
}

func contains(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

// testTopic creates a topic on both daemons through the driver and removes it
// when the test ends, so nothing a run leaves behind changes the next one.
func testTopic(t *testing.T, conn *Conn, name string) {
	t.Helper()
	if err := conn.CreateDestination(liveContext(t), model.DestinationSpec{
		Ref: model.DestinationRef{Name: name},
	}); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: name})
	})
}

func find(destinations []*model.Destination, name string) *model.Destination {
	for _, entry := range destinations {
		if entry.Ref.Name == name {
			return entry
		}
	}
	return nil
}

/*
 * The fold is the whole of what this driver has to get right about NSQ's
 * topology, and only a second nsqd can catch it being wrong. A topic carried
 * by both daemons is one row, its node list names both, and every figure on
 * it is the sum - a driver that read the first daemon and stopped would pass
 * every other test here.
 */
func TestLiveListDestinationsFoldsTheClusterIntoOneRow(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.fold"

	testTopic(t, conn, topic)
	// Channels first: a channel only receives what arrives after it exists.
	if err := conn.client.post(liveContext(t), liveNSQD1, "/channel/create",
		url.Values{"topic": {topic}, "channel": {"one"}}, nil, nil); err != nil {
		t.Fatalf("creating a channel: %v", err)
	}
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb\nc"))
	rawPost(t, liveNSQD2, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("d\ne"))

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	entry := find(destinations, topic)
	if entry == nil {
		t.Fatalf("the topic is on both daemons and not in the listing")
	}
	if seen := strings.Count(entry.Attribute(AttrNodes), ","); seen != 1 {
		t.Errorf("nodes = %q, want both daemons", entry.Attribute(AttrNodes))
	}
	// Five published, and the channel exists on both daemons because nsqd
	// reads the channel list off nsqlookupd when it creates a topic - so all
	// five are held once by a channel.
	if entry.Depth != 5 {
		t.Errorf("depth = %d, want the 5 published across both daemons", entry.Depth)
	}
	if entry.Attribute(AttrMessageCount) != "5" {
		t.Errorf("messageCount = %q, want 5", entry.Attribute(AttrMessageCount))
	}
	if entry.Partitions != model.UnknownMetric {
		t.Errorf("partitions = %d, and NSQ does not split a topic", entry.Partitions)
	}
	if entry.RateIn != model.UnknownMetric || entry.RateOut != model.UnknownMetric {
		t.Errorf("rates = %d/%d, and nsqd reports none", entry.RateIn, entry.RateOut)
	}
}

// The detail read is a different call - filtered at the daemon rather than
// folded out of the whole list - so it can disagree with the listing without
// anything else noticing.
func TestLiveDestinationDetailAgreesWithTheListing(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.detail"

	testTopic(t, conn, topic)
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb"))

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	listed := find(destinations, topic)
	if listed == nil {
		t.Fatal("the topic is missing from the listing")
	}

	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Depth != listed.Depth {
		t.Errorf("detail depth = %d, listing = %d", detail.Depth, listed.Depth)
	}
	if detail.Attribute(AttrNodes) != listed.Attribute(AttrNodes) {
		t.Errorf("detail nodes = %q, listing = %q",
			detail.Attribute(AttrNodes), listed.Attribute(AttrNodes))
	}

	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{
		Name: "MQS.TEST.nothing.is.here",
	}); err == nil {
		t.Error("a topic no daemon carries was described rather than reported missing")
	}
}

// A paused topic is the only state in NSQ where a topic depth is not zero:
// nothing is copied into its channels, so the messages sit in the topic
// itself. It is also the state a board would show as a healthy empty topic if
// the driver only added up channel depths.
func TestLiveListDestinationsSplitsTopicDepthFromChannelDepth(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.held"

	testTopic(t, conn, topic)
	rawPost(t, liveNSQD1, "/channel/create", url.Values{"topic": {topic}, "channel": {"one"}}, nil)
	rawPost(t, liveNSQD1, "/topic/pause", url.Values{"topic": {topic}}, nil)
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb\nc"))

	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Attribute(AttrPaused) != "true" {
		t.Errorf("paused = %q, want true", detail.Attribute(AttrPaused))
	}
	if detail.Attribute(AttrTopicDepth) != "3" {
		t.Errorf("topicDepth = %q, want the 3 the pause is holding",
			detail.Attribute(AttrTopicDepth))
	}
	if detail.Attribute(AttrChannelDepth) != "0" {
		t.Errorf("channelDepth = %q, want 0 while the topic is paused",
			detail.Attribute(AttrChannelDepth))
	}
	if detail.Depth != 3 {
		t.Errorf("depth = %d, want the 3 the topic is holding", detail.Depth)
	}
}

/*
 * A create has to reach every daemon and a delete has to reach the discovery
 * tier as well.
 *
 * The second half is the one that was nearly wrong. nsqd forgets a deleted
 * topic and nsqlookupd does not, so a delete that stopped at nsqd leaves the
 * name in /topics - where nsqadmin still lists it and a consumer looking it up
 * still finds it, with an empty producer list rather than a 404.
 */
func TestLiveCreateAndRemoveReachEveryDaemonAndTheDirectory(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.lifecycle"

	if err := conn.CreateDestination(liveContext(t), model.DestinationSpec{
		Ref: model.DestinationRef{Name: topic},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: topic})
	}()

	for _, address := range []string{liveNSQD1, liveNSQD2} {
		if !contains(rawTopicNames(t, address), topic) {
			t.Errorf("%s is not carrying the topic the create was meant to reach", address)
		}
	}

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, address := range []string{liveNSQD1, liveNSQD2} {
		if contains(rawTopicNames(t, address), topic) {
			t.Errorf("%s is still carrying the deleted topic", address)
		}
	}
	for _, address := range []string{liveLookupd1, liveLookupd2} {
		if contains(rawLookupdTopics(t, address), topic) {
			t.Errorf("%s still lists the deleted topic, so the delete did not reach the directory", address)
		}
	}

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic}); err == nil {
		t.Error("deleting a topic no daemon carries reported success")
	}
}

// The method exists because DestinationAdmin is one interface. Nothing in the
// UI reaches it, and it must not quietly do something instead.
func TestLiveUpdateDestinationIsRefused(t *testing.T) {
	conn := liveConn(t)
	if err := conn.UpdateDestination(liveContext(t), model.DestinationSpec{
		Ref: model.DestinationRef{Name: "MQS.TEST.whatever"},
	}); err == nil {
		t.Error("an nsq topic has nothing to reconfigure and the update reported success")
	}
}

/*
 * The purge that nearly did nothing.
 *
 * /topic/empty touches only the topic's own queue, and on a topic with any
 * channel at all that queue is already empty - nsqd copied every message into
 * the channels as it arrived. A purge built on that one call answers 200 and
 * leaves the depth on screen exactly where it was, which is the shape of a
 * control that does nothing when clicked.
 */
func TestLivePurgeEmptiesTheChannelsAsWellAsTheTopic(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.purge"

	testTopic(t, conn, topic)
	rawPost(t, liveNSQD1, "/channel/create", url.Values{"topic": {topic}, "channel": {"one"}}, nil)
	rawPost(t, liveNSQD1, "/channel/create", url.Values{"topic": {topic}, "channel": {"two"}}, nil)
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb\nc"))
	rawPost(t, liveNSQD2, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("d\ne"))

	before, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("detail before: %v", err)
	}
	// Eight from five published, and the arithmetic is worth stating because
	// it is the whole model: the first daemon has two channels, so its three
	// messages are held twice; the second was given the topic before either
	// channel existed and has none, so its two sit in the topic itself.
	if before.Depth != 8 {
		t.Fatalf("depth before = %d, want 3 messages held by two channels plus 2 held by a topic",
			before.Depth)
	}
	if before.Attribute(AttrChannelDepth) != "6" || before.Attribute(AttrTopicDepth) != "2" {
		t.Fatalf("split = topic %q, channels %q; want 2 and 6",
			before.Attribute(AttrTopicDepth), before.Attribute(AttrChannelDepth))
	}

	if err := conn.PurgeQueue(liveContext(t), model.DestinationRef{Name: topic}); err != nil {
		t.Fatalf("purge: %v", err)
	}

	after, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("detail after: %v", err)
	}
	if after.Depth != 0 {
		t.Errorf("depth after = %d, so the purge left the channels holding messages", after.Depth)
	}

	if err := conn.PurgeQueue(liveContext(t), model.DestinationRef{
		Name: "MQS.TEST.nothing.is.here",
	}); err == nil {
		t.Error("purging a topic no daemon carries reported success")
	}
}

// Pausing is not a purge and not a delete: publishing carries on and the
// messages pile up in the topic itself rather than in its channels. A board
// that only added up channel depths would show a paused topic as idle.
func TestLivePauseHoldsMessagesInTheTopic(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.pause"

	testTopic(t, conn, topic)
	rawPost(t, liveNSQD1, "/channel/create", url.Values{"topic": {topic}, "channel": {"one"}}, nil)

	if err := conn.SetTopicPaused(liveContext(t), topic, true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb"))

	paused, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
	if err != nil {
		t.Fatalf("detail while paused: %v", err)
	}
	if paused.Attribute(AttrPaused) != "true" {
		t.Errorf("paused = %q, want true", paused.Attribute(AttrPaused))
	}
	if paused.Attribute(AttrTopicDepth) != "2" {
		t.Errorf("topicDepth = %q, want the 2 the pause is holding", paused.Attribute(AttrTopicDepth))
	}
	if paused.Attribute(AttrChannelDepth) != "0" {
		t.Errorf("channelDepth = %q, want 0 while nothing is being copied out",
			paused.Attribute(AttrChannelDepth))
	}

	if err := conn.SetTopicPaused(liveContext(t), topic, false); err != nil {
		t.Fatalf("unpause: %v", err)
	}
	// Resuming is what moves them, and nsqd does it on its own message pump
	// rather than inside the call, so this is a wait rather than a read.
	deadline := time.Now().Add(5 * time.Second)
	for {
		resumed, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: topic})
		if err != nil {
			t.Fatalf("detail after resuming: %v", err)
		}
		if resumed.Attribute(AttrPaused) == "false" && resumed.Attribute(AttrChannelDepth) == "2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after resuming, topicDepth = %q and channelDepth = %q",
				resumed.Attribute(AttrTopicDepth), resumed.Attribute(AttrChannelDepth))
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := conn.SetTopicPaused(liveContext(t), "MQS.TEST.nothing.is.here", true); err == nil {
		t.Error("pausing a topic no daemon carries reported success")
	}
}

// The three QueueActions methods NSQ has no way to perform. They exist because
// the interface is one; nothing in the UI reaches them, and they must not
// quietly do something instead.
func TestLiveTheQueueActionsNSQLacksAreRefused(t *testing.T) {
	conn := liveConn(t)

	if _, err := conn.MoveMessages(liveContext(t), model.MoveRequest{
		From: "MQS.TEST.a", ToRoutingKey: "MQS.TEST.b",
	}); err == nil {
		t.Error("a move reported success, and nothing drains one nsq topic into another")
	}
	if _, err := conn.DropMessages(liveContext(t), model.DestinationRef{Name: "MQS.TEST.a"}, 5); err == nil {
		t.Error("a bounded drop reported success, and nsqd empties a queue whole")
	}
	if err := conn.RebalanceQueues(liveContext(t)); err == nil {
		t.Error("a rebalance reported success, and a topic lives on the daemon that made it")
	}
}
