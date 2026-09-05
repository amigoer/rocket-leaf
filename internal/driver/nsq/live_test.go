package nsq

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live cluster, as tests/e2e/nsq/compose.yaml publishes it.
//
// Two nsqd and two nsqlookupd, and both halves matter. A topic lives on the
// daemon it was created on, so only a second nsqd can prove a cluster figure
// is a sum rather than one node's answer.
const (
	liveNSQD1    = "http://127.0.0.1:4151"
	liveNSQD2    = "http://127.0.0.1:4153"
	liveLookupd1 = "http://127.0.0.1:4161"
	liveLookupd2 = "http://127.0.0.1:4163"
)

func requireCluster(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.NSQ,
		Name:   "the nsq cluster",
		Start:  "npm run e2e:nsq:up",
		Probe:  e2e.HTTPGet(liveNSQD1 + "/ping"),
	})
}

// liveProfile is the cluster as a user would configure it: both nsqd in the
// address field, both nsqlookupd in the discovery one.
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "nsq e2e",
		Kind:       model.KindNSQ,
		Endpoints:  liveNSQD1 + "," + liveNSQD2,
		TimeoutSec: 10,
		Options:    map[string]string{OptionLookupd: liveLookupd1 + "," + liveLookupd2},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireCluster(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Opening has to reach every address in the profile, not the first one that
// answers: a cluster is the set, and a connection that opened against half of
// it would report depths that are quietly short.
func TestLiveOpenReachesEveryDaemon(t *testing.T) {
	conn := liveConn(t)

	if len(conn.nodes) != 2 {
		t.Fatalf("opened against %d nsqd, want 2", len(conn.nodes))
	}
	for _, node := range conn.nodes {
		if node.info.Version == "" {
			t.Errorf("%s reported no version", node.address)
		}
		if node.info.TCPPort == 0 {
			t.Errorf("%s reported no tcp port, so it did not answer as an nsqd", node.address)
		}
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

/*
 * The two daemons answer /info at adjacent ports and neither refuses the
 * other's field, so putting one address in the wrong row is the likeliest way
 * to fill this form in wrong. It has to fail at open with a sentence naming
 * the swap - the alternative is a connection that opens and then reports a
 * cluster with no topics, which reads as an empty broker rather than as a
 * misconfiguration.
 */
func TestLiveOpenRefusesTheDaemonsSwappedOver(t *testing.T) {
	requireCluster(t)

	t.Run("an nsqlookupd in the nsqd field", func(t *testing.T) {
		profile := liveProfile()
		profile.Endpoints = liveLookupd1
		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened against an nsqlookupd as though it were an nsqd")
		}
		if !strings.Contains(err.Error(), "lookupd field") {
			t.Errorf("error does not say where the address belongs: %v", err)
		}
	})

	t.Run("an nsqd in the lookupd field", func(t *testing.T) {
		profile := liveProfile()
		profile.Options[OptionLookupd] = liveNSQD1
		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened against an nsqd as though it were an nsqlookupd")
		}
		if !strings.Contains(err.Error(), "nsqd field") {
			t.Errorf("error does not say where the address belongs: %v", err)
		}
	})
}

// A daemon that is not there has to fail the whole open rather than leave a
// connection speaking for a smaller cluster than the profile named.
func TestLiveOpenFailsWhenOneDaemonIsAbsent(t *testing.T) {
	requireCluster(t)

	profile := liveProfile()
	profile.Endpoints = liveNSQD1 + ",http://127.0.0.1:4159"
	profile.TimeoutSec = 2
	if _, err := open(liveContext(t), profile); err == nil {
		t.Fatal("opened against a cluster with an unreachable nsqd in it")
	}
}

// Close is called on disconnect and again on shutdown, so the second call has
// to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireCluster(t)

	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answered a ping")
	}
}
