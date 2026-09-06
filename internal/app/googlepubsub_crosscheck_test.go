package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every Google Pub/Sub board, compared against the raw API.
 *
 * Almost every figure this family shows is something the driver assembled. A
 * topic's subscription count is a second request per topic, folded in from a
 * separate call; a subscription's settings are a proto with seven optional
 * sub-messages flattened into strings; the dead-letter board is the whole
 * topology walked backwards out of the subscription listing; and a browse is a
 * pull followed by a release, which is arithmetic on delivery state rather
 * than a read. Every one of those can be subtly wrong and stay plausible, and
 * the driver testing itself would produce the same wrong answer twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * plain net/http against the REST surface the same emulator serves, its own
 * structs, its own decoding. cloud.google.com/go/pubsub is deliberately not
 * used here - the driver is a layer over it, and using it on both sides would
 * compare the driver against itself.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same project and create and delete topics and
 * subscriptions of their own while these are running.
 */

// rawPubSub is a minimal Pub/Sub client: the REST surface, and nothing else.
type rawPubSub struct {
	base string
	http *http.Client
}

func newRawPubSub() *rawPubSub {
	return &rawPubSub{
		base: "http://" + livePubSubEmulator + "/v1/projects/" + livePubSubProject,
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// call makes one request and decodes its answer.
//
// Written out rather than imported, which is the whole point of this file: a
// figure the driver's own client also produced would prove nothing about the
// driver, and the protocol is one request with one header.
func (r *rawPubSub) call(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, r.base+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")

	response, err := r.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s: %s", method, path, response.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

type rawTopic struct {
	Name                     string            `json:"name"`
	Labels                   map[string]string `json:"labels"`
	MessageRetentionDuration string            `json:"messageRetentionDuration"`
}

type rawSubscription struct {
	Name                     string `json:"name"`
	Topic                    string `json:"topic"`
	AckDeadlineSeconds       int    `json:"ackDeadlineSeconds"`
	MessageRetentionDuration string `json:"messageRetentionDuration"`
	RetainAckedMessages      bool   `json:"retainAckedMessages"`
	EnableMessageOrdering    bool   `json:"enableMessageOrdering"`
	Filter                   string `json:"filter"`
	PushConfig               struct {
		PushEndpoint string `json:"pushEndpoint"`
	} `json:"pushConfig"`
	DeadLetterPolicy struct {
		DeadLetterTopic     string `json:"deadLetterTopic"`
		MaxDeliveryAttempts int    `json:"maxDeliveryAttempts"`
	} `json:"deadLetterPolicy"`
	RetryPolicy struct {
		MinimumBackoff string `json:"minimumBackoff"`
		MaximumBackoff string `json:"maximumBackoff"`
	} `json:"retryPolicy"`
}

func (r *rawPubSub) topics(ctx context.Context, t *testing.T) map[string]rawTopic {
	t.Helper()
	var page struct {
		Topics []rawTopic `json:"topics"`
	}
	if err := r.call(ctx, http.MethodGet, "/topics?pageSize=1000", nil, &page); err != nil {
		t.Fatalf("raw list topics: %v", err)
	}
	found := make(map[string]rawTopic, len(page.Topics))
	for _, topic := range page.Topics {
		found[lastSegment(topic.Name)] = topic
	}
	return found
}

func (r *rawPubSub) subscriptions(ctx context.Context, t *testing.T) map[string]rawSubscription {
	t.Helper()
	var page struct {
		Subscriptions []rawSubscription `json:"subscriptions"`
	}
	if err := r.call(ctx, http.MethodGet, "/subscriptions?pageSize=1000", nil, &page); err != nil {
		t.Fatalf("raw list subscriptions: %v", err)
	}
	found := make(map[string]rawSubscription, len(page.Subscriptions))
	for _, subscription := range page.Subscriptions {
		found[lastSegment(subscription.Name)] = subscription
	}
	return found
}

func (r *rawPubSub) topicSubscriptions(ctx context.Context, t *testing.T, topic string) []string {
	t.Helper()
	var page struct {
		Subscriptions []string `json:"subscriptions"`
	}
	if err := r.call(ctx, http.MethodGet, "/topics/"+topic+"/subscriptions", nil, &page); err != nil {
		t.Fatalf("raw list subscriptions on %s: %v", topic, err)
	}
	names := make([]string, 0, len(page.Subscriptions))
	for _, name := range page.Subscriptions {
		names = append(names, lastSegment(name))
	}
	sort.Strings(names)
	return names
}

// pull reads a subscription and hands everything straight back, the way the
// driver does - so the comparison does not itself change what it is measuring.
func (r *rawPubSub) pull(ctx context.Context, t *testing.T, subscription string, max int) []string {
	t.Helper()
	var out struct {
		ReceivedMessages []struct {
			AckID   string `json:"ackId"`
			Message struct {
				Data       string            `json:"data"`
				MessageID  string            `json:"messageId"`
				Attributes map[string]string `json:"attributes"`
			} `json:"message"`
		} `json:"receivedMessages"`
	}
	if err := r.call(ctx, http.MethodPost, "/subscriptions/"+subscription+":pull",
		map[string]any{"maxMessages": max, "returnImmediately": true}, &out); err != nil {
		t.Fatalf("raw pull %s: %v", subscription, err)
	}

	bodies := make([]string, 0, len(out.ReceivedMessages))
	ackIDs := make([]string, 0, len(out.ReceivedMessages))
	for _, received := range out.ReceivedMessages {
		decoded, err := base64.StdEncoding.DecodeString(received.Message.Data)
		if err != nil {
			t.Fatalf("raw pull %s: undecodable body: %v", subscription, err)
		}
		bodies = append(bodies, string(decoded))
		ackIDs = append(ackIDs, received.AckID)
	}
	if len(ackIDs) > 0 {
		if err := r.call(ctx, http.MethodPost, "/subscriptions/"+subscription+":modifyAckDeadline",
			map[string]any{"ackIds": ackIDs, "ackDeadlineSeconds": 0}, nil); err != nil {
			t.Fatalf("raw release %s: %v", subscription, err)
		}
	}
	sort.Strings(bodies)
	return bodies
}

func lastSegment(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// durationSeconds reads the "1200s" the REST surface spells durations in.
func durationSeconds(t *testing.T, raw string) string {
	t.Helper()
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(raw, "s")
	// The emulator answers whole seconds; a fractional one would mean the
	// comparison below has to change rather than be rounded away silently.
	if strings.Contains(trimmed, ".") {
		t.Fatalf("the API answered a fractional duration %q, which this comparison cannot read", raw)
	}
	if _, err := strconv.Atoi(trimmed); err != nil {
		t.Fatalf("unreadable duration %q: %v", raw, err)
	}
	return trimmed
}

/*
 * The topics board, against the raw listing.
 *
 * The subscription count is what the whole board leads with and it is the one
 * figure the driver assembles: ListTopics reports it nowhere, so it is a
 * second request per topic folded in afterwards. A fan-out that counted wrong
 * would look perfectly ordinary.
 */
func TestLivePubSubTopicsMatchTheRawAPI(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub crosscheck topics"))
	ctx := pubsubContext(t)
	raw := newRawPubSub()

	listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	byName := make(map[string]*model.Destination, len(listed))
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
	}

	expected := raw.topics(ctx, t)
	for _, name := range []string{
		livePubSubOrders, livePubSubDeadLetters, livePubSubOrphaned, livePubSubQuiet,
	} {
		if _, seeded := expected[name]; !seeded {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", name)
		}
		shown := byName[name]
		if shown == nil {
			t.Errorf("%s is in the project and not on the board", name)
			continue
		}

		readers := raw.topicSubscriptions(ctx, t, name)
		if shown.Subscribers != len(readers) {
			t.Errorf("%s: board says %d subscriptions, the API says %d (%v)",
				name, shown.Subscribers, len(readers), readers)
		}
		named := strings.Split(shown.Attribute(pubsubdriver.AttrSubscriptionNames), ",")
		if len(readers) == 0 {
			if shown.Attribute(pubsubdriver.AttrSubscriptionNames) != "" {
				t.Errorf("%s: board names %v, and nothing subscribes to it", name, named)
			}
		} else if !slices.Equal(named, readers) {
			t.Errorf("%s: board names %v, the API names %v", name, named, readers)
		}

		want := durationSeconds(t, expected[name].MessageRetentionDuration)
		if got := shown.Attribute(pubsubdriver.AttrRetentionSec); got != want {
			t.Errorf("%s: board says retention %q, the API says %q", name, got, want)
		}

		// The two figures that must stay unknown rather than becoming zero: a
		// topic stores nothing and is not split.
		if shown.Depth != model.UnknownMetric || shown.Partitions != model.UnknownMetric {
			t.Errorf("%s: board invented a depth (%d) or partition count (%d)",
				name, shown.Depth, shown.Partitions)
		}
	}
}

/*
 * The subscriptions board, against the raw listing.
 *
 * A subscription is a proto with seven optional sub-messages, and the driver
 * flattens all of it into strings a board reads by key. A dead-letter policy
 * dropped on the way through, or a backoff read off the wrong field, is a
 * board that looks complete and describes a different subscription.
 */
func TestLivePubSubSubscriptionsMatchTheRawAPI(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub crosscheck subscriptions"))
	ctx := pubsubContext(t)
	raw := newRawPubSub()

	listed, err := stack.subscriptions.List(ctx, connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	byName := make(map[string]*model.Subscription, len(listed))
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
	}

	expected := raw.subscriptions(ctx, t)
	for _, name := range []string{
		livePubSubWorker, livePubSubAudit, livePubSubDeadReader, livePubSubIdle,
	} {
		want, seeded := expected[name]
		if !seeded {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", name)
		}
		shown := byName[name]
		if shown == nil {
			t.Errorf("%s is in the project and not on the board", name)
			continue
		}

		for _, field := range []struct {
			key       string
			board     string
			api       string
			describes string
		}{
			{
				pubsubdriver.SubAttrTopic,
				shown.Attribute(pubsubdriver.SubAttrTopic),
				lastSegment(want.Topic),
				"the topic it reads",
			},
			{
				pubsubdriver.SubAttrAckDeadlineSec,
				shown.Attribute(pubsubdriver.SubAttrAckDeadlineSec),
				strconv.Itoa(want.AckDeadlineSeconds),
				"how long a delivered message is held",
			},
			{
				pubsubdriver.SubAttrRetentionSec,
				shown.Attribute(pubsubdriver.SubAttrRetentionSec),
				durationSeconds(t, want.MessageRetentionDuration),
				"how long an unacknowledged message is kept",
			},
			{
				pubsubdriver.SubAttrDeadLetterTopic,
				shown.Attribute(pubsubdriver.SubAttrDeadLetterTopic),
				lastSegment(want.DeadLetterPolicy.DeadLetterTopic),
				"where it gives up to",
			},
			{
				pubsubdriver.SubAttrRetryMinSec,
				shown.Attribute(pubsubdriver.SubAttrRetryMinSec),
				durationSeconds(t, want.RetryPolicy.MinimumBackoff),
				"the shortest gap before a redelivery",
			},
			{
				pubsubdriver.SubAttrRetryMaxSec,
				shown.Attribute(pubsubdriver.SubAttrRetryMaxSec),
				durationSeconds(t, want.RetryPolicy.MaximumBackoff),
				"the longest gap before a redelivery",
			},
		} {
			if field.board != field.api {
				t.Errorf("%s: board says %s=%q, the API says %q (%s)",
					name, field.key, field.board, field.api, field.describes)
			}
		}
		if want.DeadLetterPolicy.MaxDeliveryAttempts > 0 {
			attempts := strconv.Itoa(want.DeadLetterPolicy.MaxDeliveryAttempts)
			if shown.Attribute(pubsubdriver.SubAttrMaxAttempts) != attempts {
				t.Errorf("%s: board says %q delivery attempts, the API says %q",
					name, shown.Attribute(pubsubdriver.SubAttrMaxAttempts), attempts)
			}
		}
		if shown.Attribute(pubsubdriver.SubAttrRetainAcked) != strconv.FormatBool(want.RetainAckedMessages) {
			t.Errorf("%s: board says retainAcked=%q, the API says %v",
				name, shown.Attribute(pubsubdriver.SubAttrRetainAcked), want.RetainAckedMessages)
		}

		// The backlog, which must stay unknown on every one of them: the
		// figure is a Cloud Monitoring metric and this API reports none.
		if shown.Backlog != model.UnknownMetric {
			t.Errorf("%s: board says a backlog of %d, and no call in this API reports one",
				name, shown.Backlog)
		}
	}
}

/*
 * The dead-letter board, against the raw listing it was inverted from.
 *
 * Nothing in the API answers "what gives up into this topic", so the driver
 * walks every subscription and inverts their policies. An inversion that lost
 * a source, or attributed one to the wrong topic, would still produce a board
 * that reads sensibly.
 */
func TestLivePubSubDeadLettersMatchTheRawAPI(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub crosscheck dead letters"))
	ctx := pubsubContext(t)
	raw := newRawPubSub()

	shown, err := stack.pubsub.DeadLetterQueues(ctx, connID)
	if err != nil {
		t.Fatalf("dead letters: %v", err)
	}
	byName := make(map[string]*model.DeadLetterQueue, len(shown))
	for _, entry := range shown {
		byName[entry.Name] = entry
	}

	// The same walk, done independently: every subscription that names a
	// dead-letter topic, grouped by that topic.
	expected := map[string][]string{}
	for name, subscription := range raw.subscriptions(ctx, t) {
		target := lastSegment(subscription.DeadLetterPolicy.DeadLetterTopic)
		if target == "" {
			continue
		}
		expected[target] = append(expected[target], name)
	}
	if len(expected) == 0 {
		e2e.Missing(t, "no subscription dead-letters; run `npm run e2e:google-pubsub:seed`")
	}

	for target, wantSources := range expected {
		sort.Strings(wantSources)
		entry := byName[target]
		if entry == nil {
			t.Errorf("%s is dead-lettered into and is not on the board", target)
			continue
		}

		gotSources := make([]string, 0, len(entry.Sources))
		for _, source := range entry.Sources {
			gotSources = append(gotSources, source.Subscription)
			// The half a RabbitMQ source cannot carry: the topic the message
			// came from, which is the subscription's topic rather than the
			// dead-letter one.
			if source.Queue == "" {
				t.Errorf("%s: a source names no topic at all", target)
			}
		}
		sort.Strings(gotSources)
		if !slices.Equal(gotSources, wantSources) {
			t.Errorf("%s: board says %v gives up here, the API says %v",
				target, gotSources, wantSources)
		}

		readers := raw.topicSubscriptions(ctx, t, target)
		if entry.Consumers != len(readers) {
			t.Errorf("%s: board says %d subscriptions read it, the API says %d",
				target, entry.Consumers, len(readers))
		}
		if entry.Depth != model.UnknownMetric {
			t.Errorf("%s: board says a depth of %d, and a topic holds nothing countable",
				target, entry.Depth)
		}
	}
}

/*
 * The browse, against a raw pull of the same subscription.
 *
 * The driver's browse is a pull followed by a release, and the release is what
 * makes it repeatable: without it the second read finds nothing. So the
 * comparison is only meaningful if this side releases too, which it does - and
 * on a subscription with no dead-letter policy, so the delivery attempts spent
 * proving it cannot move the seed's messages anywhere.
 */
func TestLivePubSubBrowseMatchesARawPull(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub crosscheck browse"))
	ctx := pubsubContext(t)
	raw := newRawPubSub()

	shown, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{
		Topic: livePubSubAudit, MaxResults: 20,
	})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed` (%v)",
			livePubSubAudit, err)
	}
	if len(shown) == 0 {
		e2e.Missing(t, "%s is holding nothing; run `npm run e2e:google-pubsub:seed`",
			livePubSubAudit)
	}

	bodies := make([]string, 0, len(shown))
	for _, message := range shown {
		bodies = append(bodies, message.Body)
		// The read is of a subscription, which is the one place this family's
		// vocabulary differs from every other: a topic holds nothing.
		if message.Topic != livePubSubAudit {
			t.Errorf("message %s reports %q, want the subscription it was read from",
				message.MessageID, message.Topic)
		}
	}
	sort.Strings(bodies)

	// Both sides released what they took, so the same messages are there for
	// the second read.
	want := raw.pull(ctx, t, livePubSubAudit, 20)
	if !slices.Equal(bodies, want) {
		t.Errorf("board browsed %v, a raw pull found %v", bodies, want)
	}
}

/*
 * The overview's two figures, against the same walk done independently.
 *
 * Both count a fan-out that is broken while everything else reports success: a
 * topic nothing subscribes to discards every publish, and a subscription whose
 * topic is gone will never receive again. Neither has any other symptom, so a
 * miscount here is a page that says all is well.
 */
func TestLivePubSubBrokenFanOutMatchesTheRawAPI(t *testing.T) {
	requireLivePubSub(t)
	stack := newPubSubStack(t)
	connID := stack.dial(t, livePubSubProfile("pubsub crosscheck fan-out"))
	ctx := pubsubContext(t)
	raw := newRawPubSub()

	topics, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	for _, entry := range topics {
		readers := raw.topicSubscriptions(ctx, t, entry.Ref.Name)
		if (entry.Subscribers == 0) != (len(readers) == 0) {
			t.Errorf("%s: board says %d subscriptions, the API says %d - one of them calls it "+
				"a topic that discards everything and the other does not",
				entry.Ref.Name, entry.Subscribers, len(readers))
		}
	}
	if orphaned := findDestinationNamed(topics, livePubSubOrphaned); orphaned == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", livePubSubOrphaned)
	} else if orphaned.Subscribers != 0 {
		t.Errorf("%s has %d subscriptions and the seed gives it none",
			livePubSubOrphaned, orphaned.Subscribers)
	}

	subscriptions, err := stack.subscriptions.List(ctx, connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	expected := raw.subscriptions(ctx, t)
	for _, entry := range subscriptions {
		want, known := expected[entry.Ref.Name]
		if !known {
			// Created or deleted by another test between the two reads, which
			// is ordinary in a project this suite shares with itself.
			continue
		}
		orphaned := entry.Attribute(pubsubdriver.SubAttrTopic) == deletedTopicMarker
		if orphaned != (want.Topic == "_deleted-topic_") {
			t.Errorf("%s: board says topic %q, the API says %q",
				entry.Ref.Name, entry.Attribute(pubsubdriver.SubAttrTopic), want.Topic)
		}
		if orphaned && entry.Status != model.SubscriptionOffline {
			t.Errorf("%s: its topic is gone and the board calls it %q",
				entry.Ref.Name, entry.Status)
		}
	}
}

// deletedTopicMarker is the literal Pub/Sub puts in a subscription's topic
// field once that topic is gone. Written out here rather than imported,
// because the point of this file is to compare the driver against something
// that shares nothing with it.
const deletedTopicMarker = "_deleted-topic_"

func findDestinationNamed(list []*model.Destination, name string) *model.Destination {
	for _, entry := range list {
		if entry.Ref.Name == name {
			return entry
		}
	}
	return nil
}
