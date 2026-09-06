package mqtt

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * An in-process broker, so the dial paths can be tested with nothing running.
 *
 * mochi-mqtt is to this driver what kfake is to the Kafka one, with one
 * difference that matters here: it speaks both 3.1.1 and 5.0, so a single
 * fake exercises both client libraries and the seam between them. Everything
 * these tests cover — a refused credential, a vacated address, a cancelled
 * dial — is a case a live broker can only be talked into with difficulty.
 */

// fakeBroker starts a broker that accepts anyone, and returns its address.
func fakeBroker(t *testing.T) string {
	t.Helper()
	_, address := startBroker(t, listeners.TypeTCP, new(auth.AllowHook), nil)
	return address
}

// fakeBrokerServer is the same broker with the server handed back, for tests
// that need to watch what arrived from the broker's own side.
func fakeBrokerServer(t *testing.T) (*mochi.Server, string) {
	t.Helper()
	return startBroker(t, listeners.TypeTCP, new(auth.AllowHook), nil)
}

// fakeWebSocketBroker is the same broker reached over WebSocket, which is a
// separate listener rather than a flag: the transports are different sockets
// on a real broker too.
func fakeWebSocketBroker(t *testing.T) string {
	t.Helper()
	_, address := startBroker(t, listeners.TypeWS, new(auth.AllowHook), nil)
	return address
}

// fakeBrokerWithCredentials only admits the one username and password, which
// is what makes a rejected credential distinguishable from an absent broker.
func fakeBrokerWithCredentials(t *testing.T, username, password string) string {
	t.Helper()
	ledger := &auth.Ledger{
		Auth: auth.AuthRules{{
			Username: auth.RString(username),
			Password: auth.RString(password),
			Allow:    true,
		}},
	}
	_, address := startBroker(t, listeners.TypeTCP, new(auth.Hook), &auth.Options{Ledger: ledger})
	return address
}

func startBroker(t *testing.T, kind string, hook mochi.Hook, config any) (*mochi.Server, string) {
	t.Helper()

	server := mochi.New(&mochi.Options{
		// The broker logs every connection at info, which buries the actual
		// test output. Errors still surface as test failures.
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// Lets a test subscribe from the broker's own side, which is how a
		// publish is checked to have actually arrived rather than merely to
		// have been accepted.
		InlineClient: true,
		// mochi republishes its own $SYS tree every second by default, which
		// overwrites anything a test put there. It is published once at Serve
		// either way, so this leaves the broker's state still rather than
		// silently racing whatever is reading it.
		SysTopicResendInterval: 3600,
	})
	if err := server.AddHook(hook, config); err != nil {
		t.Fatalf("add auth hook: %v", err)
	}

	// Port 0 rather than a guessed one: these tests run in parallel with the
	// live suites, and a fixed port would collide with whatever is already up.
	// The TCP listener binds inside AddListener and can then say which port it
	// was given; the WebSocket one is an http.Server that does not bind until
	// Serve and only ever reports the address it was configured with, so it
	// has to be handed a port that is already known to be free.
	var listener listeners.Listener
	switch kind {
	case listeners.TypeWS:
		listener = listeners.NewWebsocket(listeners.Config{ID: "test", Address: freePort(t)})
	default:
		listener = listeners.NewTCP(listeners.Config{ID: "test", Address: "127.0.0.1:0"})
	}
	if err := server.AddListener(listener); err != nil {
		t.Fatalf("add listener: %v", err)
	}
	// Serve does not block: it starts the listeners, publishes mochi's own
	// $SYS tree once, and returns. Running it here rather than in a goroutine
	// is what puts that publish before anything a test writes. In a goroutine
	// it lands at an arbitrary moment, and a test writing Mosquitto's key
	// names loses whichever of the two dozen names the brokers share had
	// already been written - a different subset every run, because a map is
	// walked in a different order every time.
	if err := server.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	t.Cleanup(func() { closeBroker(server, listener.ID()) })

	address := listener.Address()
	waitForListener(t, address)
	return server, address
}

/*
 * closeBroker shuts a fake broker down once its listener is empty.
 *
 * mochi's Clients.GetByListener takes a read lock and then calls Len, which
 * takes the same lock again. Go's RWMutex does not allow that safely: a writer
 * arriving between the two - Clients.Delete, which is what a disconnecting
 * client runs - makes the second read lock wait behind it and it never
 * returns. Server.Close walks exactly that path, so closing while a client is
 * still going away hangs the whole package until the binary's own timeout,
 * naming whichever test happened to be running.
 *
 * Waiting for the listener to be empty is what keeps the suite off it: with
 * nothing left to disconnect, no writer arrives while Close is walking.
 * mochi-mqtt v2.7.9 is the newest release and none carries a fix.
 *
 * The inline client is deliberately not counted. It is the broker's own, it
 * sits on LocalListener rather than this one, and it never disconnects - so
 * waiting for the map itself to empty would wait forever.
 */
func closeBroker(server *mochi.Server, listenerID string) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !listenerHasClients(server, listenerID) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = server.Close()
}

// listenerHasClients reports whether anything is still attached to one
// listener. GetAll copies the map under a single read lock, which is the part
// of this API that is safe to call.
func listenerHasClients(server *mochi.Server, listenerID string) bool {
	for _, client := range server.Clients.GetAll() {
		if client.Net.Listener == listenerID && !client.Closed() {
			return true
		}
	}
	return false
}

// freePort finds an address nothing is using, by taking it and giving it back.
//
// There is a window between the release and the broker's own bind, and no way
// to close it: the http.Server underneath mochi's WebSocket listener insists
// on binding for itself. waitForListener below is what turns losing that race
// into a clear failure rather than a mysterious dial error.
func freePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release the reserved port: %v", err)
	}
	return address
}

// waitForListener blocks until the broker is accepting connections.
//
// Only the WebSocket listener needs it - it binds inside Serve, in a goroutine
// - but waiting for both costs one dial and means no test can start racing the
// broker it was handed.
func waitForListener(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("broker never started listening on %s: %v", address, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// vacatedAddress is an address with nothing listening on it.
//
// A broker is started and stopped rather than a port being guessed, because a
// guess is only ever probably free — and a test that passes because something
// unrelated happens to be down is worse than no test.
func vacatedAddress(t *testing.T) string {
	t.Helper()

	server := mochi.New(&mochi.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	listener := listeners.NewTCP(listeners.Config{ID: "vacated", Address: "127.0.0.1:0"})
	if err := server.AddListener(listener); err != nil {
		t.Fatalf("add listener: %v", err)
	}
	address := listener.Address()
	if err := server.Close(); err != nil {
		t.Fatalf("close broker: %v", err)
	}
	return address
}

// testProfile is a connection profile pointed at address, with the options a
// test wants layered over the ones every profile here needs.
func testProfile(address, version string, options map[string]string) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       "test",
		Kind:       model.KindMQTT,
		Endpoints:  address,
		TimeoutSec: 3,
		Options: map[string]string{
			OptionProtocolVersion: version,
		},
	}
	for key, value := range options {
		profile.Options[key] = value
	}
	return profile
}

// openProfile opens a connection and closes it when the test ends.
func openProfile(t *testing.T, profile model.ConnectionProfile) *Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opened, err := New().Open(ctx, profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn, ok := opened.(*Conn)
	if !ok {
		t.Fatalf("open returned %T, want *Conn", opened)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Both client libraries have to reach the same broker and answer the same
// questions, because the protocol version is a field on the form and the rest
// of the driver is written against the seam rather than against either one.
func TestOpenConnectsAtEitherProtocolVersion(t *testing.T) {
	address := fakeBroker(t)

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			conn := openProfile(t, testProfile(address, version, nil))

			if conn.Kind() != model.KindMQTT {
				t.Errorf("kind = %q, want mqtt", conn.Kind())
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := conn.Ping(ctx); err != nil {
				t.Errorf("ping: %v", err)
			}
		})
	}
}

// WebSocket is not a variation on the TCP path: it is a different scheme, a
// different port and a URL with a path on it, and getting any of the three
// wrong fails at dial time with an error that reads like a wrong address.
func TestOpenConnectsOverWebSocket(t *testing.T) {
	address := fakeWebSocketBroker(t)

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			conn := openProfile(t, testProfile(address, version, map[string]string{
				OptionTransport: transportWS,
				// mochi serves MQTT at the root, which is also what a default
				// Mosquitto does. The field defaulting to /mqtt is EMQX's
				// convention, so this is the case that proves it is a field.
				OptionWebSocketPath: "/",
			}))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := conn.Ping(ctx); err != nil {
				t.Errorf("ping: %v", err)
			}
		})
	}
}

// A rejected credential and an absent broker send an operator to completely
// different places, so Open has to fail differently for them. This asserts
// only that it fails: what the difference is called is the probe's job, and
// arrives with it.
func TestOpenReportsARejectedCredential(t *testing.T) {
	address := fakeBrokerWithCredentials(t, "mqstudio", "correct-password")

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			profile := testProfile(address, version, nil)
			profile.TimeoutSec = 1
			profile.Auth.Mechanism = model.AuthPlain
			profile.Secrets = map[string]string{
				SecretUsername: "mqstudio",
				SecretPassword: "wrong-password",
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, err := New().Open(ctx, profile)
			if err == nil {
				_ = conn.Close()
				t.Fatal("open accepted a wrong password")
			}
		})
	}
}

// The same profile with the right password has to work, or the test above
// would pass against a driver that never authenticates successfully.
func TestOpenAcceptsTheRightCredential(t *testing.T) {
	address := fakeBrokerWithCredentials(t, "mqstudio", "correct-password")

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			profile := testProfile(address, version, nil)
			profile.Auth.Mechanism = model.AuthPlain
			profile.Secrets = map[string]string{
				SecretUsername: "mqstudio",
				SecretPassword: "correct-password",
			}
			openProfile(t, profile)
		})
	}
}

// Nothing listening has to end the dial, not begin an endless retry. autopaho
// reconnects until its context ends, so without the bound in Connect this
// hangs rather than fails - which is the whole reason the case is pinned.
func TestOpenGivesUpOnAnUnreachableBroker(t *testing.T) {
	address := vacatedAddress(t)

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			profile := testProfile(address, version, nil)
			profile.TimeoutSec = 1

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			start := time.Now()
			conn, err := New().Open(ctx, profile)
			if err == nil {
				_ = conn.Close()
				t.Fatal("open succeeded against an address with nothing on it")
			}
			if elapsed := time.Since(start); elapsed > 8*time.Second {
				t.Errorf("open took %v to give up; the dial timeout is not bounding it", elapsed)
			}
		})
	}
}

// A user who closes the dialog should not leave a dial running for the rest of
// the timeout.
func TestOpenHonoursACancelledContext(t *testing.T) {
	address := vacatedAddress(t)

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			profile := testProfile(address, version, nil)
			profile.TimeoutSec = 30

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			conn, err := New().Open(ctx, profile)
			if err == nil {
				_ = conn.Close()
				t.Fatal("open succeeded on a cancelled context")
			}
		})
	}
}

// Open must refuse a profile it cannot turn into a dial, rather than building
// a client that fails later with an error about the network.
func TestOpenRefusesAProfileItCannotDial(t *testing.T) {
	tests := []struct {
		name    string
		profile model.ConnectionProfile
	}{
		{
			name:    "no address",
			profile: testProfile("", protocol5, nil),
		},
		{
			name:    "only separators",
			profile: testProfile(" , ; ", protocol5, nil),
		},
		{
			name: "unknown transport",
			profile: testProfile("127.0.0.1:1883", protocol5, map[string]string{
				OptionTransport: "carrier-pigeon",
			}),
		},
		{
			name: "unreadable CA file",
			profile: testProfile("127.0.0.1:8883", protocol5, map[string]string{
				OptionTransport: transportTLS,
				OptionTLSCAFile: "/nonexistent/ca.pem",
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := New().Open(ctx, test.profile)
			if err == nil {
				_ = conn.Close()
				t.Fatal("open accepted a profile it cannot dial")
			}
		})
	}
}

// Ping has to touch the broker. A session object that still believes it is
// connected is exactly what a "test connection" button must not report as
// success, so the broker is stopped underneath a live connection here.
func TestPingFailsOnceTheBrokerIsGone(t *testing.T) {
	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			server := mochi.New(&mochi.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
				t.Fatalf("add auth hook: %v", err)
			}
			listener := listeners.NewTCP(listeners.Config{ID: "test", Address: "127.0.0.1:0"})
			if err := server.AddListener(listener); err != nil {
				t.Fatalf("add listener: %v", err)
			}
			go func() { _ = server.Serve() }()
			address := listener.Address()

			profile := testProfile(address, version, nil)
			profile.TimeoutSec = 1
			conn := openProfile(t, profile)

			if err := server.Close(); err != nil {
				t.Fatalf("close broker: %v", err)
			}

			// Bounded tightly: the v5 client reconnects until its context
			// ends, so this is how long the assertion costs.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := conn.Ping(ctx); err == nil {
				t.Error("ping succeeded against a broker that is gone")
			}
		})
	}
}

// The registry closes a connection on both disconnect and shutdown, so the
// second call has to be harmless rather than a panic on a closed channel.
func TestCloseIsRepeatable(t *testing.T) {
	address := fakeBroker(t)

	for _, version := range []string{protocol5, protocol311} {
		t.Run("mqtt"+version, func(t *testing.T) {
			conn := openProfile(t, testProfile(address, version, nil))

			if err := conn.Close(); err != nil {
				t.Fatalf("first close: %v", err)
			}
			if err := conn.Close(); err != nil {
				t.Errorf("second close: %v", err)
			}
		})
	}
}
