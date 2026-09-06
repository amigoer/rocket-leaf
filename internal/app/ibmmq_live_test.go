package app

import (
	"context"
	"crypto/tls"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	ibmmqdriver "github.com/amigoer/mq-studio/internal/driver/ibmmq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	ibmmqservice "github.com/amigoer/mq-studio/internal/service/ibmmq"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The IBM MQ stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * The disk half matters more here than it did for the last few families. This
 * profile carries two credential pairs, and losing the second produces a
 * connection that opens, reads every board and cannot browse one message -
 * which is indistinguishable from a working connection with the wrong mqweb
 * role until somebody clicks the messages page.
 */

const (
	liveIBMMQAddress  = "https://127.0.0.1:9443"
	liveIBMMQManager  = "QM1"
	liveIBMMQAdmin    = "admin"
	liveIBMMQAdminPw  = "passw0rd"
	liveIBMMQApp      = "app"
	liveIBMMQAppPw    = "passw0rd"
	liveIBMMQDeadLetr = "DEV.DEAD.LETTER.QUEUE"
)

// The objects scripts/e2e-ibmmq-seed.sh creates.
const (
	liveIBMMQOrders  = "MQS.SEED.ORDERS"
	liveIBMMQAudit   = "MQS.SEED.AUDIT"
	liveIBMMQBackout = "MQS.SEED.BACKOUT"
	liveIBMMQSubQ    = "MQS.SEED.SUBQ"
	liveIBMMQXmitQ   = "MQS.SEED.XMITQ"
	liveIBMMQTopic   = "MQS.SEED.EVENTS"
	liveIBMMQSub     = "MQS.SEED.SUB"
	liveIBMMQChannel = "MQS.SEED.SDR"
)

func requireLiveIBMMQ(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.IBMMQ,
		Name:   "the ibm mq e2e environment",
		Start:  "npm run e2e:ibmmq:up",
		Probe:  probeLiveIBMMQ,
	})
}

/*
 * probeLiveIBMMQ asks the REST API whether the queue manager is running.
 *
 * Not e2e.HTTPGet: the shared probe verifies certificates and mqweb presents
 * one it signed itself, so it would report every healthy environment as
 * absent. Not e2e.DialTCP either, which would go the other way - Liberty binds
 * 9443 and serves the console while the queue manager is still starting.
 */
func probeLiveIBMMQ() error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a self-signed development certificate
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

	request, err := http.NewRequest(
		http.MethodGet, liveIBMMQAddress+"/ibmmq/rest/v1/admin/qmgr/"+liveIBMMQManager, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(liveIBMMQAdmin, liveIBMMQAdminPw)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return &httpStatus{code: response.StatusCode}
	}
	return nil
}

type httpStatus struct{ code int }

func (s *httpStatus) Error() string {
	return "the mqweb server answered " + strconv.Itoa(s.code)
}

// ibmmqStack is the connection service, the IBM MQ service and the canonical
// services a board reads through, on a config directory of its own.
type ibmmqStack struct {
	connections   *connection.Service
	ibmmq         *ibmmqservice.Service
	destinations  *destination.Service
	subscriptions *subscription.Service
	messages      *message.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
	// dataFile is where the profiles land, so a second service can be opened
	// on the same store and prove what actually survived disk.
	dataFile string
	settings *settings.Service
}

func newIBMMQStack(t *testing.T) *ibmmqStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindIBMMQ); !ok {
		driver.Register(ibmmqdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &ibmmqStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		ibmmq:         ibmmqservice.New(conns, settingsService),
		destinations:  destination.New(conns, settingsService),
		subscriptions: subscription.New(conns, settingsService),
		messages:      message.New(conns, settingsService),
		conns:         conns,
		dataFile:      paths.ConnectionsFile,
		settings:      settingsService,
	}
}

/*
 * liveIBMMQProfile is the environment as a user would configure it.
 *
 * Two credential pairs, because the developer image ships one account per
 * mqweb role: admin holds MQWebAdmin and app holds MQWebUser, and neither can
 * do the other's work. Skip-verify is on and lives in the profile, which is
 * where the switch belongs - the driver must never turn verification off for
 * anybody who did not ask.
 */
func liveIBMMQProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindIBMMQ,
		Endpoints:  liveIBMMQAddress,
		TimeoutSec: 30,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			ibmmqdriver.OptionQueueManager:  liveIBMMQManager,
			ibmmqdriver.OptionTLSSkipVerify: "true",
		},
	}
	profile.SetSecret(ibmmqdriver.SecretUsername, liveIBMMQAdmin)
	profile.SetSecret(ibmmqdriver.SecretPassword, liveIBMMQAdminPw)
	profile.SetSecret(ibmmqdriver.SecretMessagingUsername, liveIBMMQApp)
	profile.SetSecret(ibmmqdriver.SecretMessagingPassword, liveIBMMQAppPw)
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *ibmmqStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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

func ibmmqContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func ibmmqTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "MQS.TEST.APP." + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile with two credential pairs, dial
 * it, declare a queue, send to it, and read it back through the id a page
 * would pass.
 *
 * The two pairs are the point. Everything before the send goes through the
 * administrative interface and everything after it through the messaging one,
 * so a profile that lost half its credentials on the way to disk fails here
 * and nowhere else.
 */
func TestLiveIBMMQStackRoundTrip(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq stack round trip"))
	ctx := ibmmqContext(t)

	queue := ibmmqTestName(t, ".ROUND")
	if err := stack.ibmmq.CreateDestination(ctx, connID, model.DestinationSpec{
		Ref: model.DestinationRef{Name: queue},
		Attributes: map[string]string{
			ibmmqdriver.AttrKind:      ibmmqdriver.KindQueue,
			ibmmqdriver.AttrQueueType: "local",
			ibmmqdriver.AttrMaxDepth:  "500",
		},
	}); err != nil {
		t.Fatalf("create %s: %v", queue, err)
	}
	t.Cleanup(func() {
		_ = stack.ibmmq.RemoveDestination(context.Background(), connID, queue, true)
	})

	result, err := stack.ibmmq.Publish(ctx, connID, ibmmqdriver.PublishRequest{
		Queue: queue,
		Body:  "round trip",
		Count: 2,
	})
	if err != nil {
		t.Fatalf("publish to %s: %v", queue, err)
	}
	if result.Sent != 2 {
		t.Errorf("sent %d of 2", result.Sent)
	}

	listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	var made *model.Destination
	for _, entry := range listed {
		if entry.Ref.Name == queue {
			made = entry
		}
	}
	if made == nil {
		t.Fatalf("%s was created and does not appear in the listing", queue)
	}
	if made.Depth != 2 {
		t.Errorf("%s reports depth %d after two sends", queue, made.Depth)
	}

	browsed, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{Topic: queue})
	if err != nil {
		t.Fatalf("browse %s: %v", queue, err)
	}
	if len(browsed) != 2 {
		t.Fatalf("browsed %d of the two messages sent", len(browsed))
	}
	if browsed[0].Body != "round trip" {
		t.Errorf("the body came back as %q", browsed[0].Body)
	}

	// The delete is refused while the queue holds them, which is the queue
	// manager's own check and the one this service keeps as its default.
	if err := stack.ibmmq.RemoveDestination(ctx, connID, queue, false); err == nil {
		t.Error("deleted a queue holding messages without being asked to discard them")
	}
	if err := stack.ibmmq.RemoveDestination(ctx, connID, queue, true); err != nil {
		t.Errorf("purging delete: %v", err)
	}
}

/*
 * The capability check runs before the type assertion, and the message says so.
 *
 * This driver declares a capability the shared vocabulary did not have -
 * CapChannels - so the service checks it first and a page that reached the
 * channels service on another family's connection gets told which capability
 * is missing rather than a Go type nobody has heard of.
 */
func TestLiveIBMMQServiceNamesTheCapabilityRatherThanTheType(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)
	connID := stack.dial(t, liveIBMMQProfile("ibm mq capability check"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	declared := conn.Capabilities()
	if !declared.Has(model.CapChannels) {
		t.Fatal("the connection does not declare CapChannels, and the channels page gates on it")
	}

	channels, err := stack.ibmmq.Channels(ibmmqContext(t), connID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) == 0 {
		t.Error("no channels at all; a fresh queue manager defines a dozen")
	}

	// An id nothing is open on is refused by the service rather than panicking
	// on a nil connection, which is what a page holds after a disconnect.
	if _, err := stack.ibmmq.Channels(ibmmqContext(t), connID+999); err == nil {
		t.Error("the channels service answered for a connection that is not open")
	}
}

/*
 * The profile survives disk with both credential pairs, and the connection
 * opened from the reloaded copy works.
 *
 * The offline test in ibmmq_credentials_test.go proves what is stored; this
 * proves the stored thing dials. They are different failures: a value can
 * round-trip through the file and still be handed to the driver under the
 * wrong name.
 */
func TestLiveIBMMQProfileSurvivesAReopen(t *testing.T) {
	requireLiveIBMMQ(t)
	stack := newIBMMQStack(t)

	created, err := stack.connections.AddConnection(liveIBMMQProfile("ibm mq reopened"))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)
	reopened := connection.New(
		stack.dataFile, stack.settings, newRegistryRuntime(registry), newDescriptorEndpoints())

	if err := reopened.Connect(created.ID); err != nil {
		t.Fatalf("connecting from a second service on the same file: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Disconnect(created.ID) })

	conns := newConnSource(registry)
	service := ibmmqservice.New(conns, stack.settings)

	qmgr, err := service.QueueManager(created.ID)
	if err != nil {
		t.Fatalf("QueueManager: %v", err)
	}
	if qmgr != liveIBMMQManager {
		t.Errorf("the reopened connection speaks to %q, want %q", qmgr, liveIBMMQManager)
	}

	// The messaging half, which is the credential a reopen would lose
	// silently: everything above this line goes through the other interface.
	messages := message.New(conns, stack.settings)
	if _, err := messages.Query(ibmmqContext(t), created.ID, model.MessageQueryParams{
		Topic:      liveIBMMQOrders,
		MaxResults: 1,
	}); err != nil {
		t.Errorf("browsing through a reopened connection: %v", err)
	}
}
