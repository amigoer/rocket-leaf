package activemq

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/go-amqp"
)

// The AMQP 1.0 tier.
//
// Everything the pages need except two things is a JMX operation, so this tier
// is optional and its absence costs a connection nothing else: browsing and
// sending both go over Jolokia, and browsing consumes nothing. What needs a
// real client is following a destination as messages arrive - JMX is
// request/response and has no push - and sending a body that is not text,
// which Classic's sendTextMessage cannot express.
//
// Released Classic and Artemis both accept AMQP out of the box today, but the
// upstream Classic default is moving to OpenWire-only for attack surface, so
// the tier is probed rather than assumed. A broker with the acceptor off keeps
// every other page.

// amqpDialTimeout bounds the probe.
//
// Short on purpose and separate from the profile's timeout: this runs while
// the user is waiting on a connection dialog, and a closed acceptor should
// report itself rather than hold the dialog for the ten seconds a Jolokia call
// is allowed.
const amqpDialTimeout = 5 * time.Second

// dialAMQP opens a connection and closes it again, reporting why it could not
// as an i18n key.
//
// An empty string means the tier is live. Reachability is worth proving at
// connect time rather than when a user first clicks Follow: the acceptor being
// absent is a deployment fact, and a page that offers to tail and then fails
// is worse than one that says up front what is missing.
func dialAMQP(ctx context.Context, config clientConfig) string {
	ctx, cancel := context.WithTimeout(ctx, amqpDialTimeout)
	defer cancel()

	conn, err := amqp.Dial(ctx, config.amqpURL, amqpOptions(config))
	if err != nil {
		if amqpRefusedCredentials(err) {
			return amqpForbidden
		}
		return amqpUnreachable
	}
	// Closed straight away. Holding it open would keep a session on the broker
	// for every connected profile whether or not anyone ever tails anything,
	// and the pages that use this tier open their own.
	_ = conn.Close()
	return ""
}

func amqpOptions(config clientConfig) *amqp.ConnOptions {
	options := &amqp.ConnOptions{}
	if config.amqpUser != "" {
		options.SASLType = amqp.SASLTypePlain(config.amqpUser, config.amqpPass)
	} else {
		options.SASLType = amqp.SASLTypeAnonymous()
	}
	return options
}

// amqpRefusedCredentials separates being turned away from not being heard.
//
// The two lead somewhere different - one to the profile's credentials, the
// other to the broker's acceptor list - and go-amqp reports both as a dial
// failure, so the distinction has to come from the text. SASL failures carry
// the mechanism's outcome in the message; a closed port carries the OS error.
func amqpRefusedCredentials(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "sasl") ||
		strings.Contains(message, "authentication")
}
