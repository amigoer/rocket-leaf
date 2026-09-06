package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	solacedriver "github.com/amigoer/mq-studio/internal/driver/solace"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/cluster"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/routing"
	"github.com/amigoer/mq-studio/internal/service/scope"
	"github.com/amigoer/mq-studio/internal/service/settings"
	solaceservice "github.com/amigoer/mq-studio/internal/service/solace"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The Solace stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * value that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * The disk half matters twice over here. This profile carries two credential
 * pairs - a SEMP management account and a client username for the REST
 * messaging interface, which are objects in different directories - and it
 * carries the Message VPN, which is the scope every path is built from. Losing
 * the second pair produces a connection that reads every board and cannot send
 * a message; losing the VPN produces one that reads an entirely different set
 * of objects and looks perfectly healthy doing it.
 */

const (
	liveSolaceAddress = "http://127.0.0.1:8080"
	liveSolaceVPN     = "default"
	liveSolaceAdmin   = "admin"
	liveSolaceAdminPw = "admin"
)

// The objects scripts/e2e-solace-seed.sh creates.
const (
	liveSolaceOrders    = "mqstudio/seed/orders"
	liveSolaceAudit     = "mqstudio/seed/audit"
	liveSolaceDMQ       = "mqstudio/seed/dmq"
	liveSolaceEvents    = "mqstudio/seed/events"
	liveSolaceEndpoint  = "mqstudio/seed/endpoint"
	liveSolaceTopicSub  = "mqstudio/seed/events/>"
	liveSolaceSecondVPN = "mqstudio-seed"
	liveSolaceOther     = "mqstudio/seed/other"
	// The name every endpoint's deadMsgQueue starts at, which no broker ever
	// creates a queue for.
	liveSolaceMissingDMQ = "#DEAD_MSG_QUEUE"
)

func requireLiveSolace(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.Solace,
		Name:   "the solace e2e environment",
		Start:  "npm run e2e:solace:up",
		Probe:  probeLiveSolace,
	})
}

/*
 * probeLiveSolace asks SEMP whether the default Message VPN is up.
 *
 * Not e2e.HTTPGet and not e2e.DialTCP. SEMP binds 8080 and answers about the
 * broker before any Message VPN is serving, so both shared probes would report
 * a broker that cannot answer a single board as present - and SEMP answers a
 * collection under a VPN that does not exist with an empty list and no error,
 * so even a probe that asked for the queues would go green.
 */
func probeLiveSolace() error {
	request, err := http.NewRequest(http.MethodGet,
		liveSolaceAddress+"/SEMP/v2/monitor/msgVpns/"+liveSolaceVPN+"?select=state", nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(liveSolaceAdmin, liveSolaceAdminPw)

	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var answer struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return fmt.Errorf("semp answered something unexpected: %s", string(body))
	}
	if answer.Data.State != "up" {
		return fmt.Errorf("message vpn %s is %q", liveSolaceVPN, answer.Data.State)
	}
	return nil
}

// solaceStack is the connection service, the Solace service and the canonical
// services a board reads through, on a config directory of its own.
type solaceStack struct {
	connections  *connection.Service
	solace       *solaceservice.Service
	destinations *destination.Service
	messages     *message.Service
	routing      *routing.Service
	cluster      *cluster.Service
	scopes       *scope.Service
	// conns is what the bridge holds to answer capability questions without
	// going through a domain service, which is how the sidebar decides what to
	// draw and why.
	conns func(connID int) (driver.Conn, error)
	// dataFile is where the profiles land, so a second service can be opened
	// on the same store and prove what actually survived disk.
	dataFile string
	settings *settings.Service
}

func newSolaceStack(t *testing.T) *solaceStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindSolace); !ok {
		driver.Register(solacedriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &solaceStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		solace:       solaceservice.New(conns, settingsService),
		destinations: destination.New(conns, settingsService),
		messages:     message.New(conns, settingsService),
		routing:      routing.New(conns, settingsService),
		cluster:      cluster.New(paths.TPSHistoryFile, conns, settingsService),
		scopes:       scope.New(conns, settingsService),
		conns:        conns,
		dataFile:     paths.ConnectionsFile,
		settings:     settingsService,
	}
}

/*
 * liveSolaceProfile is the environment as a user would configure it.
 *
 * The REST pair is left empty on purpose rather than filled with the SEMP one:
 * the seed leaves the default Message VPN's basic authentication type at
 * "none", which is how a broker ships, and an empty pair is what tells the
 * driver to send no credential at all. Filling it with "admin" would be
 * offering a management account as a client username, which is what the driver
 * refuses to do on its own.
 */
func liveSolaceProfile(name string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindSolace,
		Endpoints:  liveSolaceAddress,
		TimeoutSec: 30,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			solacedriver.OptionMsgVPN: liveSolaceVPN,
		},
	}
	profile.SetSecret(solacedriver.SecretUsername, liveSolaceAdmin)
	profile.SetSecret(solacedriver.SecretPassword, liveSolaceAdminPw)
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *solaceStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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

func solaceContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func solaceTestName(t *testing.T, suffix string) string {
	t.Helper()
	return "mqstudio/test/app/" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

/*
 * The whole path in one go: store a profile, dial it, declare a queue, send to
 * it, and read it back through the id a page would pass.
 *
 * Two interfaces are crossed here and only one of them is SEMP. Everything
 * except the send goes through the management port; the send goes through a
 * port the driver worked out from the Message VPN's own configuration, so a
 * profile that lost its scope or a driver that guessed the port fails here and
 * nowhere else.
 */
func TestLiveSolaceStackRoundTrip(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace stack round trip"))
	ctx := solaceContext(t)

	queue := solaceTestName(t, "/round")
	if err := stack.solace.CreateDestination(ctx, connID, model.DestinationSpec{
		Ref: model.DestinationRef{Name: queue},
		Attributes: map[string]string{
			solacedriver.AttrAccessType: "non-exclusive",
			solacedriver.AttrPermission: "consume",
		},
	}); err != nil {
		t.Fatalf("create %s: %v", queue, err)
	}
	t.Cleanup(func() {
		_ = stack.solace.RemoveDestination(context.Background(), connID, queue)
	})

	result, err := stack.solace.Publish(ctx, connID, solacedriver.PublishRequest{
		Target:      solacedriver.TargetQueue,
		Destination: queue,
		Body:        "round trip",
		Count:       2,
	})
	if err != nil {
		t.Fatalf("publish to %s: %v", queue, err)
	}
	if result.Sent != 2 {
		t.Errorf("sent %d of 2", result.Sent)
	}

	// The depth follows the send by a moment, so it is waited for with a
	// bounded budget rather than read straight away.
	made := waitForListedDepth(t, stack, connID, queue, 2)
	if made.Ref.Namespace != liveSolaceVPN {
		t.Errorf("%s is namespaced %q, want %q", queue, made.Ref.Namespace, liveSolaceVPN)
	}

	browsed, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{Topic: queue})
	if err != nil {
		t.Fatalf("browse %s: %v", queue, err)
	}
	if len(browsed) != 2 {
		t.Fatalf("browsed %d of the two messages sent", len(browsed))
	}
	// The body is empty on every Solace message: SEMP carries no payload at
	// any version, which is what the caveat on CapMessageQuery says.
	if browsed[0].Body != "" {
		t.Errorf("a browsed message came back with a body of %d bytes; semp has no payload "+
			"field and the caveat says so", len(browsed[0].Body))
	}
	if browsed[0].MessageID == "" {
		t.Error("a browsed message has no id, so nothing could be opened from the list")
	}

	// The delete takes the messages with it, without a word from the broker.
	// There is no guarded form and none is offered: SEMP has no precondition
	// to ask for.
	if err := stack.solace.RemoveDestination(ctx, connID, queue); err != nil {
		t.Errorf("delete a queue holding messages: %v", err)
	}
}

// waitForListedDepth reads the queue back through the canonical listing until
// it reports the depth expected, with a bounded budget.
//
// Through the listing rather than a detail read, because the listing is what a
// board shows and it is the path with the extra per-queue counts in it.
func waitForListedDepth(
	t *testing.T, stack *solaceStack, connID int, queue string, want int64,
) *model.Destination {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last *model.Destination
	for time.Now().Before(deadline) {
		listed, err := stack.destinations.List(
			solaceContext(t), connID, model.DestinationFilter{})
		if err != nil {
			t.Fatalf("list destinations: %v", err)
		}
		last = nil
		for _, entry := range listed {
			if entry.Ref.Name == queue {
				last = entry
			}
		}
		if last != nil && last.Depth >= want {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
	if last == nil {
		t.Fatalf("%s never appeared in the listing", queue)
	}
	t.Fatalf("%s reports depth %d, want %d", queue, last.Depth, want)
	return nil
}

/*
 * The scope, through the service the shell's switcher calls.
 *
 * Two connections to one broker on two Message VPNs, listing through the
 * canonical destination service: the seed puts different queues in each, so a
 * scope that was not reaching the driver would return one list twice. This is
 * the half the driver's own tests cannot cover - the option has been through
 * disk and back by the time it is read.
 */
func TestLiveSolaceScopeReachesTheDriverThroughAProfile(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	ctx := solaceContext(t)

	first := stack.dial(t, liveSolaceProfile("solace scope default"))
	second := liveSolaceProfile("solace scope second")
	second.Options[solacedriver.OptionMsgVPN] = liveSolaceSecondVPN
	otherID := stack.dial(t, second)

	names := func(connID int) map[string]bool {
		listed, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
		if err != nil {
			t.Fatalf("list destinations on %d: %v", connID, err)
		}
		found := map[string]bool{}
		for _, entry := range listed {
			found[entry.Ref.Name] = true
		}
		return found
	}

	one, other := names(first), names(otherID)
	if !one[liveSolaceOrders] {
		e2e.Missing(t, "%s is not in %s; run: npm run e2e:solace:seed",
			liveSolaceOrders, liveSolaceVPN)
	}
	if !other[liveSolaceOther] {
		e2e.Missing(t, "%s is not in %s; run: npm run e2e:solace:seed",
			liveSolaceOther, liveSolaceSecondVPN)
	}
	if other[liveSolaceOrders] || one[liveSolaceOther] {
		t.Error("the two connections read the same objects, so the message vpn is not " +
			"reaching the driver through the profile")
	}

	// And the switcher's own listing, which is what the popover draws.
	scopes, err := stack.scopes.List(ctx, first)
	if err != nil {
		t.Fatalf("list scopes: %v", err)
	}
	offered := map[string]bool{}
	for _, entry := range scopes {
		offered[entry.Name] = true
	}
	for _, name := range []string{liveSolaceVPN, liveSolaceSecondVPN} {
		if !offered[name] {
			t.Errorf("the switcher does not offer %s", name)
		}
	}
}

/*
 * The capability set a real connection reports, which is what the sidebar
 * draws from.
 *
 * Asserted through the connection source the bridge holds rather than through
 * the driver, because that is the path the renderer takes: a capability the
 * driver declares and the registry loses is a page that disappears with
 * nothing going red.
 */
func TestLiveSolaceReportsItsCapabilities(t *testing.T) {
	requireLiveSolace(t)
	stack := newSolaceStack(t)
	connID := stack.dial(t, liveSolaceProfile("solace capabilities"))

	conn, err := stack.conns(connID)
	if err != nil {
		t.Fatalf("resolve the connection: %v", err)
	}
	capabilities := conn.Capabilities()

	for _, capability := range []model.Capability{
		model.CapDestinationList,
		model.CapDestinationCreate,
		model.CapDestinationDelete,
		model.CapConnectionScope,
		model.CapMessageQuery,
		model.CapMessageByID,
		model.CapPublish,
		model.CapDeadLetterTopology,
		model.CapRouting,
		model.CapRoutingAdmin,
		model.CapClusterTopology,
		model.CapClusterMetrics,
		model.CapClientInspect,
	} {
		if !capabilities.Has(capability) {
			t.Errorf("a live connection does not report %s", capability)
		}
	}

	// The caveat, which is the difference between a page that works with a
	// consequence and one that cannot be opened. It has to be this family's
	// own rather than another's: a SEMP browse takes nothing, and what it
	// cannot do is return the message.
	caveat, present := capabilities.Caveat(model.CapMessageQuery)
	if !present {
		t.Error("browsing carries no caveat, and it returns no message body")
	}
	if caveat != "mq.solace.caveat.browseNoPayload" {
		t.Errorf("browse caveat = %q, want mq.solace.caveat.browseNoPayload", caveat)
	}

	// And no consumer group anywhere: this product has none, so declaring the
	// capability would draw a page with nothing to list.
	if capabilities.Has(model.CapSubscriptionList) {
		t.Error("a solace connection reports consumer groups, and the product has none")
	}
}
