package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	solacedriver "github.com/amigoer/mq-studio/internal/driver/solace"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every Solace board, compared against the raw SEMP API.
 *
 * Almost every figure this family shows is something the driver assembled
 * rather than read. A queue's depth is not a field at all - it is the count on
 * a sub-collection, because the field that looks like a depth is a lifetime
 * statistic. Its bound consumer count is another sub-collection. The
 * dead-letter page is every endpoint's pointer inverted, and the rows that
 * matter most are the ones whose target does not exist. The routing page is
 * one call per queue folded together. The broker's spool percentage is two
 * numbers in different units divided by each other. Every one of those can be
 * subtly wrong and stay entirely plausible, and a driver testing itself would
 * produce the same wrong answer twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * plain net/http, its own JSON, its own paths. It talks to the same broker,
 * which is the only thing the two sides have in common.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same broker and create and delete queues of their
 * own while these are running. Where a figure can move between two reads -
 * anything the broker is still counting - the two sides are read together and
 * compared as a pair rather than against a literal.
 */

// rawSEMPClient is a minimal SEMP client: basic auth, JSON, and nothing else.
type rawSEMPClient struct {
	base   string
	client *http.Client
}

func newRawSEMP() *rawSEMPClient {
	return &rawSEMPClient{
		base:   liveSolaceAddress,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// object reads one SEMP resource's data.
func (r *rawSEMPClient) object(t *testing.T, path string) map[string]any {
	t.Helper()
	decoded := r.send(t, http.MethodGet, path, nil)
	data, _ := decoded["data"].(map[string]any)
	return data
}

// collection reads a whole SEMP collection, following the cursor the way the
// driver does - but written out here rather than shared, because a paging bug
// the two sides made together would be invisible.
func (r *rawSEMPClient) collection(t *testing.T, path string) []map[string]any {
	t.Helper()
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	var all []map[string]any
	cursor := ""
	for page := 0; page < 50; page++ {
		request := fmt.Sprintf("%s%scount=100", path, separator)
		if cursor != "" {
			request += "&cursor=" + url.QueryEscape(cursor)
		}
		decoded := r.send(t, http.MethodGet, request, nil)

		rows, _ := decoded["data"].([]any)
		for _, row := range rows {
			if entry, ok := row.(map[string]any); ok {
				all = append(all, entry)
			}
		}

		meta, _ := decoded["meta"].(map[string]any)
		paging, _ := meta["paging"].(map[string]any)
		next, _ := paging["cursorQuery"].(string)
		if len(rows) == 0 || next == "" {
			return all
		}
		cursor = next
	}
	return all
}

// count is meta.count on a collection, which is how many exist rather than how
// many came back. It is the only place several of this family's figures live.
func (r *rawSEMPClient) count(t *testing.T, path string) int {
	t.Helper()
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	decoded := r.send(t, http.MethodGet, path+separator+"count=1", nil)
	meta, _ := decoded["meta"].(map[string]any)
	value, _ := meta["count"].(float64)
	return int(value)
}

func (r *rawSEMPClient) send(t *testing.T, method, path string, body []byte) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, r.base+"/SEMP/v2"+path, reader)
	if err != nil {
		t.Fatalf("build the semp request: %v", err)
	}
	request.SetBasicAuth(liveSolaceAdmin, liveSolaceAdminPw)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := r.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s answered something that is not semp: %s", path, string(payload))
	}
	meta, _ := decoded["meta"].(map[string]any)
	if failure, present := meta["error"].(map[string]any); present {
		t.Fatalf("%s %s: %v %v", method, path, failure["status"], failure["description"])
	}
	return decoded
}

func vpnPath(suffix string) string {
	return "/msgVpns/" + url.PathEscape(liveSolaceVPN) + suffix
}

// sempNumber and sempString read one field out of a decoded object. They are
// this file's own rather than shared with the other cross-checks: nothing here
// should be able to fail because another family's helper changed.
func sempNumber(row map[string]any, key string) float64 {
	value, _ := row[key].(float64)
	return value
}

func sempString(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}

/*
 * The queues board, and the figure on it that no field carries.
 *
 * The depth is compared against the message collection's own count, read
 * straight over HTTP - never against spooledMsgCount, which is what the driver
 * deliberately does not use. The seed's audit queue is asserted separately as
 * the case that tells the two apart: it holds nothing and its lifetime counter
 * says three, so a driver reading the obvious field would be caught here and
 * nowhere else.
 */
func TestLiveSolaceCrossCheckQueues(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace cross-check queues"))
	raw := newRawSEMP()

	listed, err := stack.destinations.List(solaceContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	byName := map[string]*model.Destination{}
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
	}

	seeded := []string{liveSolaceOrders, liveSolaceAudit, liveSolaceDMQ, liveSolaceEvents}
	for _, name := range seeded {
		if _, present := byName[name]; !present {
			e2e.Missing(t, "%s is not in the listing; run: npm run e2e:solace:seed", name)
		}
	}

	for _, name := range seeded {
		queue := byName[name]
		want := raw.count(t, "/monitor"+vpnPath("/queues/"+url.PathEscape(name)+"/msgs"))
		if int(queue.Depth) != want {
			t.Errorf("%s: the app reports %d spooled and semp counts %d",
				name, queue.Depth, want)
		}

		bound := raw.count(t, "/monitor"+vpnPath("/queues/"+url.PathEscape(name)+"/txFlows"))
		if queue.Subscribers != bound {
			t.Errorf("%s: the app reports %d bound consumers and semp counts %d",
				name, queue.Subscribers, bound)
		}

		config := raw.object(t, "/config"+vpnPath("/queues/"+url.PathEscape(name)))
		if got := queue.Attribute(solacedriver.AttrAccessType); got != sempString(config, "accessType") {
			t.Errorf("%s: access type = %q, semp says %q",
				name, got, sempString(config, "accessType"))
		}
		if got := queue.Attribute(solacedriver.AttrDeadMsgQueue); got != sempString(config, "deadMsgQueue") {
			t.Errorf("%s: dead message queue = %q, semp says %q",
				name, got, sempString(config, "deadMsgQueue"))
		}
	}

	/*
	 * The queue that separates the depth from the statistic.
	 *
	 * The seed's TTL hands everything on it to the dead message queue, so it
	 * holds nothing and spooledMsgCount stays at its high-water mark. A driver
	 * reading that field would report a queue holding three messages that are
	 * not there.
	 */
	audit := byName[liveSolaceAudit]
	monitor := raw.object(t, "/monitor"+vpnPath("/queues/"+url.PathEscape(liveSolaceAudit)))
	statistic := int64(sempNumber(monitor, "spooledMsgCount"))
	if statistic == 0 {
		t.Errorf("%s reports a lifetime spooled count of 0, so this check can no longer "+
			"tell the statistic from the depth; run: npm run e2e:solace:seed", liveSolaceAudit)
	}
	if audit.Depth == statistic {
		t.Errorf("%s: the app reports %d spooled and so does spooledMsgCount, which is a "+
			"lifetime statistic - the depth is meta.count on the message collection",
			liveSolaceAudit, audit.Depth)
	}
	if audit.Depth != 0 {
		t.Errorf("%s: the app reports %d spooled and semp's message collection is empty",
			liveSolaceAudit, audit.Depth)
	}
}

/*
 * The dead messages board, compared against the pointers it was built from.
 *
 * The row that matters is the one whose target does not exist. Every endpoint
 * ships pointing at "#DEAD_MSG_QUEUE" and no broker creates a queue by that
 * name, so a page that dropped those rows - or drew them with a depth of zero -
 * would hide the state this family gets into by default.
 */
func TestLiveSolaceCrossCheckDeadMessageQueues(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace cross-check dead messages"))
	raw := newRawSEMP()

	found, err := stack.solace.DeadLetters(solaceContext(t), connID)
	if err != nil {
		t.Fatalf("dead letter queues: %v", err)
	}
	byName := map[string]*model.DeadLetterQueue{}
	for _, entry := range found {
		byName[entry.Name] = entry
	}

	// Every pointer the broker holds, built here from the raw listings.
	pointers := map[string][]string{}
	for _, row := range raw.collection(t, "/config"+vpnPath("/queues?select=queueName,deadMsgQueue")) {
		target := sempString(row, "deadMsgQueue")
		if target != "" && target != sempString(row, "queueName") {
			pointers[target] = append(pointers[target], sempString(row, "queueName"))
		}
	}
	for _, row := range raw.collection(t,
		"/config"+vpnPath("/topicEndpoints?select=topicEndpointName,deadMsgQueue")) {
		target := sempString(row, "deadMsgQueue")
		if target != "" && target != sempString(row, "topicEndpointName") {
			pointers[target] = append(pointers[target], sempString(row, "topicEndpointName"))
		}
	}
	if len(pointers) == 0 {
		e2e.Missing(t, "nothing in %s dead-letters anywhere; run: npm run e2e:solace:seed",
			liveSolaceVPN)
	}

	for target, sources := range pointers {
		entry, present := byName[target]
		if !present {
			t.Errorf("%s is dead-lettered into by %s and the app does not list it",
				target, strings.Join(sources, ", "))
			continue
		}
		sort.Strings(sources)
		listed := make([]string, 0, len(entry.Sources))
		for _, source := range entry.Sources {
			listed = append(listed, source.Queue)
		}
		sort.Strings(listed)
		if strings.Join(listed, ",") != strings.Join(sources, ",") {
			t.Errorf("%s: the app lists sources %v and semp says %v", target, listed, sources)
		}
	}

	// The seeded target, which exists and has messages on it.
	real, present := byName[liveSolaceDMQ]
	if !present {
		e2e.Missing(t, "%s is not listed; run: npm run e2e:solace:seed", liveSolaceDMQ)
	}
	want := raw.count(t, "/monitor"+vpnPath("/queues/"+url.PathEscape(liveSolaceDMQ)+"/msgs"))
	if int(real.Depth) != want {
		t.Errorf("%s: the app reports %d spooled and semp counts %d",
			liveSolaceDMQ, real.Depth, want)
	}
	if want == 0 {
		t.Errorf("%s holds nothing, so the comparison above proves nothing; "+
			"run: npm run e2e:solace:seed", liveSolaceDMQ)
	}

	// And the target nobody created, which has to be listed with no depth.
	missing, present := byName[liveSolaceMissingDMQ]
	if !present {
		t.Fatalf("%s is not listed, and every endpoint the seed did not configure still "+
			"points at it", liveSolaceMissingDMQ)
	}
	if missing.Depth != model.UnknownMetric {
		t.Errorf("%s reports a depth of %d; a zero would read as an empty queue that works",
			liveSolaceMissingDMQ, missing.Depth)
	}
	// Proved against the broker rather than assumed: if a queue by that name
	// ever existed here, the assertion above would be checking nothing.
	for _, row := range raw.collection(t, "/config"+vpnPath("/queues?select=queueName")) {
		if sempString(row, "queueName") == liveSolaceMissingDMQ {
			t.Errorf("%s exists on this broker, so the assertion above no longer proves "+
				"anything", liveSolaceMissingDMQ)
		}
	}
}

/*
 * The routing board, compared against the subscriptions it was folded from.
 *
 * The driver reads one call per queue because SEMP has no collection of every
 * subscription in a Message VPN; this rebuilds the same set the same way and
 * compares the whole topology rather than one edge, so a queue skipped in the
 * fan-out is caught.
 */
func TestLiveSolaceCrossCheckRouting(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace cross-check routing"))
	raw := newRawSEMP()
	ctx := solaceContext(t)

	bindings, err := stack.routing.Bindings(ctx, connID, "")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	appEdges := map[string]bool{}
	for _, binding := range bindings {
		appEdges[binding.Destination+" -> "+binding.Source] = true
	}

	sempEdges := map[string]bool{}
	for _, row := range raw.collection(t, "/config"+vpnPath("/queues?select=queueName")) {
		queue := sempString(row, "queueName")
		subscriptions := raw.collection(t, "/config"+vpnPath(
			"/queues/"+url.PathEscape(queue)+"/subscriptions?select=subscriptionTopic"))
		for _, subscription := range subscriptions {
			sempEdges[queue+" -> "+sempString(subscription, "subscriptionTopic")] = true
		}
	}
	if len(sempEdges) == 0 {
		e2e.Missing(t, "no queue in %s subscribes to anything; run: npm run e2e:solace:seed",
			liveSolaceVPN)
	}

	/*
	 * Compared in one direction only, and deliberately.
	 *
	 * The driver package's live tests run against this broker at the same time
	 * and add subscriptions of their own, so an edge SEMP has and the app does
	 * not is a real disagreement, while one the app has and SEMP no longer
	 * does is a test that cleaned up between the two reads. Only the seeded
	 * edges are required in both.
	 */
	for edge := range sempEdges {
		if strings.HasPrefix(edge, "mqstudio/seed/") && !appEdges[edge] {
			t.Errorf("semp has the subscription %q and the app does not list it", edge)
		}
	}
	seeded := liveSolaceEvents + " -> " + liveSolaceTopicSub
	if !appEdges[seeded] {
		e2e.Missing(t, "%s is missing; run: npm run e2e:solace:seed", seeded)
	}

	// And the topic endpoints, which are the other half of the page.
	endpoints, err := stack.routing.Exchanges(ctx, connID, "")
	if err != nil {
		t.Fatalf("list topic endpoints: %v", err)
	}
	listed := map[string]*model.Destination{}
	for _, endpoint := range endpoints {
		listed[endpoint.Ref.Name] = endpoint
	}
	for _, row := range raw.collection(t,
		"/config"+vpnPath("/topicEndpoints?select=topicEndpointName")) {
		name := sempString(row, "topicEndpointName")
		if _, present := listed[name]; !present {
			t.Errorf("semp has the topic endpoint %q and the app does not list it", name)
		}
	}
	if _, present := listed[liveSolaceEndpoint]; !present {
		e2e.Missing(t, "%s is not listed; run: npm run e2e:solace:seed", liveSolaceEndpoint)
	}
}

/*
 * The broker board, and the one figure two units apart.
 *
 * msgSpoolUsage is bytes and maxMsgSpoolUsage is megabytes, on the same
 * object. The percentage is recomputed here from the raw pair rather than read
 * from the app, which is the only way an unscaled division would be caught: on
 * a development broker the true answer is 0% and so is the wrong one.
 */
func TestLiveSolaceCrossCheckBroker(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace cross-check broker"))
	raw := newRawSEMP()
	ctx := solaceContext(t)

	nodes, err := stack.cluster.GetBrokers(ctx, connID)
	if err != nil {
		t.Fatalf("list brokers: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("the app lists %d brokers; a redundancy pair shares one virtual router "+
			"and only the active half answers", len(nodes))
	}
	node := nodes[0]

	broker := raw.object(t, "/monitor?select=version")
	if node.Version != sempString(broker, "version") {
		t.Errorf("version = %q, semp says %q", node.Version, sempString(broker, "version"))
	}

	// Both figures out of one read, so a broker spooling between two calls
	// cannot make the two sides disagree about different moments.
	vpn := raw.object(t, "/monitor"+vpnPath("?select=msgSpoolUsage,maxMsgSpoolUsage"))
	usedBytes := int64(sempNumber(vpn, "msgSpoolUsage"))
	maxMb := int64(sempNumber(vpn, "maxMsgSpoolUsage"))
	if maxMb <= 0 {
		t.Fatalf("%s reports no spool quota, so there is nothing to scale against", liveSolaceVPN)
	}
	wanted := int((usedBytes * 100) / (maxMb * 1024 * 1024))
	if node.DiskUsage != wanted {
		t.Errorf("spool usage = %d%%, and %d bytes of %d MB is %d%%",
			node.DiskUsage, usedBytes, maxMb, wanted)
	}

	overview, _, err := stack.cluster.Overview(ctx, connID)
	if err != nil {
		t.Fatalf("cluster overview: %v", err)
	}
	queues := raw.count(t, "/monitor"+vpnPath("/queues"))
	if overview.Destinations != queues {
		t.Errorf("the overview counts %d queues and semp counts %d",
			overview.Destinations, queues)
	}
	endpoints := raw.count(t, "/monitor"+vpnPath("/topicEndpoints"))
	if overview.Subscriptions != endpoints {
		t.Errorf("the overview counts %d topic endpoints and semp counts %d",
			overview.Subscriptions, endpoints)
	}
	if overview.Attribute(solacedriver.AttrMsgVPN) != liveSolaceVPN {
		t.Errorf("the overview is for %q, want %q",
			overview.Attribute(solacedriver.AttrMsgVPN), liveSolaceVPN)
	}
}

/*
 * The clients board, compared against the same listing read raw.
 *
 * Nothing here compares a count. The broker opens a session per REST request
 * and drops it, and the driver package's live tests are sending on the same
 * broker - so both sides are read as sets and only the clients present in both
 * are compared. What is asserted is that every client SEMP reports is one the
 * app reports too, and that the broker's own sessions are marked rather than
 * filtered away.
 */
func TestLiveSolaceCrossCheckClients(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace cross-check clients"))
	raw := newRawSEMP()

	clients, err := stack.solace.Clients(solaceContext(t), connID)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	byName := map[string]*model.ClientConnection{}
	for _, client := range clients {
		byName[client.Name] = client
	}

	rows := raw.collection(t, "/monitor"+vpnPath(
		"/clients?select=clientName,clientUsername,msgVpnName"))
	if len(rows) == 0 {
		t.Fatal("semp lists no clients at all, and the broker holds sessions of its own")
	}

	internal := 0
	matched := 0
	for _, row := range rows {
		name := sempString(row, "clientName")
		client, present := byName[name]
		if !present {
			// A session the broker dropped between the two reads. The REST
			// listener opens one per request, so this is ordinary rather than
			// a disagreement - which is why nothing here compares counts.
			continue
		}
		matched++
		if client.User != sempString(row, "clientUsername") {
			t.Errorf("%s: user = %q, semp says %q",
				name, client.User, sempString(row, "clientUsername"))
		}
		if client.Namespace != sempString(row, "msgVpnName") {
			t.Errorf("%s: message vpn = %q, semp says %q",
				name, client.Namespace, sempString(row, "msgVpnName"))
		}
		if strings.HasPrefix(name, "#") {
			internal++
			if client.Attributes[solacedriver.AttrInternal] != "true" {
				t.Errorf("%s is one of the broker's own sessions and is not marked", name)
			}
		}
	}
	if matched == 0 {
		t.Fatal("not one client appeared in both reads, which is not a broker moving " +
			"underneath the test")
	}
	if internal == 0 {
		t.Error("no session was matched as one of the broker's own, and the internal " +
			"message bus is always connected; hiding those would hide real connections")
	}
}
