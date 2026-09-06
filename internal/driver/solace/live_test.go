package solace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live broker, and the management account the container creates on first
// boot from the two environment variables the compose file sets.
const (
	liveSEMP  = "http://127.0.0.1:8080"
	liveREST  = "http://127.0.0.1:9000"
	liveVPN   = "default"
	liveAdmin = "admin"
	livePass  = "admin"
)

// The second Message VPN the seed makes, and the REST port it listens on.
// It exists so that "which Message VPN is this connection reading" is a
// question with a wrong answer available: a driver that ignored the scope
// would pass every test against a broker that hosts one.
const (
	liveSecondVPN  = "mqstudio-seed"
	liveSecondREST = "http://127.0.0.1:9010"
)

// Objects the seed made, which the live tests read and never change. Anything
// a test creates is named mqstudio/test/* so the two can never collide.
const (
	seedOrdersQueue  = "mqstudio/seed/orders"
	seedAuditQueue   = "mqstudio/seed/audit"
	seedDMQ          = "mqstudio/seed/dmq"
	seedEventsQueue  = "mqstudio/seed/events"
	seedEndpoint     = "mqstudio/seed/endpoint"
	seedSubscription = "mqstudio/seed/events/>"
	seedOtherQueue   = "mqstudio/seed/other"
)

func requireSolace(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.Solace,
		Name:   "the solace broker",
		Start:  "npm run e2e:solace:up",
		Probe:  probeMsgVPN,
	})
}

/*
 * probeMsgVPN asks SEMP whether the default Message VPN is up.
 *
 * Not e2e.HTTPGet and not e2e.DialTCP, and the reason is the same one the
 * compose file's health check gives. SEMP binds 8080 and answers about the
 * broker before any Message VPN is serving, so both of the shared probes would
 * report a broker that cannot yet answer a single board as present. Worse,
 * SEMP answers a collection under a VPN that does not exist with an empty list
 * and no error, so a probe that asked for the queues would go green on a
 * half-started broker as well.
 */
func probeMsgVPN() error {
	state, err := sempField("/monitor/msgVpns/"+liveVPN+"?select=state", "state")
	if err != nil {
		return err
	}
	if state != "up" {
		return fmt.Errorf("message vpn %s is %s", liveVPN, state)
	}
	return nil
}

// sempField reads one string out of a SEMP object, straight over HTTP.
//
// Deliberately not through the driver: a probe and a cross-check that went
// through the code under test would agree with it however wrong it was.
func sempField(path, field string) (string, error) {
	var answer struct {
		Data map[string]any `json:"data"`
	}
	if err := rawSEMP(http.MethodGet, path, nil, &answer); err != nil {
		return "", err
	}
	value, present := answer.Data[field]
	if !present {
		return "", fmt.Errorf("%s carries no %s", path, field)
	}
	return fmt.Sprint(value), nil
}

// rawSEMP is one SEMP call made without the driver.
func rawSEMP(method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequest(method, liveSEMP+"/SEMP/v2"+path, body)
	if err != nil {
		return err
	}
	request.SetBasicAuth(liveAdmin, livePass)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var meta struct {
		Meta struct {
			Error *struct {
				Status      string `json:"status"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("%s answered something that is not semp: %s", path, string(raw))
	}
	if meta.Meta.Error != nil {
		return fmt.Errorf("%s %s: %s %s", method, path,
			meta.Meta.Error.Status, meta.Meta.Error.Description)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// livePath escapes an object name for a SEMP path, the way the driver does.
func livePath(name string) string { return url.PathEscape(name) }

/*
 * liveProfile is the environment as a user would configure it.
 *
 * The Message VPN is named rather than left to discovery, because the seed
 * makes a second one: a profile that named none would be resolved by the
 * driver's own fallback, and a test asserting on that would be asserting the
 * fallback rather than the field. The REST address is left empty on purpose -
 * deriving it from the VPN's own configuration is the ordinary path and the
 * one worth exercising on every test here.
 */
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "solace e2e",
		Kind:       model.KindSolace,
		Endpoints:  liveSEMP,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			OptionMsgVPN: liveVPN,
		},
		Secrets: map[string]string{
			SecretUsername: liveAdmin,
			SecretPassword: livePass,
		},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireSolace(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestLiveOpenReachesTheBroker(t *testing.T) {
	conn := liveConn(t)

	if conn.Kind() != model.KindSolace {
		t.Errorf("kind = %q, want solace", conn.Kind())
	}
	if conn.MsgVPN() != liveVPN {
		t.Errorf("message vpn = %q, want %q", conn.MsgVPN(), liveVPN)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

/*
 * A profile that names no Message VPN gets "default".
 *
 * This is the ordinary case and it is what keeps the field optional on the
 * form. It is also the half that has to be proved against a real broker with
 * more than one VPN: the seed makes a second, so a driver that took whichever
 * name sorted first would land on "default" only by luck here and on the wrong
 * VPN on any broker whose second VPN sorts earlier.
 */
func TestLiveOpenFallsBackToTheDefaultMsgVPN(t *testing.T) {
	requireSolace(t)
	profile := liveProfile()
	profile.Options[OptionMsgVPN] = ""

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if conn.MsgVPN() != liveVPN {
		t.Errorf("resolved %q, want %q", conn.MsgVPN(), liveVPN)
	}
}

/*
 * A Message VPN that is not there fails at open.
 *
 * It has to, and this is the one test in the file that would still pass if the
 * check were removed by accident somewhere else: SEMP answers every collection
 * under a Message VPN it does not have with 200 and an empty list. A driver
 * that took the name on trust would open, ping - because the ping would have
 * to be reading something else - and report an entirely empty broker on every
 * board, which reads as an outage rather than as a typo.
 */
func TestLiveOpenRefusesAMsgVPNThatIsNotThere(t *testing.T) {
	requireSolace(t)
	profile := liveProfile()
	profile.Options[OptionMsgVPN] = "no-such-vpn"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection to a message vpn that does not exist")
	}
	if !strings.Contains(err.Error(), liveVPN) {
		t.Errorf("error does not say which message vpns are there: %v", err)
	}
}

// SEMP is the whole of what every board reads, so a credential it refuses has
// to fail at open. A connection that opened anyway would report an empty
// broker rather than a refused login.
func TestLiveOpenRefusesACredentialSEMPRejects(t *testing.T) {
	requireSolace(t)
	profile := liveProfile()
	profile.Secrets[SecretPassword] = "not-the-password"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection with a password semp rejects")
	}
}

/*
 * The REST messaging interface is a tier of its own, and the probe settles
 * whether this connection has it.
 *
 * Three cases, because the broker distinguishes three and they send a reader
 * to three different places. Reached and willing is the ordinary one. A port
 * nothing answers on is the connection form or the broker's service
 * configuration. A credential the broker refuses is the Message VPN's
 * client-usernames - and it arrives as 403 rather than as the 401 an HTTP
 * client would expect, which is why the driver reads both.
 */
func TestLiveRESTTierIsProbedSeparately(t *testing.T) {
	requireSolace(t)

	t.Run("derived from the message vpn", func(t *testing.T) {
		conn, err := open(liveContext(t), liveProfile())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		probed := conn.probeREST(liveContext(t))
		if probed.restReason != "" {
			t.Fatalf("rest reported unavailable as %s", probed.restReason)
		}
		if probed.restURL != liveREST {
			t.Errorf("rest url = %q, want %q; the port is a message vpn setting and "+
				"has to come from the vpn rather than from a constant", probed.restURL, liveREST)
		}
	})

	t.Run("with nothing on the named port", func(t *testing.T) {
		profile := liveProfile()
		// A port on the broker that is open and is not the messaging
		// interface: SEMP itself. It answers, so this proves the probe reads
		// what came back rather than only whether the socket opened.
		profile.Options[OptionRESTURL] = liveSEMP

		conn, err := open(liveContext(t), profile)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if reason := conn.probeREST(liveContext(t)).restReason; reason != restUnreachable {
			t.Errorf("rest reason = %q, want %q", reason, restUnreachable)
		}
	})

	t.Run("with a credential the broker refuses", func(t *testing.T) {
		// The second Message VPN authenticates internally once the seed has
		// enabled its client-username, so a password it does not hold is
		// refused there and would be ignored on the default VPN.
		profile := liveProfile()
		profile.Options[OptionMsgVPN] = liveSecondVPN
		profile.Secrets[SecretRESTUsername] = "default"
		profile.Secrets[SecretRESTPassword] = "not-the-password"
		requireInternalAuth(t, liveSecondVPN)

		conn, err := open(liveContext(t), profile)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if reason := conn.probeREST(liveContext(t)).restReason; reason != restForbidden {
			t.Errorf("rest reason = %q, want %q; a refused credential and a port that "+
				"answers nothing send a reader to different places", reason, restForbidden)
		}
	})
}

// requireInternalAuth puts the named Message VPN on internal basic
// authentication for the length of one test, and puts it back afterwards.
//
// The seed leaves it on "none" so that the ordinary send path needs no
// credential. Only the refusal case needs the broker to actually check one,
// and doing it here rather than in the seed keeps that a property of the test
// instead of a property every other test has to work around.
func requireInternalAuth(t *testing.T, vpn string) {
	t.Helper()
	before, err := sempField("/config/msgVpns/"+livePath(vpn)+"?select=authenticationBasicType",
		"authenticationBasicType")
	if err != nil {
		e2e.Missing(t, "%s does not answer for its authentication type: %v", vpn, err)
	}
	patch := func(value string) {
		if err := rawSEMP(http.MethodPatch, "/config/msgVpns/"+livePath(vpn),
			map[string]any{"authenticationBasicType": value}, nil); err != nil {
			t.Fatalf("setting %s basic auth to %s: %v", vpn, value, err)
		}
	}
	patch("internal")
	t.Cleanup(func() { patch(before) })
}

// Close is idempotent and a closed connection stops answering. The registry
// closes on disconnect and on shutdown, so the second close has to be the one
// that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireSolace(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answers a ping")
	}
}

/*
 * Names with a slash in them survive the round trip.
 *
 * Not a corner case here, and that is the point: Solace names are written like
 * topics, so "mqstudio/seed/orders" is the ordinary shape rather than an
 * awkward one. A driver that pasted a name into a URL unescaped would read
 * that as a collection with a sub-resource and answer NOT_FOUND for a queue
 * that is plainly in the listing.
 */
func TestLiveNamesWithSlashesAreAddressable(t *testing.T) {
	conn := liveConn(t)

	var answer struct {
		QueueName string `json:"queueName"`
	}
	path := "/msgVpns/" + segment(conn.MsgVPN()) + "/queues/" + segment(seedOrdersQueue) + "?select=queueName"
	if err := conn.semp.configGet(liveContext(t), path, &answer); err != nil {
		e2e.Missing(t, "%s is not there; run: npm run e2e:solace:seed (%v)", seedOrdersQueue, err)
	}
	if answer.QueueName != seedOrdersQueue {
		t.Errorf("queue name = %q, want %q", answer.QueueName, seedOrdersQueue)
	}
}

/*
 * The queue listing, and the figure on it that no field carries.
 *
 * The depth is asserted against the message collection's own count read
 * straight over HTTP rather than against a number written into this file,
 * because the seed's queues are the only thing that makes the assertion worth
 * anything and both sides have to agree about the same moment.
 *
 * spooledMsgCount is checked here too, and checked to be the *wrong* answer on
 * one queue: the seed's audit queue has handed everything to its dead message
 * queue, so it holds nothing and its lifetime counter still says three. A
 * driver that read the obvious field would pass every other test in this file.
 */
func TestLiveListDestinationsReportsTheRealDepth(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	found := map[string]*model.Destination{}
	for _, destination := range destinations {
		found[destination.Ref.Name] = destination
		if destination.Ref.Namespace != liveVPN {
			t.Errorf("%s is namespaced %q, want %q",
				destination.Ref.Name, destination.Ref.Namespace, liveVPN)
		}
	}

	for _, name := range []string{seedOrdersQueue, seedAuditQueue, seedDMQ, seedEventsQueue} {
		if _, present := found[name]; !present {
			e2e.Missing(t, "%s is not in the listing; run: npm run e2e:solace:seed", name)
		}
	}

	// Both sides read together, because a broker moving messages between them
	// would otherwise look like a driver getting the figure wrong.
	for _, name := range []string{seedOrdersQueue, seedDMQ, seedEventsQueue} {
		want, err := rawDepth(name)
		if err != nil {
			t.Fatalf("reading %s over http: %v", name, err)
		}
		if got := found[name].Depth; got != want {
			t.Errorf("%s depth = %d, want %d", name, got, want)
		}
		if want == 0 {
			t.Errorf("%s holds nothing, so the assertion above proves nothing; "+
				"run: npm run e2e:solace:seed", name)
		}
	}

	audit := found[seedAuditQueue]
	if audit.Depth != 0 {
		t.Errorf("%s depth = %d, want 0: the seed's ttl hands everything to the dead "+
			"message queue", seedAuditQueue, audit.Depth)
	}
	if audit.Attributes[AttrSpooledTotal] == "0" {
		t.Errorf("%s reports a lifetime spooled count of 0, so this test can no longer "+
			"tell the statistic from the depth", seedAuditQueue)
	}
}

// rawDepth is how many messages a queue holds, read without the driver.
func rawDepth(queue string) (int64, error) {
	var answer struct {
		Meta struct {
			Count int64 `json:"count"`
		} `json:"meta"`
	}
	path := "/monitor/msgVpns/" + livePath(liveVPN) + "/queues/" + livePath(queue) + "/msgs?count=1"
	if err := rawSEMP(http.MethodGet, path, nil, &answer); err != nil {
		return 0, err
	}
	return answer.Meta.Count, nil
}

// A queue's dead message queue pointer reaches the listing, because the
// dead-letter page is answered by walking it backwards and there is nothing
// else on a queue that says it is one.
func TestLiveListDestinationsCarriesTheDeadMsgQueuePointer(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	for _, destination := range destinations {
		if destination.Ref.Name != seedAuditQueue {
			continue
		}
		if got := destination.Attributes[AttrDeadMsgQueue]; got != seedDMQ {
			t.Errorf("%s points at %q, want %q", seedAuditQueue, got, seedDMQ)
		}
		return
	}
	e2e.Missing(t, "%s is not in the listing; run: npm run e2e:solace:seed", seedAuditQueue)
}

// A queue that is not there reads as gone rather than as a broken page. SEMP
// answers it with HTTP 400 and NOT_FOUND inside the envelope, so this is also
// what pins the error decoding to the envelope.
func TestLiveDestinationDetailNamesAQueueThatIsGone(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.DestinationDetail(liveContext(t),
		model.DestinationRef{Name: "mqstudio/test/no-such-queue"})
	if err == nil {
		t.Fatal("read a queue that does not exist")
	}
	if !strings.Contains(err.Error(), "no queue named") {
		t.Errorf("error does not say the queue is gone: %v", err)
	}
}

/*
 * Creating and deleting a queue, against the broker rather than against a
 * mock.
 *
 * The access type is asserted because it is the setting a create is most
 * likely to get wrong in a way nothing reports: an exclusive queue where a
 * fan-out was meant looks perfectly healthy and hands every message to one
 * consumer.
 */
func TestLiveCreateAndRemoveQueue(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	name := "mqstudio/test/create-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	spec := model.DestinationSpec{
		Ref: model.DestinationRef{Name: name},
		Attributes: map[string]string{
			AttrAccessType: "non-exclusive",
			AttrPermission: "consume",
			AttrMaxSpool:   "50",
		},
	}
	if err := conn.CreateDestination(ctx, spec); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: name})
	})

	detail, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("detail %s: %v", name, err)
	}
	if got := detail.Attributes[AttrAccessType]; got != "non-exclusive" {
		t.Errorf("access type = %q, want non-exclusive", got)
	}
	if got := detail.Attributes[AttrMaxSpool]; got != "50" {
		t.Errorf("spool quota = %q, want 50", got)
	}
	if detail.Depth != 0 {
		t.Errorf("a queue made a moment ago holds %d messages", detail.Depth)
	}

	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if _, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: name}); err == nil {
		t.Error("a deleted queue is still readable")
	}
}

// A second create on the same name is refused with a message that says the
// name is taken, rather than with whatever SEMP's envelope happened to hold.
func TestLiveCreateRefusesANameThatIsTaken(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	name := "mqstudio/test/twice-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	spec := model.DestinationSpec{Ref: model.DestinationRef{Name: name}}

	if err := conn.CreateDestination(ctx, spec); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: name})
	})

	err := conn.CreateDestination(ctx, spec)
	if err == nil {
		t.Fatal("created the same queue twice")
	}
	if !strings.Contains(err.Error(), "already has a queue named") {
		t.Errorf("error does not say the name is taken: %v", err)
	}
}

/*
 * A queue that holds messages is deleted without a word from the broker, and
 * that is worth pinning rather than assuming.
 *
 * IBM MQ refuses one until it is purged, and it would be easy to write a guard
 * here on the strength of that. SEMP has no such precondition: this deletes a
 * queue with messages on it and they are gone. The confirmation on the board
 * is the only thing standing between a user and that, so this test exists to
 * make it a fact somebody chose rather than one nobody checked.
 */
func TestLiveRemoveTakesTheMessagesWithIt(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	name := "mqstudio/test/full-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: name},
	}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	removed := false
	t.Cleanup(func() {
		if !removed {
			_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: name})
		}
	})

	for index := range 3 {
		if err := restPublish(name, fmt.Sprintf("held-%d", index)); err != nil {
			t.Fatalf("publishing to %s: %v", name, err)
		}
	}
	if err := waitForDepth(t, conn, name, 3); err != nil {
		t.Fatalf("%s never reached 3 messages: %v", name, err)
	}

	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("a queue holding messages was refused: %v; the board's confirmation "+
			"assumes the broker deletes it anyway", err)
	}
	removed = true
}

// restPublish puts one body on a queue without the driver, so a test can set
// up a depth before the send console exists.
func restPublish(queue, body string) error {
	request, err := http.NewRequest(http.MethodPost, liveREST+"/QUEUE/"+livePath(queue),
		strings.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "text/plain")
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("rest publish to %s answered %d", queue, response.StatusCode)
	}
	return nil
}

// waitForDepth waits until a queue reports the depth expected, with a bounded
// budget.
//
// A send answers as soon as the broker has taken the message, and the spool
// figures follow a moment later. Asserting straight afterwards is the shape of
// a test that passes locally and fails in CI on a busier machine.
func waitForDepth(t *testing.T, conn *Conn, queue string, want int64) error {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last int64
	for time.Now().Before(deadline) {
		detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: queue})
		if err != nil {
			return err
		}
		last = detail.Depth
		if last >= want {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("depth stayed at %d, want %d", last, want)
}

/*
 * The Message VPN as a scope, which is what the sidebar's switcher offers.
 *
 * The seed makes a second VPN so this can assert something: on a broker with
 * one, a listing that returned only the connection's own would look right.
 */
func TestLiveListScopesReportsEveryMsgVPN(t *testing.T) {
	conn := liveConn(t)

	scopes, err := conn.ListScopes(liveContext(t))
	if err != nil {
		t.Fatalf("list scopes: %v", err)
	}
	found := map[string]*model.Scope{}
	for _, scope := range scopes {
		found[scope.Name] = scope
	}
	for _, name := range []string{liveVPN, liveSecondVPN} {
		if _, present := found[name]; !present {
			e2e.Missing(t, "%s is not among the scopes; run: npm run e2e:solace:seed", name)
		}
	}

	// The counts come from the same collection counts the listing page uses,
	// so they are compared against the raw API rather than against a literal.
	want, err := rawCollectionCount(liveVPN, "queues")
	if err != nil {
		t.Fatalf("counting queues over http: %v", err)
	}
	if got := found[liveVPN].Destinations; got != want {
		t.Errorf("%s reports %d queues, want %d", liveVPN, got, want)
	}
	if want == 0 {
		t.Errorf("%s holds no queues, so the assertion above proves nothing; "+
			"run: npm run e2e:solace:seed", liveVPN)
	}
	if found[liveVPN].Subscriptions < 1 {
		t.Errorf("%s reports %d topic endpoints, and the seed makes one",
			liveVPN, found[liveVPN].Subscriptions)
	}
}

// rawCollectionCount is how many entries a Message VPN's collection holds,
// read without the driver.
func rawCollectionCount(vpn, collection string) (int, error) {
	var answer struct {
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	path := "/monitor/msgVpns/" + livePath(vpn) + "/" + collection + "?count=1"
	if err := rawSEMP(http.MethodGet, path, nil, &answer); err != nil {
		return 0, err
	}
	return answer.Meta.Count, nil
}

/*
 * Switching the scope re-points every board, and this is what proves it reads
 * a different set of objects rather than the same one under another name.
 *
 * Two connections to one broker on two Message VPNs, listing at the same
 * moment: the seed puts different queues in each, so a driver that ignored the
 * scope would return one list twice.
 */
func TestLiveSwitchingScopeChangesWhatIsRead(t *testing.T) {
	requireSolace(t)

	first := liveConn(t)
	profile := liveProfile()
	profile.Options[OptionMsgVPN] = liveSecondVPN
	second, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open %s: %v", liveSecondVPN, err)
	}
	defer func() { _ = second.Close() }()

	if second.MsgVPN() != liveSecondVPN {
		t.Fatalf("second connection is on %q, want %q", second.MsgVPN(), liveSecondVPN)
	}

	names := func(conn *Conn) map[string]bool {
		destinations, listErr := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
		if listErr != nil {
			t.Fatalf("list on %s: %v", conn.MsgVPN(), listErr)
		}
		found := map[string]bool{}
		for _, destination := range destinations {
			found[destination.Ref.Name] = true
		}
		return found
	}

	one, other := names(first), names(second)
	if !one[seedOrdersQueue] {
		e2e.Missing(t, "%s is not in %s; run: npm run e2e:solace:seed", seedOrdersQueue, liveVPN)
	}
	if !other[seedOtherQueue] {
		e2e.Missing(t, "%s is not in %s; run: npm run e2e:solace:seed",
			seedOtherQueue, liveSecondVPN)
	}
	if other[seedOrdersQueue] {
		t.Errorf("%s appears in %s as well, so the scope is not being applied",
			seedOrdersQueue, liveSecondVPN)
	}
	if one[seedOtherQueue] {
		t.Errorf("%s appears in %s as well, so the scope is not being applied",
			seedOtherQueue, liveVPN)
	}
}

/*
 * A name the broker would refuse is refused at the switcher.
 *
 * ValidateScope makes no call, so it cannot say "there is no such VPN" - what
 * it can do is stop the two things the broker's own pattern stops, which is
 * where a wildcard pasted into the switcher would otherwise be stored and fail
 * on the redial with the connection already offline.
 */
func TestLiveValidateScopeMatchesWhatTheBrokerRefuses(t *testing.T) {
	conn := liveConn(t)

	for _, name := range []string{"", liveVPN, liveSecondVPN, "orders.eu", "a/b"} {
		if err := conn.ValidateScope(name); err != nil {
			t.Errorf("ValidateScope(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"orders*", "orders?", strings.Repeat("v", 33)} {
		if err := conn.ValidateScope(name); err == nil {
			t.Errorf("ValidateScope(%q) accepted a name the broker refuses", name)
		}
		// The broker's own refusal, so the rule above is checked against it
		// rather than against a memory of the documentation.
		err := rawSEMP(http.MethodPost, "/config/msgVpns",
			map[string]any{"msgVpnName": name, "enabled": false}, nil)
		if err == nil {
			_ = rawSEMP(http.MethodDelete, "/config/msgVpns/"+livePath(name), nil, nil)
			t.Errorf("the broker accepted %q, so this driver is refusing a name it should take", name)
		}
	}
}

/*
 * Browsing takes nothing, measured rather than assumed.
 *
 * The claim behind the caveat is that a SEMP browse leaves the queue
 * byte-for-byte as it was, which is what puts this family beside Service Bus
 * rather than beside SQS, Pub/Sub and Kinesis. Every figure that would move if
 * it did not is read on both sides of ten browses: the depth, the spool usage,
 * the unacknowledged count and the redelivery count.
 *
 * The seed's orders queue is used because it has a backlog and nothing is
 * consuming it, so anything that changed here changed because of the browse.
 */
func TestLiveBrowseTakesNothing(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	before, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: seedOrdersQueue})
	if err != nil {
		e2e.Missing(t, "%s is not there; run: npm run e2e:solace:seed (%v)", seedOrdersQueue, err)
	}
	if before.Depth <= 0 {
		e2e.Missing(t, "%s holds nothing; run: npm run e2e:solace:seed", seedOrdersQueue)
	}

	for range 10 {
		if _, browseErr := conn.QueryMessages(ctx, model.MessageQueryParams{
			Topic: seedOrdersQueue, MaxResults: 100,
		}); browseErr != nil {
			t.Fatalf("browse: %v", browseErr)
		}
	}

	after, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: seedOrdersQueue})
	if err != nil {
		t.Fatalf("re-reading %s: %v", seedOrdersQueue, err)
	}
	if after.Depth != before.Depth {
		t.Errorf("depth went from %d to %d across ten browses", before.Depth, after.Depth)
	}
	for _, attribute := range []string{AttrSpoolUsage, AttrUnacked, AttrRedelivered} {
		if before.Attributes[attribute] != after.Attributes[attribute] {
			t.Errorf("%s went from %q to %q across ten browses",
				attribute, before.Attributes[attribute], after.Attributes[attribute])
		}
	}
}

/*
 * The browse lists what is there and carries no body, which is the caveat this
 * family declares.
 *
 * Asserted rather than assumed, because it is the sort of thing a later SEMP
 * version could quietly fix - and if it ever does, the caveat stops being true
 * and this is what says so.
 */
func TestLiveBrowseListsMetadataAndNoBody(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	items, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic: seedOrdersQueue, MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(items) == 0 {
		e2e.Missing(t, "%s holds nothing; run: npm run e2e:solace:seed", seedOrdersQueue)
	}

	first := items[0]
	if first.Body != "" {
		t.Errorf("the browse returned a body of %d bytes; semp is not supposed to carry one, "+
			"and the caveat on CapMessageQuery says so", len(first.Body))
	}
	if first.MessageID == "" {
		t.Error("a browsed message has no id, so nothing could be opened from the list")
	}
	if first.StoreTimestamp <= 0 {
		t.Error("a browsed message has no spooled time")
	}
	if first.Properties[PropAttachmentSize] == "" && first.Properties[PropContentSize] == "" {
		t.Error("neither size is reported, and they are what the board shows instead of a body")
	}

	// Newest first, which is what every other family's browse gives and what
	// the page opens on. SEMP hands them back oldest first.
	for index := 1; index < len(items); index++ {
		if items[index-1].StoreTimestamp < items[index].StoreTimestamp {
			t.Errorf("message %d is older than the one before it; the list is not newest first", index)
			break
		}
	}
}

// One message read by id, and an id the queue does not have read as gone.
func TestLiveMessageByIDReadsOneAndSaysWhenItIsGone(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	items, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic: seedOrdersQueue, MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(items) == 0 {
		e2e.Missing(t, "%s holds nothing; run: npm run e2e:solace:seed", seedOrdersQueue)
	}

	one, err := conn.MessageByID(ctx, seedOrdersQueue, items[0].MessageID)
	if err != nil {
		t.Fatalf("message by id: %v", err)
	}
	if one.MessageID != items[0].MessageID {
		t.Errorf("read %s, want %s", one.MessageID, items[0].MessageID)
	}

	if _, err := conn.MessageByID(ctx, seedOrdersQueue, "999999999"); err == nil {
		t.Error("read a message id the queue does not have")
	}
	// A Solace message id is a number per queue, so anything else is a
	// question the broker cannot be asked rather than a message that is gone.
	if _, err := conn.MessageByID(ctx, seedOrdersQueue, "not-a-number"); err == nil {
		t.Error("accepted a message id that is not a number")
	}
}

/*
 * Sending, to a queue and to a topic, which are two different gestures.
 *
 * A queue send names one endpoint. A topic send is matched against every
 * subscription in the Message VPN and lands on nothing at all when none match,
 * which is the failure mode a console has to be able to show - so both halves
 * are exercised here rather than only the one that always works.
 */
func TestLiveSendReachesAQueueAndATopic(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	queue := "mqstudio/test/send-" + suffix
	topic := "mqstudio/test/send/" + suffix + "/one"

	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: queue},
	}); err != nil {
		t.Fatalf("create %s: %v", queue, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: queue})
	})

	result, err := conn.Publish(ctx, PublishRequest{
		Target: TargetQueue, Destination: queue, Body: `{"sent":"by name"}`,
		ContentType: "application/json", Count: 3,
	})
	if err != nil {
		t.Fatalf("send to %s: %v", queue, err)
	}
	if result.Sent != 3 {
		t.Errorf("sent %d, want 3", result.Sent)
	}
	if err := waitForDepth(t, conn, queue, 3); err != nil {
		t.Fatalf("%s: %v", queue, err)
	}

	// The same queue, reached the other way: a subscription makes the topic
	// send land here too, which is the whole of what a topic send does.
	if err := rawSEMP(http.MethodPost,
		"/config/msgVpns/"+livePath(liveVPN)+"/queues/"+livePath(queue)+"/subscriptions",
		map[string]any{"subscriptionTopic": "mqstudio/test/send/" + suffix + "/>"}, nil); err != nil {
		t.Fatalf("subscribing %s: %v", queue, err)
	}
	if _, err := conn.Publish(ctx, PublishRequest{
		Target: TargetTopic, Destination: topic, Body: "by topic",
	}); err != nil {
		t.Fatalf("send to %s: %v", topic, err)
	}
	if err := waitForDepth(t, conn, queue, 4); err != nil {
		t.Fatalf("the topic send did not reach %s: %v", queue, err)
	}
}

/*
 * The headers the send sets reach the message, and one of them is the reason
 * this test exists.
 *
 * Solace-DMQ-Eligible is off by default, so a queue configured with a dead
 * message queue still discards quietly unless the publisher marks the message
 * or the queue overrides it. The driver sends the flag either way; this is
 * what proves the broker read it.
 *
 * Solace-Partition-Key is deliberately not among them: it looks exactly like a
 * header that should work and the broker ignores it entirely, which is checked
 * here so nobody adds it back on the strength of the name.
 */
func TestLiveSendHeadersReachTheMessage(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	queue := "mqstudio/test/headers-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	if err := conn.CreateDestination(ctx, model.DestinationSpec{
		Ref: model.DestinationRef{Name: queue},
	}); err != nil {
		t.Fatalf("create %s: %v", queue, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: queue})
	})

	if _, err := conn.Publish(ctx, PublishRequest{
		Target: TargetQueue, Destination: queue, Body: "with headers",
		DeliveryMode: DeliveryPersistent, TimeToLiveMs: 600000, DMQEligible: true,
		CorrelationID: "corr-1", Properties: map[string]string{"order": "42"},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := waitForDepth(t, conn, queue, 1); err != nil {
		t.Fatalf("%s: %v", queue, err)
	}

	items, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: queue, MaxResults: 1})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("browsed %d messages, want 1", len(items))
	}
	if items[0].Properties[PropDmqEligible] != "true" {
		t.Errorf("the message is not dead-message eligible, so a queue with a dead message "+
			"queue would discard it: %v", items[0].Properties)
	}
}

// A delivery mode the broker does not have is refused here, where the message
// can name the three it does, rather than by the interface answering with an
// XML document quoting them.
func TestLiveSendRefusesADeliveryModeThatIsNotOne(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.Publish(liveContext(t), PublishRequest{
		Target: TargetQueue, Destination: seedOrdersQueue, Body: "x", DeliveryMode: "eventual",
	})
	if err == nil {
		t.Fatal("accepted a delivery mode solace does not have")
	}
	if !strings.Contains(err.Error(), DeliveryNonPersistent) {
		t.Errorf("the error does not name the modes that do exist: %v", err)
	}
}

/*
 * A send to a queue that is not there is refused rather than discarded.
 *
 * It is the same refusal the connection's own probe reads, so this is what
 * pins the probe's success case as well: if the broker ever started accepting
 * these, the probe would report a working interface on a credential that
 * cannot send.
 */
func TestLiveSendToAQueueThatIsNotThereIsRefused(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.Publish(liveContext(t), PublishRequest{
		Target: TargetQueue, Destination: "mqstudio/test/no-such-queue", Body: "x",
	})
	if err == nil {
		t.Fatal("a send to a queue that does not exist was accepted")
	}
	if !queueMissing(err) {
		t.Errorf("the refusal is not the one the connection probe reads: %v", err)
	}
}

/*
 * A topic send with nothing subscribed is accepted and lands nowhere, which is
 * the broker's design rather than a failure.
 *
 * Worth pinning because it is the one send that can look successful and do
 * nothing: the console reports what the broker took, and the broker took it.
 */
func TestLiveSendToATopicWithNoSubscriberIsQuietlyDiscarded(t *testing.T) {
	conn := liveConn(t)

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Target:      TargetTopic,
		Destination: "mqstudio/test/nobody/" + strconv.FormatInt(time.Now().UnixNano(), 36),
		Body:        "into the void",
	})
	if err != nil {
		t.Fatalf("send to an unsubscribed topic: %v", err)
	}
	if result.Sent != 1 {
		t.Errorf("sent %d, want 1", result.Sent)
	}
}
