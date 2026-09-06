package solace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
