package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/settings"
	sqsservice "github.com/amigoer/mq-studio/internal/service/sqs"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The SQS stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * On this family the first risk is the whole point of the phase. An SQS
 * profile has no address at all, so what has to survive the round trip is a
 * region, a credential pair under names of this driver's own, and an endpoint
 * override - and getting any of them wrong produces a profile that saves,
 * reloads and then cannot dial.
 */

const (
	liveSQSEndpoint  = "http://127.0.0.1:4566"
	liveSQSRegion    = "eu-west-1"
	liveSQSAccessKey = "test"
	liveSQSSecretKey = "test"
)

// The queues scripts/e2e-sqs-seed.sh creates.
const (
	liveSQSOrders  = "MQS-SEED-orders"
	liveSQSDLQ     = "MQS-SEED-orders-dlq"
	liveSQSDelayed = "MQS-SEED-delayed"
	liveSQSEmpty   = "MQS-SEED-empty"
	liveSQSFIFO    = "MQS-SEED-orders.fifo"
)

func requireLiveSQS(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.SQS,
		Name:   "the sqs e2e environment",
		Start:  "npm run e2e:sqs:up",
		Probe:  e2e.HTTPGet(liveSQSEndpoint + "/_localstack/health"),
	})
}

// sqsStack is the connection service, the SQS service and the canonical
// services a board reads through, on a config directory of its own.
type sqsStack struct {
	connections  *connection.Service
	sqs          *sqsservice.Service
	destinations *destination.Service
	messages     *message.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
	// dataFile is where the profiles land, so a second service can be opened
	// on the same store and prove what actually survived disk.
	dataFile string
	settings *settings.Service
}

func newSQSStack(t *testing.T) *sqsStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindSQS); !ok {
		driver.Register(sqsdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &sqsStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		sqs:          sqsservice.New(conns, settingsService),
		destinations: destination.New(conns, settingsService),
		messages:     message.New(conns, settingsService),
		conns:        conns,
		dataFile:     paths.ConnectionsFile,
		settings:     settingsService,
	}
}

// liveSQSProfile is the environment as a user would configure it: a region, a
// credential, and no address anywhere on the form.
func liveSQSProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindSQS,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			sqsdriver.OptionRegion:      liveSQSRegion,
			sqsdriver.OptionEndpointURL: liveSQSEndpoint,
		},
	}
	profile.SetSecret(sqsdriver.SecretAccessKeyID, liveSQSAccessKey)
	profile.SetSecret(sqsdriver.SecretSecretAccessKey, liveSQSSecretKey)
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *sqsStack) dial(t *testing.T, profile model.ConnectionProfile) int {
	t.Helper()
	created, err := s.connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := s.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.connections.Disconnect(created.ID) })
	return created.ID
}

func sqsContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// testQueueVia creates a queue through the service layer and removes it when
// the test ends, so nothing a run leaves behind changes the next one.
func testQueueVia(t *testing.T, stack *sqsStack, connID int, spec sqsdriver.QueueSpec) {
	t.Helper()
	if err := stack.sqs.CreateQueue(sqsContext(t), connID, spec); err != nil {
		t.Fatalf("creating %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = stack.sqs.RemoveQueue(context.Background(), connID, spec.Name)
	})
}

func sqsTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "MQS-TEST-app-" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile with no address, dial it, declare
 * a queue, send to it, and read both back through the id a page would pass.
 */
func TestLiveSQSStackRoundTrip(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)
	connID := stack.dial(t, liveSQSProfile("sqs stack"))
	name := sqsTestName(t, "")

	testQueueVia(t, stack, connID, sqsdriver.QueueSpec{Name: name, VisibilityTimeoutSec: 60})

	result, err := stack.sqs.Publish(sqsContext(t), connID, sqsdriver.PublishRequest{
		Queue: name, Body: "through the stack", Count: 4,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Sent != 4 {
		t.Fatalf("sent %d, want 4", result.Sent)
	}

	destinations, err := stack.destinations.List(sqsContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	var listed *model.Destination
	for _, entry := range destinations {
		if entry.Ref.Name == name {
			listed = entry
		}
	}
	if listed == nil {
		t.Fatal("the queue the stack created is not in the listing")
	}
	if listed.Depth != 4 {
		t.Errorf("depth = %d, want the 4 sent", listed.Depth)
	}

	messages, err := stack.messages.Query(sqsContext(t), connID, model.MessageQueryParams{
		Topic: name, MaxResults: 4,
	})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(messages) != 4 {
		t.Errorf("browsed %d messages, want the 4 sent", len(messages))
	}

	// The renderer's list keys, which the service assigns rather than the
	// driver: a page keys its rows on them, and duplicates make React drop
	// rows silently.
	seen := map[int]bool{}
	for _, entry := range destinations {
		if seen[entry.ID] {
			t.Errorf("two destinations share the list key %d", entry.ID)
		}
		seen[entry.ID] = true
	}
}

/*
 * The addressless profile, proved through disk.
 *
 * This is what phase 11 was for. A profile whose Endpoints is empty has to
 * save - the connection service refuses one that is empty for every other
 * family - come back off disk with its region, its credential and its endpoint
 * override intact, and then dial. Losing any one of them produces a profile
 * that saves and reloads and cannot connect, which is the shape of failure
 * this family is most exposed to: there is no address to look at and notice
 * missing.
 */
func TestLiveSQSAddresslessProfileSurvivesDisk(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)

	input := liveSQSProfile("sqs from disk")
	input.Options[sqsdriver.OptionQueuePrefix] = "MQS-SEED-"
	created, err := stack.connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if created.Endpoints != "" {
		t.Fatalf("the saved profile carries an address %q; this family has none", created.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		stack.dataFile, stack.settings, newRegistryRuntime(driver.NewRegistry()), newDescriptorEndpoints())
	stored, err := reopened.GetConnection(created.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}
	if stored.Endpoints != "" {
		t.Errorf("the stored profile grew an address %q", stored.Endpoints)
	}
	if stored.Option(sqsdriver.OptionRegion) != liveSQSRegion {
		t.Errorf("region after reload = %q, want %q",
			stored.Option(sqsdriver.OptionRegion), liveSQSRegion)
	}
	if stored.Option(sqsdriver.OptionEndpointURL) != liveSQSEndpoint {
		t.Errorf("endpoint override after reload = %q, want %q",
			stored.Option(sqsdriver.OptionEndpointURL), liveSQSEndpoint)
	}
	if stored.Option(sqsdriver.OptionQueuePrefix) != "MQS-SEED-" {
		t.Errorf("queue prefix after reload = %q", stored.Option(sqsdriver.OptionQueuePrefix))
	}
	if stored.Secret(sqsdriver.SecretAccessKeyID) != liveSQSAccessKey ||
		stored.Secret(sqsdriver.SecretSecretAccessKey) != liveSQSSecretKey {
		t.Errorf("credentials after reload = %q / %q, want the pair that was saved",
			stored.Secret(sqsdriver.SecretAccessKeyID),
			stored.Secret(sqsdriver.SecretSecretAccessKey))
	}
	/*
	 * The reserved names, which are the trap this family walks past. accessKey
	 * and secretKey are RocketMQ's ACL pair: they skip applyCredentials'
	 * generic loop, are written only through SetACL, and are filled from
	 * global settings for a profile that named no mechanism. A family reusing
	 * them would have its own credentials cleared on save and RocketMQ's
	 * global pair stamped on at dial time.
	 */
	if stored.Secret(model.SecretAccessKey) != "" || stored.Secret(model.SecretSecretKey) != "" {
		t.Error("the reserved RocketMQ ACL names were written onto an SQS profile")
	}
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain", stored.Auth.Mechanism)
	}

	// And it dials, which is the half a stored-value check cannot cover.
	if err := stack.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect a profile read back from disk: %v", err)
	}
	t.Cleanup(func() { _ = stack.connections.Disconnect(created.ID) })

	destinations, err := stack.destinations.List(sqsContext(t), created.ID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	if len(destinations) == 0 {
		e2e.Missing(t, "the region holds no MQS-SEED- queues; run `npm run e2e:sqs:seed`")
	}
	// The prefix survived too, which is only visible in what the listing did
	// not return.
	for _, entry := range destinations {
		if !strings.HasPrefix(entry.Ref.Name, "MQS-SEED-") {
			t.Errorf("%s is outside the stored prefix, which did not survive disk", entry.Ref.Name)
		}
	}
}

/*
 * The capability set a real connection reports, which is what the sidebar
 * draws from.
 *
 * Asserted here rather than only in the driver's package because this is the
 * object a page actually holds: the driver's test builds a Conn by hand, and
 * this one comes back from a dial through the registry.
 */
func TestLiveSQSCapabilitiesReachThePages(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)
	connID := stack.dial(t, liveSQSProfile("sqs capabilities"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	capabilities := conn.Capabilities()

	for _, wanted := range []model.Capability{
		model.CapDestinationList,
		model.CapDestinationCreate,
		model.CapDestinationUpdate,
		model.CapDestinationDelete,
		model.CapDestinationPurge,
		model.CapMessageQuery,
		model.CapPublish,
		model.CapDelayedDelivery,
		model.CapDeadLetterTopology,
	} {
		if !capabilities.Has(wanted) {
			t.Errorf("a live connection does not report %s", wanted)
		}
	}

	// The caveat has to reach the page, because browsing is the one thing on
	// this family with a consequence: it is the same call a consumer makes.
	caveat, warned := capabilities.Caveat(model.CapMessageQuery)
	if !warned {
		t.Error("browsing is offered with no caveat on a live connection")
	}
	if !strings.HasPrefix(caveat, "mq.sqs.") {
		t.Errorf("caveat = %q, which is not an i18n key the sidebar can resolve", caveat)
	}

	// And nothing that would draw a page onto an empty set forever.
	for _, absent := range []model.Capability{
		model.CapSubscriptionList,
		model.CapClusterTopology,
		model.CapClusterMetrics,
		model.CapClientInspect,
		model.CapAccessControl,
		model.CapMessageByID,
	} {
		if capabilities.Has(absent) {
			t.Errorf("a live connection reports %s, which SQS has no concept of", absent)
		}
	}
}

/*
 * Asking for something the family has no call for fails with a sentence about
 * SQS rather than about a Go type.
 *
 * MessageService.ByID type-asserts MessageReader without consulting the
 * capability, which SQS satisfies for the browse - the same shape RabbitMQ is
 * in. So the message a page gets back is the driver's own, and it has to
 * explain the service rather than report a nil.
 */
func TestLiveSQSMessageByIDExplainsItself(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)
	connID := stack.dial(t, liveSQSProfile("sqs unsupported"))

	_, err := stack.messages.ByID(sqsContext(t), connID, liveSQSOrders, "any-id")
	if err == nil {
		t.Fatal("looked a message up by id, which SQS has no call for")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error does not say why there is no lookup: %v", err)
	}

	// And the capability the sidebar reads is genuinely absent, which is what
	// keeps the page that would call this off the nav in the first place.
	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	if conn.Capabilities().Has(model.CapMessageByID) {
		t.Error("a live connection reports message-by-id, which SQS has no call for")
	}
}
