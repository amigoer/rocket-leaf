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
