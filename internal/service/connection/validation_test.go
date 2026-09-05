package connection

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/model"
)

func TestValidateConnectionFields(t *testing.T) {
	service := newTestService(t, nil)

	if _, _, err := service.validateConnectionFields(model.KindRocketMQ, "", "127.0.0.1:9876", 5); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, _, err := service.validateConnectionFields(model.KindRocketMQ, "prod", "", 5); err == nil {
		t.Fatal("empty NameServer should fail")
	}
	if _, _, err := service.validateConnectionFields(model.KindRocketMQ, "prod", "ns:9876", 999); err == nil {
		t.Fatal("timeout greater than 300 seconds should fail")
	}
	name, nameServer, err := service.validateConnectionFields(model.KindRocketMQ, "  prod  ", " ns:9876;ns2:9876 ", 0)
	if err != nil || name != "prod" || nameServer != "ns:9876;ns2:9876" {
		t.Fatalf("valid input was not normalized: err=%v name=%q nameServer=%q", err, name, nameServer)
	}
}

// A family that declares no endpoint field is exempt from the address check
// and from nothing else.
func TestValidateConnectionFieldsSkipsOnlyTheAddress(t *testing.T) {
	service := newHostedTestService(t, nil)

	if _, _, err := service.validateConnectionFields(hostedKind, "queues", "", 5); err != nil {
		t.Fatalf("a hosted family should save without an address: %v", err)
	}
	if _, _, err := service.validateConnectionFields(hostedKind, "  ", "", 5); err == nil {
		t.Error("a hosted family still needs a name")
	}
	if _, _, err := service.validateConnectionFields(hostedKind, "queues", "", 999); err == nil {
		t.Error("a hosted family's timeout is still bounded")
	}
	// The exemption is per family, not a switch on the service.
	if _, _, err := service.validateConnectionFields(model.KindRocketMQ, "prod", "", 5); err == nil {
		t.Error("an addressed family should still reject an empty address")
	}
}

// Every path a profile can enter by has its own address check, so a hosted
// family has to clear all four.
func TestHostedFamilyNeedsNoAddressOnAnyPath(t *testing.T) {
	service := newHostedTestService(t, nil)

	added, err := service.AddConnection(hostedProfile("queues"))
	if err != nil {
		t.Fatalf("AddConnection: %v", err)
	}
	if added.Endpoints != "" {
		t.Errorf("stored endpoints = %q; want empty", added.Endpoints)
	}

	renamed := hostedProfile("renamed")
	if _, err := service.UpdateConnection(added.ID, renamed); err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}

	// A submission that omits the kind is edited under the stored one, or the
	// profile would be validated as the default family and asked for an
	// address it has no field for.
	untyped := hostedProfile("untyped")
	untyped.Kind = ""
	if _, err := service.UpdateConnection(added.ID, untyped); err != nil {
		t.Fatalf("UpdateConnection without a kind: %v", err)
	}

	if err := service.ProbeProfile(hostedProfile("probe")); err != nil {
		t.Fatalf("ProbeProfile: %v", err)
	}

	imported := hostedProfile("imported")
	if err := service.ValidateConnections([]*model.ConnectionProfile{&imported}); err != nil {
		t.Fatalf("ValidateConnections: %v", err)
	}
	if err := service.ReplaceConnections([]*model.ConnectionProfile{&imported}); err != nil {
		t.Fatalf("ReplaceConnections: %v", err)
	}
	stored := service.GetConnections()
	if len(stored) != 1 || stored[0].Name != "imported" || stored[0].Endpoints != "" {
		t.Fatalf("imported connections = %+v; want one hosted profile with no address", stored)
	}
}

// The same service must still hold every addressed family to its address, or
// the exemption would be a hole rather than a family's own answer.
func TestAddressedFamilyStillNeedsAnAddress(t *testing.T) {
	service := newHostedTestService(t, nil)

	if _, err := service.AddConnection(profileOf("prod", "", "", 5, false, "", "", "")); err == nil {
		t.Error("AddConnection accepted a RocketMQ profile with no address")
	}
	if err := service.ProbeProfile(profileOf("prod", "", "", 5, false, "", "", "")); err == nil {
		t.Error("ProbeProfile accepted a RocketMQ profile with no address")
	}
	addressless := profileOf("prod", "", "", 5, false, "", "", "")
	if err := service.ValidateConnections([]*model.ConnectionProfile{&addressless}); err == nil {
		t.Error("ValidateConnections accepted a RocketMQ profile with no address")
	}

	added, err := service.AddConnection(profileOf("prod", "", "ns:9876", 5, false, "", "", ""))
	if err != nil {
		t.Fatalf("AddConnection: %v", err)
	}
	if _, err := service.UpdateConnection(added.ID, profileOf("prod", "", "", 5, false, "", "", "")); err == nil {
		t.Error("UpdateConnection accepted clearing a RocketMQ profile's address")
	}
}

func TestNormalizeConnectionGroupACLAndTimeout(t *testing.T) {
	if normalizeConnectionGroup("  \t ") != "" {
		t.Fatal("a blank group should normalize to ungrouped")
	}
	if normalizeConnectionGroup("  staging   cluster ") != "staging cluster" {
		t.Fatal("group whitespace should be trimmed and collapsed")
	}

	enabled, accessKey, secretKey, err := normalizeACLConfig(false, "a", "b")
	if err != nil || enabled || accessKey != "" || secretKey != "" {
		t.Fatalf("disabled ACL should clear credentials: err=%v enabled=%v accessKey=%q secretKey=%q", err, enabled, accessKey, secretKey)
	}
	if _, _, _, err := normalizeACLConfig(true, "", "sk"); err == nil {
		t.Fatal("enabled ACL without AccessKey should fail")
	}
	enabled, accessKey, secretKey, err = normalizeACLConfig(true, " ak ", " sk ")
	if err != nil || !enabled || accessKey != "ak" || secretKey != "sk" {
		t.Fatalf("ACL normalization failed: err=%v enabled=%v accessKey=%q secretKey=%q", err, enabled, accessKey, secretKey)
	}
}
