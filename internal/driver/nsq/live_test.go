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

func findChannel(subscriptions []*model.Subscription, topic, name string) *model.Subscription {
	for _, entry := range subscriptions {
		if entry.Ref.Namespace == topic && entry.Ref.Name == name {
			return entry
		}
	}
	return nil
}

/*
 * A channel is identified by its topic as well as its own name, and the
 * listing has to keep them apart. Two topics with a channel called the same
 * thing have separate backlogs and separate consumers, and folding them into
 * one row would report a backlog that belongs to neither.
 */
func TestLiveListSubscriptionsKeepsTheTopicInTheIdentity(t *testing.T) {
	conn := liveConn(t)
	const first = "MQS.TEST.chan.one"
	const second = "MQS.TEST.chan.two"

	testTopic(t, conn, first)
	testTopic(t, conn, second)
	for _, topic := range []string{first, second} {
		if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
			Ref: model.SubscriptionRef{Namespace: topic, Name: "shared"},
		}); err != nil {
			t.Fatalf("creating a channel on %s: %v", topic, err)
		}
	}
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {first}}, strings.NewReader("a\nb\nc"))

	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	one := findChannel(subscriptions, first, "shared")
	two := findChannel(subscriptions, second, "shared")
	if one == nil || two == nil {
		t.Fatal("the same channel name under two topics did not produce two rows")
	}
	if one.Backlog != 3 {
		t.Errorf("backlog on %s = %d, want the 3 published", first, one.Backlog)
	}
	if two.Backlog != 0 {
		t.Errorf("backlog on %s = %d, want none - nothing was published to it", second, two.Backlog)
	}
	if one.Destinations != 1 {
		t.Errorf("destinations = %d; a channel belongs to exactly one topic", one.Destinations)
	}
	if one.RateOut != model.UnknownMetric {
		t.Errorf("rateOut = %d, and nsqd reports no rate", one.RateOut)
	}

	detail, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{
		Namespace: first, Name: "shared",
	})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Backlog != one.Backlog {
		t.Errorf("detail backlog = %d, listing = %d", detail.Backlog, one.Backlog)
	}
}

/*
 * What a channel created after the fact inherits, which is not what the
 * obvious reading of NSQ says.
 *
 * A channel receives what arrives after it exists - so a second channel added
 * to a busy topic starts at nothing, and there is no position to rewind it to.
 * But a topic with no channel at all holds its messages in its own queue
 * rather than discarding them, and the first channel created drains that queue
 * into itself. Both halves are pinned here because a page that promised either
 * one alone would be wrong half the time.
 */
func TestLiveWhatALateChannelInherits(t *testing.T) {
	conn := liveConn(t)

	t.Run("the first channel drains what the topic was holding", func(t *testing.T) {
		const topic = "MQS.TEST.firstchannel"
		testTopic(t, conn, topic)
		rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb\nc"))

		if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
			Ref: model.SubscriptionRef{Namespace: topic, Name: "first"},
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if backlog := awaitBacklog(t, conn, topic, "first", 3); backlog != 3 {
			t.Errorf("backlog = %d; the topic was holding 3 with nowhere to put them", backlog)
		}
	})

	t.Run("a second channel starts at nothing", func(t *testing.T) {
		const topic = "MQS.TEST.secondchannel"
		testTopic(t, conn, topic)
		if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
			Ref: model.SubscriptionRef{Namespace: topic, Name: "early"},
		}); err != nil {
			t.Fatalf("creating the first channel: %v", err)
		}
		rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb\nc"))

		if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
			Ref: model.SubscriptionRef{Namespace: topic, Name: "late"},
		}); err != nil {
			t.Fatalf("creating the second channel: %v", err)
		}
		detail, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{
			Namespace: topic, Name: "late",
		})
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if detail.Backlog != 0 {
			t.Errorf("backlog = %d; the copies were already made into the first channel",
				detail.Backlog)
		}
	})
}

// awaitBacklog waits for a channel to reach a backlog, because nsqd moves
// messages out of a topic on its own message pump rather than inside the call
// that created the channel.
func awaitBacklog(t *testing.T, conn *Conn, topic, channel string, wanted int64) int64 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		detail, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{
			Namespace: topic, Name: channel,
		})
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if detail.Backlog == wanted || time.Now().After(deadline) {
			return detail.Backlog
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Pausing a channel stops its consumers and leaves the topic's other channels
// running, which is what makes it a different control from pausing the topic.
func TestLivePauseOneChannelLeavesTheOthersRunning(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.halfpaused"

	testTopic(t, conn, topic)
	for _, name := range []string{"held", "running"} {
		if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
			Ref: model.SubscriptionRef{Namespace: topic, Name: name},
		}); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}

	if err := conn.SetChannelPaused(liveContext(t), topic, "held", true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	rawPost(t, liveNSQD1, "/mpub", url.Values{"topic": {topic}}, strings.NewReader("a\nb"))

	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	held := findChannel(subscriptions, topic, "held")
	running := findChannel(subscriptions, topic, "running")
	if held == nil || running == nil {
		t.Fatal("one of the two channels is missing from the listing")
	}
	if held.Attribute(AttrPaused) != "true" || running.Attribute(AttrPaused) != "false" {
		t.Errorf("paused = %q and %q, want only the first",
			held.Attribute(AttrPaused), running.Attribute(AttrPaused))
	}
	// A paused channel with no consumer still shows warning rather than
	// offline: the pause is the reason nothing is moving, and it outranks
	// having nobody attached.
	if held.Status != model.SubscriptionWarning {
		t.Errorf("status = %q, want warning while the channel is paused", held.Status)
	}
	if running.Status != model.SubscriptionOffline {
		t.Errorf("status = %q, want offline with no consumer attached", running.Status)
	}
	// Both still receive their copy: pausing stops delivery to consumers, not
	// the copy into the channel.
	if held.Backlog != 2 || running.Backlog != 2 {
		t.Errorf("backlogs = %d and %d, want 2 each", held.Backlog, running.Backlog)
	}

	if err := conn.EmptyChannel(liveContext(t), topic, "held"); err != nil {
		t.Fatalf("empty: %v", err)
	}
	subscriptions, err = conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list after emptying: %v", err)
	}
	if emptied := findChannel(subscriptions, topic, "held"); emptied.Backlog != 0 {
		t.Errorf("backlog after emptying = %d, want 0", emptied.Backlog)
	}
	if kept := findChannel(subscriptions, topic, "running"); kept.Backlog != 2 {
		t.Errorf("emptying one channel took %d from the other", 2-kept.Backlog)
	}
}

// A channel delete has to reach the discovery tier for the same reason a topic
// delete does: a channel nsqlookupd still lists is one that every nsqd later
// carrying the topic recreates for itself.
func TestLiveRemoveSubscriptionReachesTheDirectory(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.chandelete"

	testTopic(t, conn, topic)
	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: "doomed"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{
		Namespace: topic, Name: "doomed",
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	for _, address := range []string{liveLookupd1, liveLookupd2} {
		if contains(rawLookupdChannels(t, address, topic), "doomed") {
			t.Errorf("%s still lists the deleted channel", address)
		}
	}
	// A daemon asked for a topic it has forgotten the channel of recreates it
	// from the directory, so a delete that left the registration behind comes
	// back the moment anything touches the topic.
	if err := conn.CreateDestination(liveContext(t), model.DestinationSpec{
		Ref: model.DestinationRef{Name: topic},
	}); err != nil {
		t.Fatalf("recreating the topic: %v", err)
	}
	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if findChannel(subscriptions, topic, "doomed") != nil {
		t.Error("the deleted channel came back, so its registration outlived it")
	}

	if err := conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{
		Namespace: topic, Name: "doomed",
	}); err == nil {
		t.Error("deleting a channel no daemon carries reported success")
	}
}

// rawLookupdChannels is the discovery tier's own channel registry for a topic,
// which is a separate list from any nsqd's.
func rawLookupdChannels(t *testing.T, address, topic string) []string {
	t.Helper()
	response, err := http.Get(address + "/channels?topic=" + url.QueryEscape(topic))
	if err != nil {
		t.Fatalf("channels on %s: %v", address, err)
	}
	defer func() { _ = response.Body.Close() }()

	var payload struct {
		Channels []string `json:"channels"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decoding channels on %s: %v", address, err)
	}
	return payload.Channels
}

// The method exists because SubscriptionAdmin is one interface. Nothing in the
// UI reaches it, and it must not quietly do something instead.
func TestLiveUpdateSubscriptionIsRefused(t *testing.T) {
	conn := liveConn(t)
	if err := conn.UpdateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: "MQS.TEST.a", Name: "b"},
	}); err == nil {
		t.Error("an nsq channel has nothing to reconfigure and the update reported success")
	}
}

// rawChannelDepth reads one channel's depth off one daemon, without going
// through the driver.
func rawChannelDepth(t *testing.T, address, topic, channel string) int64 {
	t.Helper()
	response, err := http.Get(address + "/stats?format=json&topic=" + url.QueryEscape(topic))
	if err != nil {
		t.Fatalf("stats on %s: %v", address, err)
	}
	defer func() { _ = response.Body.Close() }()

	var stats struct {
		Topics []struct {
			Channels []struct {
				Name     string `json:"channel_name"`
				Depth    int64  `json:"depth"`
				Deferred int    `json:"deferred_count"`
			} `json:"channels"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatalf("decoding stats on %s: %v", address, err)
	}
	for _, entry := range stats.Topics {
		for _, found := range entry.Channels {
			if found.Name == channel {
				return found.Depth
			}
		}
	}
	return -1
}

/*
 * A publish goes to one daemon and stays there, which is the fact the send
 * console's node field exists for. A driver that fanned a send out across the
 * cluster would multiply every message by the number of nsqd; one that always
 * used the first would make the field a lie.
 */
func TestLivePublishGoesToTheDaemonItNames(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.publish"

	testTopic(t, conn, topic)
	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: "one"},
	}); err != nil {
		t.Fatalf("creating a channel: %v", err)
	}

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic, Body: "second daemon", Count: 3, Node: hostPort(liveNSQD2),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Sent != 3 || result.Node != hostPort(liveNSQD2) {
		t.Fatalf("sent %d to %q, want 3 to %q", result.Sent, result.Node, hostPort(liveNSQD2))
	}

	if depth := rawChannelDepth(t, liveNSQD2, topic, "one"); depth != 3 {
		t.Errorf("the named daemon holds %d, want 3", depth)
	}
	if depth := rawChannelDepth(t, liveNSQD1, topic, "one"); depth != 0 {
		t.Errorf("the other daemon holds %d; a publish reaches one nsqd", depth)
	}

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic, Body: "x", Node: "127.0.0.1:9999",
	}); err == nil {
		t.Error("a publish through a daemon outside the connection reported success")
	}
}

/*
 * A delayed batch is the one shape that has to go one message at a time.
 *
 * /mpub takes a defer parameter and ignores it - confirmed against 1.3.0,
 * where an mpub with defer=1000 answers OK and the messages are in the channel
 * immediately. A driver that used /mpub for a repeat with a delay would report
 * a scheduled send and deliver it now.
 */
func TestLivePublishHonoursADelayOnEveryCopy(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.deferred"

	testTopic(t, conn, topic)
	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: "one"},
	}); err != nil {
		t.Fatalf("creating a channel: %v", err)
	}

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic, Body: "later", Count: 4, Delay: 30 * time.Minute, Node: hostPort(liveNSQD1),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	detail, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{
		Namespace: topic, Name: "one",
	})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Attribute(AttrDeferred) != "4" {
		t.Errorf("deferred = %q, want all 4 held back", detail.Attribute(AttrDeferred))
	}
	// Deferred messages are counted apart from the backlog, so a channel with
	// four waiting on a delivery time reports a depth of nothing.
	if detail.Backlog != 0 {
		t.Errorf("backlog = %d; a deferred message is not yet deliverable", detail.Backlog)
	}

	// nsqd's own ceiling, and the driver passes the value through rather than
	// guessing at a deployment's --max-req-timeout.
	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic, Body: "far too late", Delay: 2 * time.Hour,
	}); err == nil {
		t.Error("a delay past nsqd's ceiling was accepted")
	}
}

// A body with a newline in it cannot go through /mpub, which uses newline as
// its separator: a repeat of one would arrive as several messages each time.
func TestLivePublishKeepsAMultilineBodyWhole(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.multiline"

	testTopic(t, conn, topic)
	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: "one"},
	}); err != nil {
		t.Fatalf("creating a channel: %v", err)
	}

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic, Body: "{\n  \"id\": 1\n}", Count: 3, Node: hostPort(liveNSQD1),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if depth := rawChannelDepth(t, liveNSQD1, topic, "one"); depth != 3 {
		t.Errorf("the channel holds %d, want the 3 sent rather than their lines", depth)
	}
}

/*
 * The canonical port, and the two arguments it carries that NSQ cannot.
 *
 * Refusing rather than ignoring is the whole point: a tag put in the field and
 * silently dropped would be reported as sent, and the consumer would never see
 * a value the sender believes it carried.
 */
func TestLiveSendMessageRefusesWhatNSQCannotCarry(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.canonical"

	testTopic(t, conn, topic)

	if _, err := conn.SendMessage(liveContext(t), topic, "orders", "", "body", 0); err == nil {
		t.Error("a tag was accepted, and an nsq message has nowhere to put one")
	}
	if _, err := conn.SendMessage(liveContext(t), topic, "", "key-1", "body", 0); err == nil {
		t.Error("a key was accepted, and an nsq message has nowhere to put one")
	}

	id, err := conn.SendMessage(liveContext(t), topic, "", "", "body", 0)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	// Empty rather than a placeholder: nsqd answers a publish with the word OK
	// and hands back nothing that could look the message up again.
	if id != "" {
		t.Errorf("id = %q; nsqd returns no message id at all", id)
	}

	// delayLevel is seconds here, which is the reading the console labels.
	if _, err := conn.SendMessage(liveContext(t), topic, "", "", "body", 60); err != nil {
		t.Errorf("send with a delay: %v", err)
	}
	if _, err := conn.SendMessage(liveContext(t), topic, "", "", "", 0); err == nil {
		t.Error("an empty body was accepted, and nsqd answers MSG_EMPTY")
	}
}

/*
 * The node list is the profile's, not the cluster's, and that is the decision
 * worth pinning.
 *
 * There is no nsqd that knows about the others. nsqlookupd knows which daemons
 * registered with it, and those need not be the ones this connection speaks
 * for - so a driver that built the list from discovery would report figures
 * that are not the sum the rest of the app shows.
 */
func TestLiveListNodesIsWhatTheProfileNames(t *testing.T) {
	conn := liveConn(t)

	nodes, err := conn.ListNodes(liveContext(t))
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("listed %d nodes, want the 2 the profile names", len(nodes))
	}

	addresses := make([]string, 0, len(nodes))
	for _, node := range nodes {
		addresses = append(addresses, node.Address)
		if node.Version == "" {
			t.Errorf("%s reported no version", node.Address)
		}
		if node.Status != model.NodeOnline {
			t.Errorf("%s is %q with health %q, want online",
				node.Address, node.Status, node.Attribute(AttrHealth))
		}
		if node.DiskUsage != model.UnknownMetric {
			t.Errorf("%s reports a disk figure, and nsqd has none", node.Address)
		}
		if node.RateIn != model.UnknownMetric || node.RateOut != model.UnknownMetric {
			t.Errorf("%s reports a rate, and nsqd has none", node.Address)
		}
	}
	if !contains(addresses, hostPort(liveNSQD1)) || !contains(addresses, hostPort(liveNSQD2)) {
		t.Errorf("nodes = %v, want both addresses in the profile", addresses)
	}

	detail, err := conn.NodeDetail(liveContext(t), hostPort(liveNSQD2))
	if err != nil {
		t.Fatalf("node detail: %v", err)
	}
	if detail.Address != hostPort(liveNSQD2) {
		t.Errorf("detail = %q, want %q", detail.Address, hostPort(liveNSQD2))
	}
	if _, err := conn.NodeDetail(liveContext(t), "127.0.0.1:9999"); err == nil {
		t.Error("a node outside the connection was described")
	}
}

// The overview counts distinct objects across the cluster, not per daemon:
// a topic on both nsqd is one topic, and a driver that added the daemons'
// counts would double it.
func TestLiveClusterOverviewCountsDistinctObjects(t *testing.T) {
	conn := liveConn(t)
	const topic = "MQS.TEST.overview"

	before, err := conn.ClusterOverview(liveContext(t))
	if err != nil {
		t.Fatalf("overview: %v", err)
	}

	testTopic(t, conn, topic)
	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: topic, Name: "one"},
	}); err != nil {
		t.Fatalf("creating a channel: %v", err)
	}

	after, err := conn.ClusterOverview(liveContext(t))
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if after.Destinations != before.Destinations+1 {
		t.Errorf("destinations went %d -> %d for one topic on two daemons",
			before.Destinations, after.Destinations)
	}
	if after.Subscriptions != before.Subscriptions+1 {
		t.Errorf("subscriptions went %d -> %d for one channel on two daemons",
			before.Subscriptions, after.Subscriptions)
	}
	if after.TotalNodes != 2 || after.OnlineNodes != 2 {
		t.Errorf("nodes = %d of %d, want 2 of 2", after.OnlineNodes, after.TotalNodes)
	}
	if after.AvgDiskUsage != model.UnknownMetric {
		t.Errorf("disk = %d, and nsqd reports no disk figure", after.AvgDiskUsage)
	}
}

/*
 * The directory tier reports what a consumer would be told, which is not the
 * same as what this connection can reach.
 *
 * nsqlookupd hands back whatever each nsqd broadcast about itself. The e2e
 * cluster is deliberately set up so the two agree - a broadcast address only
 * the compose network could resolve would make every consumer using discovery
 * fail - and this is what asserts they still do.
 */
func TestLiveDirectoryReportsWhatEachDaemonAdvertises(t *testing.T) {
	conn := liveConn(t)

	directory, err := conn.ListDirectoryNodes(liveContext(t))
	if err != nil {
		t.Fatalf("directory: %v", err)
	}
	if len(directory) != 2 {
		t.Fatalf("listed %d nsqlookupd, want the 2 the profile names", len(directory))
	}
	for _, node := range directory {
		if node.Version == "" {
			t.Errorf("%s reported no version", node.Address)
		}
		advertised := node.Attribute(AttrNodes)
		for _, wanted := range []string{hostPort(liveNSQD1), hostPort(liveNSQD2)} {
			if !strings.Contains(advertised, wanted) {
				t.Errorf("%s advertises %q, which does not include %q",
					node.Address, advertised, wanted)
			}
		}
	}

	// A profile with no discovery tier says so rather than drawing an empty
	// board, and the capability is degraded rather than absent.
	profile := liveProfile()
	profile.Options[OptionLookupd] = ""
	bare, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open without a directory: %v", err)
	}
	defer func() { _ = bare.Close() }()

	if bare.Capabilities().Has(model.CapDirectory) {
		t.Error("a connection naming no nsqlookupd claims a discovery tier")
	}
	if _, err := bare.ListDirectoryNodes(liveContext(t)); err == nil {
		t.Error("a connection naming no nsqlookupd listed a discovery tier anyway")
	}
}

// The settings a daemon reports about itself, and the one that matters most:
// a consumer that cannot find a topic which plainly exists is usually looking
// at an nsqd registered with a different nsqlookupd.
func TestLiveNodeConfigReportsTheLookupdItRegistersWith(t *testing.T) {
	conn := liveConn(t)

	config, err := conn.NodeConfig(liveContext(t), hostPort(liveNSQD1))
	if err != nil {
		t.Fatalf("node config: %v", err)
	}
	if config["version"] == "" {
		t.Error("the config reports no version")
	}
	if config["nsqlookupd_tcp_addresses"] == "" {
		t.Error("the config does not say which nsqlookupd this daemon registers with")
	}
	if config["tcp_port"] == "" || config["http_port"] == "" {
		t.Error("the config reports no ports")
	}

	directory, err := conn.DirectoryConfig(liveContext(t))
	if err != nil {
		t.Fatalf("directory config: %v", err)
	}
	if len(directory) != 2 {
		t.Errorf("directory config has %d entries, want one per nsqlookupd", len(directory))
	}
}

/*
 * The clients board's whole data source, and the one figure only it can show.
 *
 * A ready count of zero is a consumer that is connected, holding its channel,
 * and asking for nothing - a backlog that will not move while every other
 * figure looks healthy. The compose file keeps one consumer attached for
 * exactly this, on a topic nothing publishes to so it drains nothing.
 */
func TestLiveListClientConnectionsFindsTheAttachedConsumer(t *testing.T) {
	conn := liveConn(t)

	clients, err := conn.ListClientConnections(liveContext(t), "")
	if err != nil {
		t.Fatalf("clients: %v", err)
	}
	if len(clients) == 0 {
		e2e.Missing(t, "%s no consumer is attached; run `npm run e2e:nsq:seed` so the "+
			"compose consumer has a topic to subscribe to", e2e.SkipMarker)
	}

	for _, client := range clients {
		if client.Attribute(AttrClientTopic) == "" || client.Attribute(AttrClientChannel) == "" {
			t.Errorf("%s names no topic or channel, and a client exists only inside one",
				client.Name)
		}
		if client.Node == "" {
			t.Errorf("%s names no daemon; one consumer holds a connection per nsqd", client.Name)
		}
		if client.Channels != 1 {
			t.Errorf("%s reports %d channels; an nsq connection reads exactly one",
				client.Name, client.Channels)
		}
		if client.State != "subscribed" {
			t.Errorf("%s is %q; a client in a channel's list has subscribed",
				client.Name, client.State)
		}
		if client.ConnectedAtMs == 0 {
			t.Errorf("%s reports no connect time", client.Name)
		}
	}

	// There is no session layer under an NSQ connection, so the channel half
	// of the port is empty rather than absent - and it must stay empty rather
	// than growing rows that are the connections again.
	channels, err := conn.ListClientChannels(liveContext(t), "")
	if err != nil {
		t.Fatalf("client channels: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("listed %d client channels; an nsq connection cannot multiplex", len(channels))
	}
}
