package connection

import (
	"fmt"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

// hasEndpoint reports whether raw carries at least one address.
//
// The delimiters are the endpoint-list convention every family shares -
// RocketMQ name servers, Kafka bootstrap servers - and emptiness is all this
// check needs. Anything more specific is the driver's job, which is why this
// no longer goes through the RocketMQ parser.
func hasEndpoint(raw string) bool {
	return len(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})) > 0
}

// normalizeConnectionGroup trims a group label and collapses inner whitespace.
// The label is free-form, so an empty result is valid and means "ungrouped".
func normalizeConnectionGroup(group string) string {
	return strings.Join(strings.Fields(group), " ")
}

func normalizeACLConfig(enableACL bool, accessKey, secretKey string) (bool, string, string, error) {
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	if !enableACL {
		return false, "", "", nil
	}
	if accessKey == "" {
		return false, "", "", fmt.Errorf("AccessKey is required when ACL is enabled")
	}
	if secretKey == "" {
		return false, "", "", fmt.Errorf("SecretKey is required when ACL is enabled")
	}
	return true, accessKey, secretKey, nil
}

// requiresEndpoints asks the family whether it has an address at all.
//
// A service built without a policy keeps the rule every family had before one
// could decline: an address is required.
func (s *Service) requiresEndpoints(kind model.MQKind) bool {
	if s.endpoints == nil {
		return true
	}
	return s.endpoints.RequiresEndpoints(kind)
}

// validateConnectionFields checks what every family's form has in common.
//
// The address is named for the family on the form - NameServers, a management
// URL, bootstrap servers - so the message when it is empty must not be. It
// used to say NameServer, which sent a Kafka or RabbitMQ user looking for a
// field their form does not have.
//
// Whether there is an address to name is the family's own answer: a hosted
// one is reached by a region and a credential, so demanding one would make it
// unsaveable. Nothing else relaxes - a connection still needs a name, and the
// timeout is still bounded.
func (s *Service) validateConnectionFields(
	kind model.MQKind, name, endpoints string, timeoutSec int,
) (string, string, error) {
	name = strings.TrimSpace(name)
	endpoints = strings.TrimSpace(endpoints)
	if name == "" {
		return "", "", fmt.Errorf("connection name cannot be empty")
	}
	if s.requiresEndpoints(kind) && !hasEndpoint(endpoints) {
		return "", "", fmt.Errorf("connection address cannot be empty")
	}
	if timeoutSec < 0 || timeoutSec > 300 {
		return "", "", fmt.Errorf("connection timeout must be between 1 and 300 seconds")
	}
	return name, endpoints, nil
}
