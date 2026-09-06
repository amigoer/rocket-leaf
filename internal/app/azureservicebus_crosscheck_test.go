package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	servicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every Azure Service Bus board, compared against the raw API.
 *
 * Almost every figure this family shows is something the driver assembled. The
 * entities board is two listings folded into one, plus a subscription count
 * that is a further request per topic; a subscription's settings are an Atom
 * document flattened into strings; the routing board is three levels of
 * listing walked and turned into a shape RabbitMQ's page draws; and the entity
 * kind is decided by a discriminator, because both kinds answer the same URL.
 * Every one of those can be subtly wrong and stay plausible, and the driver
 * testing itself would produce the same wrong answer twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * plain net/http against the Atom management surface the same emulator serves,
 * its own XML structs, its own SAS signing. azservicebus/admin is deliberately
 * not used here - the driver is a layer over it, and using it on both sides
 * would compare the driver against itself.
 *
 * The messages half cannot be done this way and is not attempted: Service Bus
 * accepts and hands over messages over AMQP 1.0 and nothing else, and its REST
 * surface refuses a send outright. What that half is cross-checked against
 * instead is the seed, which builds its topology through the SDK directly and
 * prints the counts it verified.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same namespace and create and delete entities of
 * their own while these are running.
 */

// rawServiceBus is a minimal Service Bus management client: the Atom surface,
// a SAS token, and nothing else.
type rawServiceBus struct {
	base   string
	key    string
	name   string
	client *http.Client
}

func newRawServiceBus() *rawServiceBus {
	return &rawServiceBus{
		base:   "http://" + liveServiceBusManagement,
		key:    liveServiceBusKey,
		name:   liveServiceBusKeyName,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

/*
 * token signs one request the way every Service Bus client does.
 *
 * Written out rather than imported, which is the whole point of this file: a
 * signature the driver's own SDK also produced would prove nothing about the
 * driver, and the scheme is one HMAC over the URL and an expiry.
 */
func (r *rawServiceBus) token(target string) string {
	encoded := url.QueryEscape(target)
	expiry := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	mac := hmac.New(sha256.New, []byte(r.key))
	mac.Write([]byte(encoded + "\n" + expiry))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("SharedAccessSignature sr=%s&sig=%s&se=%s&skn=%s",
		encoded, url.QueryEscape(signature), expiry, r.name)
}

// get fetches one Atom document and decodes it.
func (r *rawServiceBus) get(ctx context.Context, path string, out any) error {
	target := r.base + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", r.token(target))

	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s: %s", path, response.Status, strings.TrimSpace(string(raw)))
	}
	return xml.Unmarshal(raw, out)
}

/*
 * The Atom shapes, spelled out.
 *
 * Only the fields the boards actually show. Service Bus returns a
 * QueueDescription, a TopicDescription, a SubscriptionDescription or a
 * RuleDescription inside the same <entry><content> wrapper, so one envelope
 * with four optional bodies reads the lot.
 */
type sbRawFeed struct {
	Entries []sbRawEntry `xml:"entry"`
}

type sbRawEntry struct {
	Title   string       `xml:"title"`
	Content sbRawContent `xml:"content"`
}

type sbRawContent struct {
	Queue        *sbRawQueue        `xml:"QueueDescription"`
	Topic        *sbRawTopic        `xml:"TopicDescription"`
	Subscription *sbRawSubscription `xml:"SubscriptionDescription"`
	Rule         *sbRawRule         `xml:"RuleDescription"`
}

type sbRawQueue struct {
	LockDuration       string `xml:"LockDuration"`
	MaxSizeInMegabytes string `xml:"MaxSizeInMegabytes"`
	RequiresSession    string `xml:"RequiresSession"`
	MaxDeliveryCount   string `xml:"MaxDeliveryCount"`
	Status             string `xml:"Status"`
	TimeToLive         string `xml:"DefaultMessageTimeToLive"`
	Partitioned        string `xml:"EnablePartitioning"`
}

type sbRawTopic struct {
	MaxSizeInMegabytes string `xml:"MaxSizeInMegabytes"`
	Status             string `xml:"Status"`
	TimeToLive         string `xml:"DefaultMessageTimeToLive"`
	Partitioned        string `xml:"EnablePartitioning"`
}

type sbRawSubscription struct {
	LockDuration     string `xml:"LockDuration"`
	RequiresSession  string `xml:"RequiresSession"`
	MaxDeliveryCount string `xml:"MaxDeliveryCount"`
	Status           string `xml:"Status"`
	TimeToLive       string `xml:"DefaultMessageTimeToLive"`
}

type sbRawRule struct {
	Filter struct {
		// The kind is an xsi:type attribute rather than an element, which is
		// how one <Filter> carries three different shapes.
		Type          string `xml:"type,attr"`
		SQLExpression string `xml:"SqlExpression"`
		Subject       string `xml:"Label"`
		CorrelationID string `xml:"CorrelationId"`
	} `xml:"Filter"`
	Action struct {
		SQLExpression string `xml:"SqlExpression"`
	} `xml:"Action"`
}

// names is every title in a feed, sorted.
func names(feed sbRawFeed) []string {
	found := make([]string, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		found = append(found, entry.Title)
	}
	sort.Strings(found)
	return found
}

func (r *rawServiceBus) queues(ctx context.Context, t *testing.T) sbRawFeed {
	t.Helper()
	var feed sbRawFeed
	if err := r.get(ctx, "/$Resources/Queues?api-version=2021-05", &feed); err != nil {
		t.Fatalf("raw queues: %v", err)
	}
	return feed
}

func (r *rawServiceBus) topics(ctx context.Context, t *testing.T) sbRawFeed {
	t.Helper()
	var feed sbRawFeed
	if err := r.get(ctx, "/$Resources/Topics?api-version=2021-05", &feed); err != nil {
		t.Fatalf("raw topics: %v", err)
	}
	return feed
}

func (r *rawServiceBus) subscriptions(ctx context.Context, t *testing.T, topic string) sbRawFeed {
	t.Helper()
	var feed sbRawFeed
	if err := r.get(ctx, "/"+topic+"/Subscriptions?api-version=2021-05", &feed); err != nil {
		t.Fatalf("raw subscriptions on %s: %v", topic, err)
	}
	return feed
}

func (r *rawServiceBus) rules(ctx context.Context, t *testing.T, topic, subscription string) sbRawFeed {
	t.Helper()
	var feed sbRawFeed
	path := "/" + topic + "/Subscriptions/" + subscription + "/Rules?api-version=2021-05"
	if err := r.get(ctx, path, &feed); err != nil {
		t.Fatalf("raw rules on %s/%s: %v", topic, subscription, err)
	}
	return feed
}

// seeded keeps only the objects the seed made, because the driver package's
// live tests create and delete their own against the same namespace.
func seeded(found []string) []string {
	kept := make([]string, 0, len(found))
	for _, name := range found {
		if strings.HasPrefix(name, "mqs-seed-") {
			kept = append(kept, name)
		}
	}
	return kept
}

/*
 * The entities board against the two raw listings it is folded from.
 *
 * The fold is the thing worth checking: the management API lists queues and
 * topics at two different URLs and the board shows one table, so a driver that
 * dropped a page, filtered the wrong listing, or labelled a row with the wrong
 * kind would still produce a plausible board.
 */
func TestLiveServiceBusEntitiesMatchTheRawAPI(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus entity cross-check"))
	raw := newRawServiceBus()
	ctx := serviceBusContext(t)

	listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}

	appQueues := []string{}
	appTopics := []string{}
	byName := map[string]*model.Destination{}
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
		if !strings.HasPrefix(entry.Ref.Name, "mqs-seed-") {
			continue
		}
		if entry.Attributes[servicebusdriver.AttrEntityType] == servicebusdriver.EntityTopic {
			appTopics = append(appTopics, entry.Ref.Name)
		} else {
			appQueues = append(appQueues, entry.Ref.Name)
		}
	}
	sort.Strings(appQueues)
	sort.Strings(appTopics)

	rawQueues := seeded(names(raw.queues(ctx, t)))
	rawTopics := seeded(names(raw.topics(ctx, t)))
	if len(rawQueues) == 0 || len(rawTopics) == 0 {
		e2e.Missing(t, "the namespace holds %d seeded queues and %d seeded topics; "+
			"run npm run e2e:azure-servicebus:seed", len(rawQueues), len(rawTopics))
	}

	if strings.Join(appQueues, ",") != strings.Join(rawQueues, ",") {
		t.Errorf("the board lists queues %v and the API lists %v", appQueues, rawQueues)
	}
	if strings.Join(appTopics, ",") != strings.Join(rawTopics, ",") {
		t.Errorf("the board lists topics %v and the API lists %v", appTopics, rawTopics)
	}

	// And the settings on one of each, because the kind decides which half of
	// the API answered and a mislabelled row would carry the wrong fields.
	for _, entry := range raw.queues(ctx, t).Entries {
		if entry.Title != liveServiceBusOrders || entry.Content.Queue == nil {
			continue
		}
		shown := byName[entry.Title]
		if shown == nil {
			t.Fatalf("%s is in the API and not on the board", entry.Title)
		}
		compareSeconds(t, entry.Title, "lock duration",
			entry.Content.Queue.LockDuration,
			shown.Attributes[servicebusdriver.AttrLockDurationSec])
		compare(t, entry.Title, "delivery limit",
			entry.Content.Queue.MaxDeliveryCount,
			shown.Attributes[servicebusdriver.AttrMaxDeliveryCount])
		compare(t, entry.Title, "status",
			entry.Content.Queue.Status, shown.Attributes[servicebusdriver.AttrStatus])
		compare(t, entry.Title, "sessions",
			entry.Content.Queue.RequiresSession,
			shown.Attributes[servicebusdriver.AttrRequiresSession])
	}

	// A topic has no lock duration and no delivery limit at all, which is what
	// the kind is for: those belong to its subscriptions.
	events := byName[liveServiceBusEvents]
	if events == nil {
		e2e.Missing(t, "%s is not on the board; run npm run e2e:azure-servicebus:seed",
			liveServiceBusEvents)
	}
	if events.Attributes[servicebusdriver.AttrLockDurationSec] != "" {
		t.Errorf("%s carries a lock duration, and a topic has none", liveServiceBusEvents)
	}
	if events.Depth != model.UnknownMetric {
		t.Errorf("%s reports a depth of %d, and a topic holds nothing",
			liveServiceBusEvents, events.Depth)
	}
}

/*
 * The subscription count on a topic row, against a listing per topic.
 *
 * It is the figure this board leads with and the one that costs a second
 * request, so it is exactly where a fold can go wrong: a driver that counted
 * the wrong topic's subscriptions, or applied the connection's name prefix to
 * them, would produce a row that looks right on a namespace where every name
 * matches.
 */
func TestLiveServiceBusSubscriptionCountsMatchTheRawAPI(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus subscriber cross-check"))
	raw := newRawServiceBus()
	ctx := serviceBusContext(t)

	listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}

	checked := 0
	for _, entry := range listed {
		if entry.Attributes[servicebusdriver.AttrEntityType] != servicebusdriver.EntityTopic {
			continue
		}
		if !strings.HasPrefix(entry.Ref.Name, "mqs-seed-") {
			continue
		}
		want := len(raw.subscriptions(ctx, t, entry.Ref.Name).Entries)
		if entry.Subscribers != want {
			t.Errorf("%s reports %d subscriptions and the API lists %d",
				entry.Ref.Name, entry.Subscribers, want)
		}
		checked++
	}
	if checked == 0 {
		e2e.Missing(t, "no seeded topics on the board; run npm run e2e:azure-servicebus:seed")
	}
}

/*
 * The subscriptions board against the per-topic listings it walks.
 *
 * The walk is three requests deep - topics, then each topic's subscriptions,
 * then each subscription's rules - and every level is a place a row can go
 * missing without anything failing.
 */
func TestLiveServiceBusSubscriptionsMatchTheRawAPI(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus subscription cross-check"))
	raw := newRawServiceBus()
	ctx := serviceBusContext(t)

	listed, err := stack.subscriptions.List(ctx, connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}

	shown := map[string]*model.Subscription{}
	appNames := []string{}
	for _, entry := range listed {
		path := entry.Ref.Namespace + "/" + entry.Ref.Name
		shown[path] = entry
		if strings.HasPrefix(entry.Ref.Name, "mqs-seed-") {
			appNames = append(appNames, path)
		}
	}
	sort.Strings(appNames)

	rawNames := []string{}
	for _, topic := range seeded(names(raw.topics(ctx, t))) {
		for _, entry := range raw.subscriptions(ctx, t, topic).Entries {
			if strings.HasPrefix(entry.Title, "mqs-seed-") {
				rawNames = append(rawNames, topic+"/"+entry.Title)
			}
		}
	}
	sort.Strings(rawNames)

	if len(rawNames) == 0 {
		e2e.Missing(t, "the namespace holds no seeded subscriptions; "+
			"run npm run e2e:azure-servicebus:seed")
	}
	if strings.Join(appNames, ",") != strings.Join(rawNames, ",") {
		t.Fatalf("the board lists %v and the API lists %v", appNames, rawNames)
	}

	// One subscription's settings in full, which is where the Atom document
	// is flattened into strings.
	for _, entry := range raw.subscriptions(ctx, t, liveServiceBusEvents).Entries {
		if entry.Title != liveServiceBusSubRed || entry.Content.Subscription == nil {
			continue
		}
		row := shown[liveServiceBusEvents+"/"+entry.Title]
		if row == nil {
			t.Fatalf("%s is in the API and not on the board", entry.Title)
		}
		compareSeconds(t, entry.Title, "lock duration",
			entry.Content.Subscription.LockDuration,
			row.Attributes[servicebusdriver.SubAttrLockDurationSec])
		compare(t, entry.Title, "delivery limit",
			entry.Content.Subscription.MaxDeliveryCount,
			row.Attributes[servicebusdriver.SubAttrMaxDeliveryCount])
		compare(t, entry.Title, "status",
			entry.Content.Subscription.Status,
			row.Attributes[servicebusdriver.SubAttrStatus])
		compare(t, entry.Title, "topic",
			liveServiceBusEvents, row.Attributes[servicebusdriver.SubAttrTopic])
	}
}

/*
 * The routing board against the rule listings it is built from.
 *
 * The mapping is the thing under test: a rule is turned into a canonical
 * binding, and the filter - which arrives as an xsi:type attribute with three
 * different bodies behind it - becomes a routing key a shared page draws. A
 * driver that read the wrong body would produce a row that looks like a
 * routing decision and is not the one in force.
 */
func TestLiveServiceBusRulesMatchTheRawAPI(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus rule cross-check"))
	raw := newRawServiceBus()
	ctx := serviceBusContext(t)

	bindings, err := stack.routing.Bindings(ctx, connID, "")
	if err != nil {
		t.Fatalf("bindings: %v", err)
	}
	shown := map[string]*model.Binding{}
	appNames := []string{}
	for _, binding := range bindings {
		path := binding.Source + "/" + binding.Destination + "/" + binding.PropertiesKey
		shown[path] = binding
		if strings.HasPrefix(binding.Source, "mqs-seed-") {
			appNames = append(appNames, path)
		}
	}
	sort.Strings(appNames)

	rawNames := []string{}
	kinds := map[string]string{}
	expressions := map[string]string{}
	for _, topic := range seeded(names(raw.topics(ctx, t))) {
		for _, subscription := range raw.subscriptions(ctx, t, topic).Entries {
			for _, rule := range raw.rules(ctx, t, topic, subscription.Title).Entries {
				path := topic + "/" + subscription.Title + "/" + rule.Title
				rawNames = append(rawNames, path)
				if rule.Content.Rule != nil {
					kinds[path] = rule.Content.Rule.Filter.Type
					expressions[path] = rule.Content.Rule.Filter.SQLExpression
				}
			}
		}
	}
	sort.Strings(rawNames)

	if len(rawNames) == 0 {
		e2e.Missing(t, "the namespace holds no rules; run npm run e2e:azure-servicebus:seed")
	}
	if strings.Join(appNames, ",") != strings.Join(rawNames, ",") {
		t.Fatalf("the board lists %v and the API lists %v", appNames, rawNames)
	}

	/*
	 * The filter kind, mapped from the xsi:type the service actually sends.
	 *
	 * The names differ on purpose - the wire says SqlFilter and the board says
	 * sql - so this is the one place the two vocabularies are checked against
	 * each other rather than assumed to line up.
	 */
	expected := map[string]string{
		"SqlFilter":         servicebusdriver.FilterSQL,
		"CorrelationFilter": servicebusdriver.FilterCorrelation,
		"TrueFilter":        servicebusdriver.FilterTrue,
		"FalseFilter":       servicebusdriver.FilterFalse,
	}
	// The seed builds one of each of the three kinds it can, so a run that
	// compared fewer than three has stopped reading the attribute rather than
	// found agreement.
	compared := map[string]bool{}
	for path, wireKind := range kinds {
		binding := shown[path]
		if binding == nil {
			continue
		}
		want, known := expected[wireKind]
		if !known {
			// A kind the wire has and this driver does not map yet is worth
			// failing on rather than passing over: it would reach the board
			// as "matches everything", which is the most dangerous default.
			t.Errorf("%s has filter type %q, which the driver does not map", path, wireKind)
			continue
		}
		if got := binding.Arguments[servicebusdriver.ArgFilterType]; got != want {
			t.Errorf("%s is shown as a %q filter and the API calls it %q", path, got, wireKind)
		}
		compared[want] = true
		// A SQL rule's routing key is its expression, verbatim: it is what the
		// service evaluates, so anything else on the row would be a summary of
		// a rule rather than the rule.
		if want == servicebusdriver.FilterSQL && binding.RoutingKey != expressions[path] {
			t.Errorf("%s routes on %q and the API says %q",
				path, binding.RoutingKey, expressions[path])
		}
	}
	for _, kind := range []string{
		servicebusdriver.FilterSQL,
		servicebusdriver.FilterCorrelation,
		servicebusdriver.FilterTrue,
	} {
		if !compared[kind] {
			e2e.Missing(t, "no %s rule was compared; run npm run e2e:azure-servicebus:seed", kind)
		}
	}
}

/*
 * The counts, and the one thing this environment cannot cross-check.
 *
 * Service Bus reports a queue's depth and a subscription's backlog in the
 * CountDetails element of the entity's Atom description. The emulator sends
 * that element for a queue and a topic not at all, and for a subscription with
 * its five children renamed to obfuscated tokens - so there is nothing on
 * either side to compare, and a test that compared zero against zero would
 * pass whatever the driver did.
 *
 * What is asserted instead is that the two agree about there being no answer:
 * the raw document carries no readable count, and the board reports unknown
 * rather than a zero. A run where either half changes - the emulator starting
 * to report, or the driver starting to invent - turns this red, which is the
 * only way the gap gets noticed.
 */
func TestLiveServiceBusCountsAreAbsentOnBothSides(t *testing.T) {
	requireLiveServiceBus(t)
	stack := newServiceBusStack(t)
	connID := stack.dial(t, liveServiceBusProfile("service bus count cross-check"))
	raw := newRawServiceBus()
	ctx := serviceBusContext(t)

	// The raw side: an authenticated GET of the queue, looked at as text
	// rather than through a struct, because what is being asserted is the
	// absence of an element rather than the value of one.
	target := raw.base + "/" + liveServiceBusOrders + "?api-version=2021-05"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	request.Header.Set("Authorization", raw.token(target))
	response, err := raw.client.Do(request)
	if err != nil {
		t.Fatalf("raw queue: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("reading the raw queue: %v", err)
	}
	if strings.Contains(string(body), "ActiveMessageCount") {
		t.Fatal("the emulator now reports a readable message count; " +
			"the driver can stop degrading it and this test needs rewriting")
	}

	// The app side: unknown rather than zero, which is the whole difference
	// between "this queue is empty" and "this endpoint reports no counts".
	listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	for _, entry := range listed {
		if entry.Ref.Name != liveServiceBusOrders {
			continue
		}
		if entry.Depth != model.UnknownMetric {
			t.Errorf("%s reports a depth of %d against an endpoint that reports none",
				entry.Ref.Name, entry.Depth)
		}
	}

	groups, err := stack.subscriptions.List(ctx, connID)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	for _, entry := range groups {
		if entry.Backlog != model.UnknownMetric {
			t.Errorf("%s/%s reports a backlog of %d against an endpoint that reports none",
				entry.Ref.Namespace, entry.Ref.Name, entry.Backlog)
		}
	}
}

// compare fails when the board shows something the API does not.
func compare(t *testing.T, entity, field, want, got string) {
	t.Helper()
	if strings.TrimSpace(want) != strings.TrimSpace(got) {
		t.Errorf("%s: the board shows %s %q and the API says %q", entity, field, got, want)
	}
}

/*
 * compareSeconds compares an ISO-8601 duration against the whole seconds the
 * board shows.
 *
 * Parsed here rather than by the driver's own helper: the conversion is one of
 * the things being checked, so borrowing the driver's would compare it against
 * itself. Only the shapes the service actually sends are handled - PT1M, PT5S,
 * PT1H - which is the whole of what a lock duration or a time to live is.
 */
func compareSeconds(t *testing.T, entity, field, iso, got string) {
	t.Helper()
	text := strings.TrimPrefix(strings.TrimSpace(iso), "PT")
	var seconds int
	switch {
	case strings.HasSuffix(text, "H"):
		hours, err := strconv.Atoi(strings.TrimSuffix(text, "H"))
		if err != nil {
			t.Fatalf("%s: could not read %q", entity, iso)
		}
		seconds = hours * 3600
	case strings.HasSuffix(text, "M"):
		minutes, err := strconv.Atoi(strings.TrimSuffix(text, "M"))
		if err != nil {
			t.Fatalf("%s: could not read %q", entity, iso)
		}
		seconds = minutes * 60
	case strings.HasSuffix(text, "S"):
		value, err := strconv.Atoi(strings.TrimSuffix(text, "S"))
		if err != nil {
			t.Fatalf("%s: could not read %q", entity, iso)
		}
		seconds = value
	default:
		t.Fatalf("%s: %q is not a duration this check reads", entity, iso)
	}
	if strconv.Itoa(seconds) != strings.TrimSpace(got) {
		t.Errorf("%s: the board shows %s %qs and the API says %q (%ds)",
			entity, field, got, iso, seconds)
	}
}
