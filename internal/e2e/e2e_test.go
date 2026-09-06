package e2e

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The whole policy as a table. The first row is the regression this package
// exists for: in CI, with nobody having set the opt-in, the suite runs.
//
// Every row claims the family it names, so these rows say what the gate did
// before sharding existed; the shard rules get their own tables below.
func TestDecide(t *testing.T) {
	absent := errors.New("nothing listening")
	up := func() error { return nil }
	down := func() error { return absent }

	for _, test := range []struct {
		name  string
		ci    bool
		optIn string
		probe func() error
		want  verdict
	}{
		{"CI does not consult the opt-in", true, "", up, run},
		{"CI fails rather than skips when the environment is absent", true, "", down, failAbsent},
		{"CI runs with the opt-in set as well", true, "1", up, run},
		{"locally the opt-in is what asks for the live suites", false, "", up, skipOptOut},
		{"locally an absent environment is a skip", false, "1", down, skipAbsent},
		{"locally the opt-in and a live environment run", false, "1", up, run},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := decide(test.ci, test.optIn, nil, Kafka, test.probe)
			if got != test.want {
				t.Fatalf("decide = %d, want %d", got, test.want)
			}
			if (err != nil) != (test.want == skipAbsent || test.want == failAbsent) {
				t.Fatalf("decide returned err %v with verdict %d", err, got)
			}
		})
	}
}

// Opting out must not dial anything: it is what keeps `go test ./...` quick on
// a checkout with the brokers running.
func TestDecideDoesNotProbeWhenOptedOut(t *testing.T) {
	probed := 0
	probe := func() error { probed++; return nil }

	if _, _ = decide(false, "", nil, Kafka, probe); probed != 0 {
		t.Fatalf("probed %d times while opted out", probed)
	}
	if _, _ = decide(true, "", nil, Kafka, probe); probed != 1 {
		t.Fatalf("probed %d times in CI, want 1", probed)
	}
}

// An Env with no family cannot be claimed by any shard, so it would go unrun
// rather than red. That is the #48 shape, so it fails everywhere - in CI, and
// locally too, where whoever added the Env is the one who can fix it.
func TestDecideFailsAnEnvThatDeclaresNoFamily(t *testing.T) {
	for _, ci := range []bool{true, false} {
		got, _ := decide(ci, "1", nil, "", func() error { return nil })
		if got != failUndeclared {
			t.Fatalf("decide with no family in ci=%v = %d, want failUndeclared", ci, got)
		}
	}
}

// The family check comes first, so an unclaimed family never dials: a shard
// that did not start RabbitMQ should not wait on RabbitMQ's port.
func TestDecideDoesNotProbeAnUnclaimedFamily(t *testing.T) {
	probed := 0
	probe := func() error { probed++; return nil }

	got, _ := decide(true, "1", shard{Kafka: true}, RabbitMQ, probe)
	if got != skipUnclaimed {
		t.Fatalf("decide = %d, want skipUnclaimed", got)
	}
	if probed != 0 {
		t.Fatalf("probed %d times for an unclaimed family", probed)
	}
}

// A claimed family in CI keeps the old rule: absent is the run's problem.
func TestDecideStillFailsAClaimedFamilyThatIsAbsent(t *testing.T) {
	got, err := decide(true, "", shard{Kafka: true}, Kafka, func() error { return errors.New("down") })
	if got != failAbsent {
		t.Fatalf("decide = %d, want failAbsent", got)
	}
	if err == nil {
		t.Fatal("decide returned no error with failAbsent")
	}
}

func TestParseShard(t *testing.T) {
	for _, test := range []struct {
		name   string
		raw    string
		set    bool
		want   shard
		wantOK bool
	}{
		{"unset claims every family", "", false, nil, true},
		{"none claims nothing", NoFamilies, true, shard{}, true},
		{"one family", "kafka", true, shard{Kafka: true}, true},
		{"several families", "kafka,redis", true, shard{Kafka: true, Redis: true}, true},
		{"surrounding space is tolerated", " kafka , redis ", true, shard{Kafka: true, Redis: true}, true},

		// Both of these would take a suite out of CI without turning
		// anything red, which is exactly what this package prevents.
		{"set but empty is an error", "", true, nil, false},
		{"whitespace only is an error", "   ", true, nil, false},
		{"an unknown family is an error", "rocketmqq", true, nil, false},
		{"one unknown family among good ones is an error", "kafka,nope", true, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseShard(test.raw, test.set)
			if (err == nil) != test.wantOK {
				t.Fatalf("parseShard(%q, %v) err = %v, want ok = %v", test.raw, test.set, err, test.wantOK)
			}
			if !test.wantOK {
				return
			}
			if (got == nil) != (test.want == nil) {
				t.Fatalf("parseShard(%q) = %v, want %v", test.raw, got, test.want)
			}
			for _, family := range AllFamilies {
				if got.claims(family) != test.want.claims(family) {
					t.Fatalf("parseShard(%q).claims(%s) = %v, want %v", test.raw, family, got.claims(family), test.want.claims(family))
				}
			}
		})
	}
}

// A nil shard is the unsharded case, and has to claim everything or a local
// run would stop exercising the live suites.
func TestNilShardClaimsEveryFamily(t *testing.T) {
	var unsharded shard
	for _, family := range AllFamilies {
		if !unsharded.claims(family) {
			t.Fatalf("the unsharded case does not claim %s", family)
		}
	}
}

// The CI shard matrix and AllFamilies are two lists of the same thing, and
// two lists drift. This is the one that matters: a family with no shard is a
// family whose live tests skip in every job of the run, which is #48 again.
// The coverage job would catch it after the fact; this catches it in review.
func TestEveryFamilyHasACIShard(t *testing.T) {
	const workflow = "../../.github/workflows/ci.yml"

	source, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatalf("reading %s: %v", workflow, err)
	}

	// The matrix spells one family per line as `- family: <name>`. Matching
	// the line rather than parsing YAML keeps this test off the dependency
	// list; the format is pinned by the failure message below. The hyphen is
	// part of a name rather than a separator: google-pubsub is one family.
	pattern := regexp.MustCompile(`(?m)^\s*-\s*family:\s*([a-z][a-z-]*)\s*$`)
	matches := pattern.FindAllSubmatch(source, -1)
	if len(matches) == 0 {
		t.Fatalf("found no `- family: <name>` entries in %s; the shard matrix moved and this guard needs to follow it", workflow)
	}

	sharded := map[string]bool{}
	for _, match := range matches {
		sharded[string(match[1])] = true
	}

	declared := map[string]bool{}
	for _, family := range AllFamilies {
		declared[string(family)] = true
	}

	var missing, unknown []string
	for family := range declared {
		if !sharded[family] {
			missing = append(missing, family)
		}
	}
	for family := range sharded {
		if !declared[family] {
			unknown = append(unknown, family)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)

	if len(missing) > 0 {
		t.Errorf("families with no shard in %s: %v - their live tests would skip in every job", workflow, missing)
	}
	if len(unknown) > 0 {
		t.Errorf("shards in %s for families this package does not declare: %v", workflow, unknown)
	}
}

// The CI coverage check greps skip output for SkipMarker to tell a test no
// shard claimed from a test that skipped itself. A gate skip that forgot the
// marker would read as a deliberate omission and stop being accounted for, so
// the marker is pinned here rather than left to review.
func TestEveryGateSkipCarriesTheMarker(t *testing.T) {
	source, err := os.ReadFile("e2e.go")
	if err != nil {
		t.Fatalf("reading e2e.go: %v", err)
	}

	// Require only. Missing's skip is local-only - in CI it is a failure - so
	// it never reaches the coverage check and needs no marker.
	body := regexp.MustCompile(`(?s)func Require\(.*?\n\}`).FindString(string(source))
	if body == "" {
		t.Fatal("could not find func Require in e2e.go; this guard needs to follow it")
	}

	skips := regexp.MustCompile(`t\.Skipf?\([^\n]*`).FindAllString(body, -1)
	if len(skips) == 0 {
		t.Fatal("found no t.Skip calls in Require; this guard needs to follow them")
	}
	for _, skip := range skips {
		if !strings.Contains(skip, "SkipMarker") {
			t.Errorf("gate skip without SkipMarker: %s", skip)
		}
	}
}
