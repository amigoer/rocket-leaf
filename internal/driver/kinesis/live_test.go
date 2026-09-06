package kinesis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live environment, as tests/e2e/kinesis/compose.yaml publishes it.
//
// LocalStack, reached through the endpoint override. That is not a testing
// shortcut around the real code path: a VPC interface endpoint is configured
// with the same field, so every test here exercises it.
//
// The port is not the SQS stack's. Two LocalStack containers run at once on
// one machine, which is what keeps the two AWS families independent.
const (
	liveEndpoint  = "http://127.0.0.1:4567"
	liveRegion    = "eu-west-1"
	liveAccessKey = "test"
	liveSecretKey = "test"
)

// The streams scripts/e2e-kinesis-seed.sh creates. Tests read these and never
// write to them; anything a test needs to change it creates for itself.
const (
	seedOrders   = "MQS-SEED-orders"
	seedSplit    = "MQS-SEED-split"
	seedEmpty    = "MQS-SEED-empty"
	seedOnDemand = "MQS-SEED-ondemand"
)

func requireRegion(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.Kinesis,
		Name:   "the kinesis environment",
		Start:  "npm run e2e:kinesis:up",
		Probe:  e2e.HTTPGet(liveEndpoint + "/_localstack/health"),
	})
}

// liveProfile is the environment as a user would configure it: a region, a
// credential, and no address anywhere on the form.
func liveProfile() model.ConnectionProfile {
	profile := model.ConnectionProfile{
		ID:         1,
		Name:       "kinesis e2e",
		Kind:       model.KindKinesis,
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
 * A profile whose Endpoints is empty opens, which only the hosted families can
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
func TestLiveOpenProvesTheCredentialReachesKinesis(t *testing.T) {
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
		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened with no region to sign for")
		} else if !strings.Contains(err.Error(), "region") {
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

// The consumer calls take a stream's ARN and nothing else, so the name a user
// types has to reach one - and a name nothing answers for has to say so rather
// than failing later with an SDK error naming an operation nobody asked for.
func TestLiveStreamARNResolvesANameAndRefusesAnUnknownOne(t *testing.T) {
	conn := liveConn(t)

	arn, err := conn.streamARN(liveContext(t), seedOrders)
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedOrders, err)
	}
	if !strings.HasSuffix(arn, ":stream/"+seedOrders) {
		t.Errorf("stream arn %q does not end in the stream name", arn)
	}
	if streamNameOf(arn) != seedOrders {
		t.Errorf("streamNameOf(%q) = %q, want %q", arn, streamNameOf(arn), seedOrders)
	}

	if _, err := conn.streamARN(liveContext(t), "MQS-TEST-not-here"); err == nil {
		t.Fatal("resolved a stream that does not exist")
	} else if !strings.Contains(err.Error(), "MQS-TEST-not-here") {
		t.Errorf("error does not name the stream that is missing: %v", err)
	}
}
