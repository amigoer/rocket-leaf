package sqs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live environment, as tests/e2e/sqs/compose.yaml publishes it.
//
// LocalStack, reached through the endpoint override. That is not a testing
// shortcut around the real code path: a VPC interface endpoint is configured
// with the same field, so every test here exercises it.
const (
	liveEndpoint  = "http://127.0.0.1:4566"
	liveRegion    = "eu-west-1"
	liveAccessKey = "test"
	liveSecretKey = "test"
)

// The queues scripts/e2e-sqs-seed.sh creates. Tests read these and never write
// to them; anything a test needs to change it creates for itself.
const (
	seedOrders  = "MQS-SEED-orders"
	seedDLQ     = "MQS-SEED-orders-dlq"
	seedDelayed = "MQS-SEED-delayed"
	seedEmpty   = "MQS-SEED-empty"
	seedFIFO    = "MQS-SEED-orders.fifo"
)

func requireRegion(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.SQS,
		Name:   "the sqs environment",
		Start:  "npm run e2e:sqs:up",
		Probe:  e2e.HTTPGet(liveEndpoint + "/_localstack/health"),
	})
}

// liveProfile is the environment as a user would configure it: a region, a
// credential, and no address anywhere on the form.
func liveProfile() model.ConnectionProfile {
	profile := model.ConnectionProfile{
		ID:         1,
		Name:       "sqs e2e",
		Kind:       model.KindSQS,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			OptionRegion:      liveRegion,
			OptionEndpointURL: liveEndpoint,
		},
	}
	profile.SetSecret(SecretAccessKeyID, liveAccessKey)
	profile.SetSecret(SecretSecretAccessKey, liveSecretKey)
	return profile
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireRegion(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

/*
 * The point of this family, exercised end to end.
 *
 * A profile whose Endpoints is empty opens, which nothing else in this app can
 * do. It is asserted here rather than only in the descriptor test because the
 * descriptor only says the form asks for no address - this proves the driver
 * needs none either.
 */
func TestLiveOpenNeedsNoAddress(t *testing.T) {
	requireRegion(t)

	profile := liveProfile()
	if profile.Endpoints != "" {
		t.Fatalf("the live profile carries an address %q; this family has none", profile.Endpoints)
	}

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn.Region() != liveRegion {
		t.Errorf("region = %q, want %q", conn.Region(), liveRegion)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// Nothing is dialled, so a wrong region, a wrong credential and a wrong
// endpoint all look identical until a request is signed and sent. Open has to
// send one, or every one of them opens and then reports an empty account.
func TestLiveOpenProvesTheCredentialReachesSQS(t *testing.T) {
	requireRegion(t)

	t.Run("an endpoint with nothing behind it", func(t *testing.T) {
		profile := liveProfile()
		profile.TimeoutSec = 2
		profile.Options[OptionEndpointURL] = "http://127.0.0.1:4599"
		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened against an endpoint that answers nothing")
		}
	})

	t.Run("a profile naming no region", func(t *testing.T) {
		profile := liveProfile()
		delete(profile.Options, OptionRegion)
		err := func() error { _, err := open(liveContext(t), profile); return err }()
		if err == nil {
			t.Fatal("opened with no region to sign for")
		}
		if !strings.Contains(err.Error(), "region") {
			t.Errorf("error does not name the missing field: %v", err)
		}
	})
}

// Close is called on disconnect and again on shutdown, so the second call has
// to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireRegion(t)

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
		t.Error("a closed connection still answers a ping")
	}
}

// The name a user types has to reach the URL every call actually takes, and a
// name nothing answers for has to say so rather than failing later with an
// SDK error naming an operation the user never asked for.
func TestLiveQueueURLResolvesANameAndRefusesAnUnknownOne(t *testing.T) {
	conn := liveConn(t)

	url, err := conn.queueURL(liveContext(t), seedOrders)
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:sqs:seed` (%v)", seedOrders, err)
	}
	if !strings.HasSuffix(url, "/"+seedOrders) {
		t.Errorf("queue url %q does not end in the queue name", url)
	}
	if queueNameOf(url) != seedOrders {
		t.Errorf("queueNameOf(%q) = %q, want %q", url, queueNameOf(url), seedOrders)
	}

	_, err = conn.queueURL(liveContext(t), "MQS-TEST-not-here")
	if err == nil {
		t.Fatal("resolved a queue that does not exist")
	}
	if !strings.Contains(err.Error(), "MQS-TEST-not-here") {
		t.Errorf("error does not name the queue that is missing: %v", err)
	}
}
