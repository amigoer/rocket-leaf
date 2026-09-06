package azureservicebus

import (
	"fmt"
	"strconv"
	"strings"
)

/*
 * ISO-8601 durations, both ways.
 *
 * Every timespan on a Service Bus entity is one - PT1M for a lock, P14D for a
 * time to live, P10675199DT2H48M5.4775807S for "never" - and neither Go nor
 * the SDK converts. The boards want whole seconds, and a form sends whole
 * seconds back, so this is the one place the shape is written down.
 *
 * Only days downwards are handled. Years and months are legal ISO and have no
 * fixed length, so a caller that met one would have to invent a calendar;
 * Service Bus never sends one, which is why an unparsed value leaves its
 * attribute out rather than guessing at it.
 */

// isoSeconds reads an ISO-8601 duration as whole seconds.
//
// Fractional seconds are truncated rather than rounded: they only appear in
// the service's own "never" value, where 0.4775807 of a second either way
// changes nothing, and truncating cannot turn a sub-second timespan into one
// the form would then send back as a whole second.
func isoSeconds(value string) (int64, bool) {
	text := strings.TrimSpace(strings.ToUpper(value))
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	if !strings.HasPrefix(text, "P") {
		return 0, false
	}
	text = text[1:]

	date, timePart, split := strings.Cut(text, "T")
	if !split {
		date, timePart = text, ""
	}

	var total float64
	consume := func(section string, units map[byte]float64) bool {
		digits := strings.Builder{}
		for index := 0; index < len(section); index++ {
			character := section[index]
			if (character >= '0' && character <= '9') || character == '.' {
				digits.WriteByte(character)
				continue
			}
			scale, known := units[character]
			if !known || digits.Len() == 0 {
				return false
			}
			amount, err := strconv.ParseFloat(digits.String(), 64)
			if err != nil {
				return false
			}
			total += amount * scale
			digits.Reset()
		}
		// A trailing run of digits has no unit, which is not a duration.
		return digits.Len() == 0
	}

	if !consume(date, map[byte]float64{'D': 86400, 'W': 604800}) {
		return 0, false
	}
	if !consume(timePart, map[byte]float64{'H': 3600, 'M': 60, 'S': 1}) {
		return 0, false
	}
	seconds := int64(total)
	if negative {
		seconds = -seconds
	}
	return seconds, true
}

// isoDuration writes whole seconds as the shape the API takes.
//
// Always a time component, never a date one: PT604800S is as valid as P7D and
// needs no arithmetic that could disagree with the parser above.
func isoDuration(seconds int) string {
	return fmt.Sprintf("PT%dS", seconds)
}
