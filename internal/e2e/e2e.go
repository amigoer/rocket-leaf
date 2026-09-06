// Package e2e gates the tests that need a real broker.
//
// It is imported only from _test.go files. It exists because the rule it
// carries was written four times, once per package, and the copies disagreed:
// two of them consulted the opt-in variable before probing anything, so the
// whole app-layer suite skipped every CI run from the day it was written
// (issue #48).
package e2e

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

// OptIn is the variable a developer sets to run the live suites locally.
const OptIn = "MQ_STUDIO_E2E"

// ShardVar names the families the current run is responsible for, so CI can
// split the live suites across jobs that each start one broker family. Unset
// means every family, which is what a local run and any unsharded run get.
const ShardVar = "MQ_STUDIO_E2E_FAMILIES"

// NoFamilies is the value a run sets to claim nothing at all - the unit job,
// which starts no broker. It is a word rather than the empty string on
// purpose: an empty value is far more likely to be a variable somebody meant
// to fill in, and silently claiming nothing is the shape of #48.
const NoFamilies = "none"

// SkipMarker prefixes every skip this gate produces. The CI coverage check
// greps for it to tell a test no shard claimed - a sharding bug - from a test
// that skipped itself because the broker lacks the feature it covers, which is
// a deliberate omission and not a regression.
const SkipMarker = "[e2e-gate]"

// probeTimeout is how long an environment has to answer that it is there. It
// is not how long the tests then give it to work.
const probeTimeout = 2 * time.Second

// Family is the broker family an environment belongs to. CI shards on it, so
// it is also the unit in which live coverage is accounted for.
type Family string

const (
	RocketMQ Family = "rocketmq"
	RabbitMQ Family = "rabbitmq"
	Kafka    Family = "kafka"
	Pulsar   Family = "pulsar"
	Redis    Family = "redis"
	MQTT     Family = "mqtt"
	NATS     Family = "nats"
	ActiveMQ Family = "activemq"
	NSQ      Family = "nsq"
	SQS      Family = "sqs"
	// GooglePubSub is spelled with the vendor because "pubsub" alone names
	// the pattern rather than the product, and two other families here are
	// publish/subscribe too.
	GooglePubSub Family = "google-pubsub"
	// AzureServiceBus is spelled with the vendor for the same reason: a
	// service bus is a pattern, and this is one vendor's product.
	AzureServiceBus Family = "azure-servicebus"
)

// AllFamilies is the list every other place derives from - the CI shard matrix
// included, which a test in this package pins against it. Enumerating families
// twice is how the lists drift apart.
var AllFamilies = []Family{
	RocketMQ, RabbitMQ, Kafka, Pulsar, Redis, MQTT, NATS, ActiveMQ, NSQ, SQS,
	GooglePubSub, AzureServiceBus,
}

// Env is one broker environment a live test needs.
type Env struct {
	// Name reads inside a sentence: "kafka is not running".
	Name string
	// Family is which broker family Name belongs to. Required: a test whose
	// environment declares none cannot be claimed by any shard, and would go
	// unrun rather than red.
	Family Family
	// Start is the npm script that brings it up.
	Start string
	// Probe reports whether the environment is answering.
	Probe func() error
}

// shard is the set of families this run is responsible for. A nil shard claims
// every family: that is the unsharded case, and it keeps a local `go test` and
// a plain `npm run test:e2e` behaving exactly as they did before sharding.
type shard map[Family]bool

func (s shard) claims(f Family) bool { return s == nil || s[f] }

// verdict is what the gate decided to do with a test.
type verdict int

const (
	run            verdict = iota
	skipOptOut             // locally, and the developer did not ask for the live suites
	skipAbsent             // locally, and the environment is not up
	skipUnclaimed          // another shard of this run owns the family
	failAbsent             // in CI, where an absent environment is the run's problem
	failUndeclared         // the Env named no family, so no shard can own it
)

// parseShard reads ShardVar. The zero value of set is what an unset variable
// gives, and returns a nil shard - every family claimed.
//
// Every other unrecognised input is an error rather than a quiet fallback. A
// misspelled family name that resolved to "claims nothing" would take a whole
// suite out of CI without turning anything red, which is the defect this
// package was written to make impossible.
func parseShard(raw string, set bool) (shard, error) {
	if !set {
		return nil, nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("is set but empty; use %q to claim no families", NoFamilies)
	}
	if strings.TrimSpace(raw) == NoFamilies {
		return shard{}, nil
	}

	known := make(map[Family]bool, len(AllFamilies))
	for _, family := range AllFamilies {
		known[family] = true
	}

	claimed := shard{}
	for _, field := range strings.Split(raw, ",") {
		family := Family(strings.TrimSpace(field))
		if !known[family] {
			return nil, fmt.Errorf("names unknown family %q; known families are %s", family, strings.Join(familyNames(), ", "))
		}
		claimed[family] = true
	}
	return claimed, nil
}

func familyNames() []string {
	names := make([]string, 0, len(AllFamilies))
	for _, family := range AllFamilies {
		names = append(names, string(family))
	}
	sort.Strings(names)
	return names
}

// decide is the policy, kept apart from testing.T so it can be pinned by a
// test of its own rather than only by the CI run it governs.
//
// Locally the suites are opt-in, so a plain `go test ./...` stays offline and
// quick whether or not the brokers happen to be up. In CI the opt-in is not
// consulted at all: the workflow starts every environment, so an absent one is
// a failure, and no variable anybody forgot to set can be the reason a suite
// did not run. That last clause is the whole fix - reading the opt-in first is
// what kept the app-layer suite silent in every CI run (#48).
//
// A shard that does not claim the family skips, which is the one skip CI
// tolerates. It is safe only because the coverage job asserts that every test
// passed in some shard, so a family no shard claims is still caught.
//
// It returns the probe's error too, so the caller reports what it saw without
// probing a second time.
func decide(ci bool, optIn string, s shard, family Family, probe func() error) (verdict, error) {
	if family == "" {
		return failUndeclared, nil
	}
	if !s.claims(family) {
		return skipUnclaimed, nil
	}
	if !ci && optIn == "" {
		return skipOptOut, nil
	}
	switch err := probe(); {
	case err == nil:
		return run, nil
	case ci:
		return failAbsent, err
	default:
		return skipAbsent, err
	}
}

// Require skips the test when env is absent, and fails instead when CI is set.
func Require(t *testing.T, env Env) {
	t.Helper()

	raw, set := os.LookupEnv(ShardVar)
	claimed, err := parseShard(raw, set)
	if err != nil {
		t.Fatalf("%s %v", ShardVar, err)
	}

	switch decision, probeErr := decide(inCI(), os.Getenv(OptIn), claimed, env.Family, env.Probe); decision {
	case failUndeclared:
		t.Fatalf("the %s environment declares no Family; every e2e.Env needs one or no CI shard can claim it", env.Name)
	case skipUnclaimed:
		t.Skipf("%s this run covers %s=%s, and %s is in the %s family", SkipMarker, ShardVar, raw, env.Name, env.Family)
	case skipOptOut:
		t.Skipf("%s set %s=1 and run `%s` to exercise %s", SkipMarker, OptIn, env.Start, env.Name)
	case skipAbsent:
		t.Skipf("%s %s is not running; start it with `%s` (%v)", SkipMarker, env.Name, env.Start, probeErr)
	case failAbsent:
		t.Fatalf("%s must be running in CI: %v", env.Name, probeErr)
	}
}

// Missing reports a precondition the environment answered the probe but did
// not provide - an unseeded group, a broker that then refused the connection.
// Same rule as Require: a failure in CI, a skip with the remedy locally.
func Missing(t *testing.T, format string, args ...any) {
	t.Helper()
	if inCI() {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

func inCI() bool { return os.Getenv("CI") != "" }

// DialTCP probes an environment by opening a connection to one of its ports.
func DialTCP(address string) func() error {
	return func() error {
		conn, err := net.DialTimeout("tcp", address, probeTimeout)
		if err != nil {
			return err
		}
		return conn.Close()
	}
}

// HTTPGet probes an environment through its management API.
func HTTPGet(url string) func() error {
	return func() error {
		client := &http.Client{Timeout: probeTimeout}
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		return response.Body.Close()
	}
}

// DockerContainer probes an environment the test drives with docker exec
// rather than over the wire, where a reachable port is not enough.
func DockerContainer(name string) func() error {
	return func() error {
		if err := exec.Command("docker", "inspect", name).Run(); err != nil {
			return fmt.Errorf("docker inspect %s: %w", name, err)
		}
		return nil
	}
}
