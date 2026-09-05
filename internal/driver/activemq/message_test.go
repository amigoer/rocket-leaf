package activemq

import (
	"encoding/json"
	"testing"
)

// The two products render the same JMS header in different types, and a driver
// that assumes one gets a plausible wrong answer rather than an error: every
// message reads as having no time, and the board sorts them arbitrarily.
//
// Both forms below came off a real browse - Artemis answers with epoch
// milliseconds, Classic with an ISO-8601 string.
func TestATimestampIsReadFromEitherRendering(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want int64
	}{
		"artemis epoch millis":    {`1788625848418`, 1788625848418},
		"classic iso with millis": {`"2026-09-05T16:34:32.882Z"`, 1788626072882},
		"absent":                  {`null`, 0},
		"empty string":            {`""`, 0},
		"unparseable":             {`"not a time"`, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := millisOf(json.RawMessage(tc.raw)); got != tc.want {
				t.Errorf("millisOf(%s) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// Classic renders JMSDeliveryMode as the word and Artemis reports durability
// as a boolean, so the number JMS defines is the form neither actually sends.
func TestDeliveryModeIsReadFromEitherRendering(t *testing.T) {
	cases := map[string]bool{
		`"PERSISTENT"`:     true,
		`"NON-PERSISTENT"`: false,
		`"non_persistent"`: false,
		`2`:                true,
		`1`:                false,
		`null`:             true,
	}
	for raw, want := range cases {
		if got := isPersistent(json.RawMessage(raw)); got != want {
			t.Errorf("isPersistent(%s) = %v, want %v", raw, got, want)
		}
	}
}

// JMS keeps user properties by type, so a message carrying one of each arrives
// as six separate maps. The canonical model has one, and a reader flattening
// only the string map would silently drop every number a producer set.
func TestTypedPropertyMapsAreFlattenedIntoOne(t *testing.T) {
	entry := map[string]json.RawMessage{
		"StringProperties":  json.RawMessage(`{"tenant":"acme"}`),
		"IntProperties":     json.RawMessage(`{"attempt":3}`),
		"BooleanProperties": json.RawMessage(`{"replay":true}`),
		"LongProperties":    json.RawMessage(`null`),
	}
	properties := jmsProperties(entry)

	for key, want := range map[string]string{"tenant": "acme", "attempt": "3", "replay": "true"} {
		if got := properties[key]; got != want {
			t.Errorf("property %q = %q, want %q", key, got, want)
		}
	}
}
