package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	ibmmqdriver "github.com/amigoer/mq-studio/internal/driver/ibmmq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every IBM MQ board, compared against the raw REST API.
 *
 * Almost every figure this family shows is something the driver assembled. The
 * queues board is one REST listing whose eighteen fields are lifted out of
 * four nested objects, plus a second and third read for the topics and their
 * subscriber counts. The channels board is two MQSC calls joined by name, with
 * the definition's fields overwritten by a running instance's where there is
 * one and a status chosen from several instances by how bad it is. A
 * subscription's backlog is not its own at all - it is the depth of another
 * object. And a message's body is one call and its identifier another. Every
 * one of those can be subtly wrong and stay entirely plausible, and a driver
 * testing itself would produce the same wrong answer twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * plain net/http, its own JSON, its own MQSC payloads. It talks to the same
 * mqweb server, which is the only thing the two sides have in common.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same queue manager and create and delete queues
 * of their own while these are running.
 */

// rawMQWeb is a minimal mqweb client: basic auth, a CSRF header, and nothing
// else. It holds both accounts because the server authorises its two
// interfaces separately and the developer image puts one user on each.
type rawMQWeb struct {
	base   string
	client *http.Client
}

func newRawMQWeb() *rawMQWeb {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a self-signed development certificate
	return &rawMQWeb{
		base:   liveIBMMQAddress,
		client: &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// get reads one resource as the administrative account.
func (r *rawMQWeb) get(t *testing.T, path string) map[string]any {
	t.Helper()
	return r.send(t, http.MethodGet, path, liveIBMMQAdmin, liveIBMMQAdminPw, "", nil)
}

// mqsc runs one MQSC command and returns the decoded objects, in the order the
// command server answered.
func (r *rawMQWeb) mqsc(t *testing.T, request map[string]any) []map[string]any {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode the mqsc request: %v", err)
	}
	decoded := r.send(t, http.MethodPost,
		"/ibmmq/rest/v1/admin/action/qmgr/"+liveIBMMQManager+"/mqsc",
		liveIBMMQAdmin, liveIBMMQAdminPw, "application/json", body)

	responses, _ := decoded["commandResponse"].([]any)
	objects := make([]map[string]any, 0, len(responses))
	for _, entry := range responses {
		result, _ := entry.(map[string]any)
		parameters, ok := result["parameters"].(map[string]any)
		if !ok {
			continue
		}
		objects = append(objects, parameters)
	}
	return objects
}

// send is one request. Failures are fatal: this side is the reference, so a
// broken read here is a broken test rather than a finding about the driver.
func (r *rawMQWeb) send(
	t *testing.T, method, path, user, password, contentType string, body []byte,
) map[string]any {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, r.base+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	request.SetBasicAuth(user, password)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet {
		// The server checks only that the header is there; a browser cannot
		// add one to a cross-site form post, which is the whole mechanism.
		request.Header.Set("ibm-mq-rest-csrf-token", "crosscheck")
	}

	response, err := r.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	if response.StatusCode >= 400 {
		t.Fatalf("%s %s answered %d: %s", method, path, response.StatusCode, payload)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s %s did not answer json: %v", method, path, err)
	}
	return decoded
}

// queue reads one queue's configuration and status straight from the resource.
func (r *rawMQWeb) queue(t *testing.T, name string) map[string]any {
	t.Helper()
	decoded := r.get(t, fmt.Sprintf(
		"/ibmmq/rest/v1/admin/qmgr/%s/queue/%s?attributes=*&status=*",
		liveIBMMQManager, url.PathEscape(name)))
	entries, _ := decoded["queue"].([]any)
	if len(entries) == 0 {
		t.Fatalf("%s does not exist on the queue manager", name)
	}
	entry, _ := entries[0].(map[string]any)
	return entry
}

// nested walks the objects the queue resource nests its fields inside.
func nested(t *testing.T, object map[string]any, path ...string) any {
	t.Helper()
	var current any = object
	for _, part := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = asMap[part]
	}
	return current
}

func rawNumber(t *testing.T, value any, what string) int64 {
	t.Helper()
	asFloat, ok := value.(float64)
	if !ok {
		t.Fatalf("%s is %v, not a number", what, value)
	}
	return int64(asFloat)
}

func rawText(value any) string {
	text, _ := value.(string)
	return strings.TrimRight(text, " ")
}

/*
 * The queues board, row by row.
 *
 * The row is not a listing entry: the resource nests its fields inside
 * general, storage, extended, cluster and status, and the driver lifts
 * eighteen of them out into a flat attribute map. This compares that map
 * against the same resource read independently, which is what would catch a
 * field lifted out of the wrong object or a status attached to the wrong queue.
 */
func TestLiveIBMMQCrossCheckQueuesBoard(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck queues"))
	raw := newRawMQWeb()

	listed, err := stack.destinations.List(ibmmqContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := make(map[string]*model.Destination, len(listed))
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
	}

	for _, name := range []string{
		liveIBMMQOrders, liveIBMMQAudit, liveIBMMQBackout, liveIBMMQSubQ, liveIBMMQXmitQ,
		liveIBMMQDeadLetr,
	} {
		row := byName[name]
		if row == nil {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:ibmmq:seed`", name)
			return
		}
		object := raw.queue(t, name)

		if want := rawNumber(t, nested(t, object, "status", "currentDepth"), name+" currentDepth"); row.Depth != want {
			t.Errorf("%s: the board shows depth %d, the API says %d", name, row.Depth, want)
		}
		if want := rawNumber(t, nested(t, object, "status", "openInputCount"), name+" openInputCount"); int64(row.Subscribers) != want {
			t.Errorf("%s: the board shows %d readers, the API says %d", name, row.Subscribers, want)
		}
		if want := rawNumber(t, nested(t, object, "storage", "maximumDepth"), name+" maximumDepth"); row.Attribute(ibmmqdriver.AttrMaxDepth) != fmt.Sprint(want) {
			t.Errorf("%s: the board shows maximum depth %q, the API says %d",
				name, row.Attribute(ibmmqdriver.AttrMaxDepth), want)
		}
		if want := rawText(object["type"]); row.Attribute(ibmmqdriver.AttrQueueType) != want {
			t.Errorf("%s: the board calls it a %q queue, the API says %q",
				name, row.Attribute(ibmmqdriver.AttrQueueType), want)
		}
		// The two that decide whether a queue accepts anything at all, and the
		// commonest reason a queue manager dead-letters.
		if want, _ := nested(t, object, "general", "inhibitPut").(bool); row.Attribute(ibmmqdriver.AttrInhibitPut) != fmt.Sprint(want) {
			t.Errorf("%s: the board says inhibitPut=%q, the API says %v",
				name, row.Attribute(ibmmqdriver.AttrInhibitPut), want)
		}
		if want, _ := nested(t, object, "general", "isTransmissionQueue").(bool); row.Attribute(ibmmqdriver.AttrTransmission) != fmt.Sprint(want) {
			t.Errorf("%s: the board says transmissionQueue=%q, the API says %v",
				name, row.Attribute(ibmmqdriver.AttrTransmission), want)
		}
		// The pointer the dead-letter page is built from.
		if want := rawText(nested(t, object, "extended", "backoutRequeueQueueName")); row.Attribute(ibmmqdriver.AttrBackoutQueue) != want {
			t.Errorf("%s: the board shows backout queue %q, the API says %q",
				name, row.Attribute(ibmmqdriver.AttrBackoutQueue), want)
		}

		// The three the board must never invent, whatever the API says.
		if row.Partitions != model.UnknownMetric {
			t.Errorf("%s: the board reports %d partitions, and IBM MQ divides nothing",
				name, row.Partitions)
		}
		if row.RateIn != model.UnknownMetric || row.RateOut != model.UnknownMetric {
			t.Errorf("%s: the board reports rates (%d in, %d out), and no call returns one",
				name, row.RateIn, row.RateOut)
		}
	}

	// The topic half, which does not come from the resource at all - there is
	// no topic endpoint, so the driver reads MQSC and the comparison has to.
	topic := byName[liveIBMMQTopic]
	if topic == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:ibmmq:seed`", liveIBMMQTopic)
		return
	}
	objects := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "topic",
		"name": liveIBMMQTopic, "responseParameters": []string{"topicstr", "type"},
	})
	if len(objects) != 1 {
		t.Fatalf("the command server answered with %d topics named %s", len(objects), liveIBMMQTopic)
	}
	if want := rawText(objects[0]["topicstr"]); topic.Attribute(ibmmqdriver.AttrTopicString) != want {
		t.Errorf("%s: the board shows topic string %q, MQSC says %q",
			liveIBMMQTopic, topic.Attribute(ibmmqdriver.AttrTopicString), want)
	}
	if topic.Attribute(ibmmqdriver.AttrKind) != ibmmqdriver.KindTopic {
		t.Errorf("%s came back as a %q", liveIBMMQTopic, topic.Attribute(ibmmqdriver.AttrKind))
	}
}

/*
 * The channels board, against both MQSC calls it is built from.
 *
 * This is the row with the most folding behind it: a definition and its
 * running instances are two separate displays, and the board's row takes some
 * fields from one and some from the other. So the comparison reads both
 * independently - which is what would catch a status joined to the wrong
 * channel, or a definition's blank connection name overwriting a running
 * instance's real one.
 */
func TestLiveIBMMQCrossCheckChannelsBoard(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck channels"))
	raw := newRawMQWeb()

	channels, err := stack.ibmmq.Channels(ibmmqContext(t), connID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	byName := make(map[string]*model.Channel, len(channels))
	for _, channel := range channels {
		byName[channel.Name] = channel
	}

	definitions := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "channel",
		"name": "*", "responseParameters": []string{"chltype", "conname", "xmitq"},
	})
	if len(definitions) != len(channels) {
		t.Errorf("the board draws %d channels and MQSC lists %d", len(channels), len(definitions))
	}
	for _, definition := range definitions {
		name := rawText(definition["channel"])
		channel := byName[name]
		if channel == nil {
			t.Errorf("MQSC lists %s and the board does not draw it", name)
			continue
		}
		// The type decides which group the row is drawn in, and MQSC spells it
		// in abbreviations the driver expands - a mapping with no other test.
		if want := rawText(definition["chltype"]); !strings.EqualFold(expandedType(channel.Type), want) {
			t.Errorf("%s: the board calls it %q, MQSC says %q", name, channel.Type, want)
		}
	}

	statuses := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "chstatus",
		"name": "*", "responseParameters": []string{"all"},
	})
	running := make(map[string]int, len(statuses))
	for _, status := range statuses {
		running[rawText(status["channel"])]++
	}
	for name, channel := range byName {
		if channel.Instances != running[name] {
			t.Errorf("%s: the board reports %d running instances, MQSC reports %d",
				name, channel.Instances, running[name])
		}
		// A definition with nothing running must report no status at all: the
		// difference between "inactive" and "never started" is the whole value
		// of the page, and a driver defaulting the empty case would erase it.
		if running[name] == 0 && channel.Status != "" {
			t.Errorf("%s: the board shows status %q and MQSC reports no status row",
				name, channel.Status)
		}
	}

	seeded := byName[liveIBMMQChannel]
	if seeded == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:ibmmq:seed`", liveIBMMQChannel)
		return
	}
	for _, status := range statuses {
		if rawText(status["channel"]) != liveIBMMQChannel {
			continue
		}
		if want := strings.ToLower(rawText(status["status"])); string(seeded.Status) != want {
			t.Errorf("%s: the board shows status %q, MQSC says %q",
				liveIBMMQChannel, seeded.Status, want)
		}
		// The running instance's connection name, which is what has to survive
		// onto the row - the definition carries one too, and either could win.
		if want := rawText(status["conname"]); seeded.ConnectionName != want {
			t.Errorf("%s: the board shows connection %q, MQSC says %q",
				liveIBMMQChannel, seeded.ConnectionName, want)
		}
	}
}

// expandedType turns the canonical channel type back into MQSC's abbreviation,
// so the two can be compared without the driver's own mapping.
func expandedType(kind model.ChannelType) string {
	switch kind {
	case model.ChannelServerConnection:
		return "SVRCONN"
	case model.ChannelClientConnection:
		return "CLNTCONN"
	case model.ChannelSender:
		return "SDR"
	case model.ChannelReceiver:
		return "RCVR"
	case model.ChannelServer:
		return "SVR"
	case model.ChannelRequester:
		return "RQSTR"
	case model.ChannelClusterSender:
		return "CLUSSDR"
	case model.ChannelClusterReceiver:
		return "CLUSRCVR"
	case model.ChannelAMQP:
		return "AMQP"
	default:
		return string(kind)
	}
}

/*
 * The subscriptions board, and the backlog that belongs to a different object.
 *
 * The row joins three reads: the definition, its runtime status, and the depth
 * of the queue it delivers to. Only the last of those is a number anybody
 * would think to check, and it is the one that would be silently wrong if the
 * driver matched a subscription to the wrong queue.
 */
func TestLiveIBMMQCrossCheckSubscriptionsBoard(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck subscriptions"))
	raw := newRawMQWeb()

	subscriptions, err := stack.subscriptions.List(ibmmqContext(t), connID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var seeded *model.Subscription
	for _, subscription := range subscriptions {
		if subscription.Ref.Name == liveIBMMQSub {
			seeded = subscription
		}
	}
	if seeded == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:ibmmq:seed`", liveIBMMQSub)
		return
	}

	definitions := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "sub",
		"name": liveIBMMQSub, "responseParameters": []string{"topicstr", "dest", "durable", "subtype"},
	})
	if len(definitions) != 1 {
		t.Fatalf("the command server answered with %d subscriptions named %s",
			len(definitions), liveIBMMQSub)
	}
	definition := definitions[0]

	if want := rawText(definition["topicstr"]); seeded.Attribute(ibmmqdriver.SubAttrTopicString) != want {
		t.Errorf("%s: the board shows topic string %q, MQSC says %q",
			liveIBMMQSub, seeded.Attribute(ibmmqdriver.SubAttrTopicString), want)
	}
	destination := rawText(definition["dest"])
	if seeded.Attribute(ibmmqdriver.SubAttrDestination) != destination {
		t.Errorf("%s: the board shows destination %q, MQSC says %q",
			liveIBMMQSub, seeded.Attribute(ibmmqdriver.SubAttrDestination), destination)
	}

	// The backlog, read from the queue rather than from the subscription -
	// which is the only place it exists.
	queue := raw.queue(t, destination)
	if want := rawNumber(t, nested(t, queue, "status", "currentDepth"), destination+" currentDepth"); seeded.Backlog != want {
		t.Errorf("%s: the board shows a backlog of %d and %s holds %d",
			liveIBMMQSub, seeded.Backlog, destination, want)
	}

	// And the lifetime total, which is a different number and comes from a
	// different command: a board that showed one for the other would look
	// entirely reasonable.
	statuses := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "sbstatus",
		"name": liveIBMMQSub, "responseParameters": []string{"nummsgs", "actconn"},
	})
	if len(statuses) != 1 {
		t.Fatalf("the command server answered with %d statuses for %s", len(statuses), liveIBMMQSub)
	}
	received := rawNumber(t, statuses[0]["nummsgs"], "nummsgs")
	if seeded.Attribute(ibmmqdriver.SubAttrMessages) != fmt.Sprint(received) {
		t.Errorf("%s: the board shows %q received, MQSC says %d",
			liveIBMMQSub, seeded.Attribute(ibmmqdriver.SubAttrMessages), received)
	}
	if received == seeded.Backlog && seeded.Backlog == 0 {
		e2e.Missing(t, "%s has received nothing; run `npm run e2e:ibmmq:seed`", liveIBMMQSub)
	}
}

/*
 * The messages board, against the same two calls made independently - and
 * against the depth, which is what proves the browse took nothing.
 */
func TestLiveIBMMQCrossCheckMessagesBoard(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck messages"))
	raw := newRawMQWeb()

	before := rawNumber(t, nested(t, raw.queue(t, liveIBMMQOrders), "status", "currentDepth"),
		liveIBMMQOrders+" currentDepth")
	if before == 0 {
		e2e.Missing(t, "%s is empty; run `npm run e2e:ibmmq:seed`", liveIBMMQOrders)
		return
	}

	browsed, err := stack.messages.Query(ibmmqContext(t), connID, model.MessageQueryParams{
		Topic: liveIBMMQOrders,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if int64(len(browsed)) != before {
		t.Errorf("the board shows %d messages and the queue holds %d", len(browsed), before)
	}

	// The identifiers, in order, against the list the driver did not read. The
	// limit is named because the server's own default is ten, which is fewer
	// than the driver asks for and would look like a driver inventing rows.
	listed := raw.send(t, http.MethodGet, fmt.Sprintf(
		"/ibmmq/rest/v1/messaging/qmgr/%s/queue/%s/messagelist?limit=200",
		liveIBMMQManager, url.PathEscape(liveIBMMQOrders)),
		liveIBMMQApp, liveIBMMQAppPw, "", nil)
	entries, _ := listed["messages"].([]any)
	if len(entries) != len(browsed) {
		t.Fatalf("the board shows %d messages and the raw list holds %d", len(browsed), len(entries))
	}
	for index, entry := range entries {
		message, _ := entry.(map[string]any)
		want := rawText(message["messageId"])
		if !strings.EqualFold(browsed[index].MessageID, want) {
			t.Errorf("message %d: the board shows id %q, the raw list says %q",
				index, browsed[index].MessageID, want)
		}
		if got := browsed[index].Properties[ibmmqdriver.PropFormat]; got != rawText(message["format"]) {
			t.Errorf("message %d: the board shows format %q, the raw list says %q",
				index, got, rawText(message["format"]))
		}
	}

	// And the depth afterwards, which is the whole claim about browsing here:
	// the other families reached through a management API cannot say this.
	after := rawNumber(t, nested(t, raw.queue(t, liveIBMMQOrders), "status", "currentDepth"),
		liveIBMMQOrders+" currentDepth")
	if after != before {
		t.Errorf("%s held %d messages and holds %d after the board browsed it",
			liveIBMMQOrders, before, after)
	}
}

/*
 * The dead-letter board, against the two attributes it is built by inverting.
 *
 * Nothing on the queue manager marks a dead-letter queue, so this page is
 * entirely derived - and the derivation is the thing worth checking: the queue
 * manager's DEADQ attribute and every queue's backout queue, read here
 * independently and inverted the same way.
 */
func TestLiveIBMMQCrossCheckDeadLetterBoard(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck dead letters"))
	raw := newRawMQWeb()

	queues, err := stack.ibmmq.DeadLetters(ibmmqContext(t), connID)
	if err != nil {
		t.Fatalf("DeadLetters: %v", err)
	}
	byName := make(map[string]*model.DeadLetterQueue, len(queues))
	for _, queue := range queues {
		byName[queue.Name] = queue
	}

	qmgr := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "qmgr",
		"responseParameters": []string{"deadq"},
	})
	if len(qmgr) != 1 {
		t.Fatalf("the command server answered with %d queue managers", len(qmgr))
	}
	deadLetter := rawText(qmgr[0]["deadq"])
	if deadLetter == "" {
		e2e.Missing(t, "the queue manager names no DEADQ; the developer image sets one")
		return
	}
	entry := byName[deadLetter]
	if entry == nil {
		t.Fatalf("the queue manager names %s as its DEADQ and the board does not list it",
			deadLetter)
	}
	if want := rawNumber(t, nested(t, raw.queue(t, deadLetter), "status", "currentDepth"), deadLetter+" currentDepth"); entry.Depth != want {
		t.Errorf("%s: the board shows %d dead letters, the API says %d",
			deadLetter, entry.Depth, want)
	}

	// Every backout queue named by any queue, inverted independently. A driver
	// that read the attribute off the wrong queue would produce a page that
	// looks right and points at the wrong sources.
	definitions := raw.mqsc(t, map[string]any{
		"type": "runCommandJSON", "command": "display", "qualifier": "qlocal",
		"name": "*", "responseParameters": []string{"boqname", "bothresh"},
	})
	expected := make(map[string]map[string]bool)
	for _, definition := range definitions {
		target := rawText(definition["boqname"])
		source := rawText(definition["queue"])
		if target == "" || target == source {
			continue
		}
		if expected[target] == nil {
			expected[target] = map[string]bool{}
		}
		expected[target][source] = true
	}

	for target, sources := range expected {
		listedEntry := byName[target]
		if listedEntry == nil {
			t.Errorf("%s is named as a backout queue and the board does not list it", target)
			continue
		}
		found := map[string]bool{}
		for _, source := range listedEntry.Sources {
			if source.Queue != liveIBMMQManager {
				found[source.Queue] = true
			}
		}
		for source := range sources {
			if !found[source] {
				t.Errorf("%s backs out into %s and the board does not say so", source, target)
			}
		}
	}
}

/*
 * A send, read back through the board that shows it.
 *
 * The two halves go through two interfaces authorised against two roles, so
 * this is the one path where a credential mix-up produces a send that appears
 * to work and a browse that cannot see it.
 */
func TestLiveIBMMQCrossCheckASendLandsWhereItSaid(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq crosscheck send"))
	raw := newRawMQWeb()
	ctx := ibmmqContext(t)

	queue := ibmmqTestName(t, ".SEND")
	if err := stack.ibmmq.CreateDestination(ctx, connID, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: queue},
		Attributes: map[string]string{ibmmqdriver.AttrKind: ibmmqdriver.KindQueue},
	}); err != nil {
		t.Fatalf("create %s: %v", queue, err)
	}
	t.Cleanup(func() {
		_ = stack.ibmmq.RemoveDestination(context.Background(), connID, queue, true)
	})

	const body = "crosscheck body"
	result, err := stack.ibmmq.Publish(ctx, connID, ibmmqdriver.PublishRequest{
		Queue:      queue,
		Body:       body,
		Persistent: true,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.MessageID == "" {
		t.Fatal("the send reported no message id, and the queue manager assigns one")
	}

	// The depth, from the administrative interface the send did not use.
	if want := rawNumber(t, nested(t, raw.queue(t, queue), "status", "currentDepth"), queue+" currentDepth"); want != 1 {
		t.Errorf("%s holds %d messages after one send", queue, want)
	}

	// And the message itself, read raw with the messaging account.
	found := raw.send(t, http.MethodGet, fmt.Sprintf(
		"/ibmmq/rest/v1/messaging/qmgr/%s/queue/%s/messagelist?limit=200",
		liveIBMMQManager, url.PathEscape(queue)),
		liveIBMMQApp, liveIBMMQAppPw, "", nil)
	entries, _ := found["messages"].([]any)
	if len(entries) != 1 {
		t.Fatalf("the raw message list holds %d messages after one send", len(entries))
	}
	message, _ := entries[0].(map[string]any)
	if want := rawText(message["messageId"]); !strings.EqualFold(result.MessageID, want) {
		t.Errorf("the send reported id %q and the queue holds %q", result.MessageID, want)
	}
}
