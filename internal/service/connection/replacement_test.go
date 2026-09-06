package connection

import (
	"bytes"
	"os"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

func TestReplaceConnectionsNormalizesEncryptsAndReloads(t *testing.T) {
	service := newTestService(t, nil)
	connections := []*model.ConnectionProfile{
		{
			ID:         7,
			Name:       "  primary  ",
			Group:      "  staging   cluster ",
			Endpoints:  " ns-a:9876 ",
			TimeoutSec: 0,
			Auth:       model.AuthConfig{Mechanism: model.AuthNone},
			Secrets: map[string]string{
				model.SecretAccessKey: "discarded",
				model.SecretSecretKey: "discarded",
			},
			Status:    model.StatusOnline,
			LastCheck: "old",
			IsDefault: true,
		},
		{
			ID:         7,
			Name:       "secured",
			Group:      "production",
			Endpoints:  "ns-b:9876",
			TimeoutSec: 8,
			Auth:       model.AuthConfig{Mechanism: model.AuthACL},
			Secrets: map[string]string{
				model.SecretAccessKey: "portable-ak",
				model.SecretSecretKey: "portable-sk",
			},
			IsDefault: true,
		},
		{
			Name:       "third",
			Endpoints:  "ns-c:9876",
			TimeoutSec: 5,
		},
	}
	if err := service.ReplaceConnections(connections); err != nil {
		t.Fatal(err)
	}

	got := service.GetConnections()
	if len(got) != 3 || got[0].ID != 7 || got[1].ID != 8 || got[2].ID != 9 {
		t.Fatalf("IDs were not normalized deterministically: %#v", got)
	}
	// A blank timeout stays blank, which is what lets the application setting
	// reach the dial; stamping a default here is what used to shadow it.
	if got[0].Name != "primary" || got[0].Endpoints != "ns-a:9876" || got[0].Group != "staging cluster" || got[0].TimeoutSec != 0 {
		t.Fatalf("first connection was not normalized: %#v", got[0])
	}
	if got[0].Status != model.StatusOffline || got[0].LastCheck != "-" || got[0].Secret(model.SecretAccessKey) != "" || got[0].Secret(model.SecretSecretKey) != "" {
		t.Fatalf("runtime state or disabled ACL was not normalized: %#v", got[0])
	}
	defaultCount := 0
	for _, connection := range got {
		if connection.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 || !got[0].IsDefault {
		t.Fatalf("expected one deterministic default: %#v", got)
	}
	if got[1].Secret(model.SecretAccessKey) != "portable-ak" || got[1].Secret(model.SecretSecretKey) != "portable-sk" {
		t.Fatalf("credentials were not restored after reload: %#v", got[1])
	}

	diskData, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(diskData, []byte("portable-ak")) || bytes.Contains(diskData, []byte("portable-sk")) {
		t.Fatal("replacement file contains plaintext credentials")
	}
}

func TestReplaceConnectionsRejectsInvalidInputWithoutChangingState(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("existing", "test", "ns:9876", 5, false, "", "", "")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}
	err = service.ReplaceConnections([]*model.ConnectionProfile{{
		Name:       "invalid",
		Endpoints:  "ns:9876",
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthACL},
		Secrets: map[string]string{
			model.SecretSecretKey: "missing-access-key",
		},
	}})
	if err == nil {
		t.Fatal("invalid replacement should fail")
	}
	after, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("invalid replacement changed the persisted file")
	}
	got := service.GetConnections()
	if len(got) != 1 || got[0].Name != "existing" {
		t.Fatalf("invalid replacement changed in-memory state: %#v", got)
	}
}

func TestValidateConnectionsHasNoSideEffects(t *testing.T) {
	service := newTestService(t, nil)
	connection, err := service.AddConnection(profileOf("existing", "test", "ns:9876", 5, false, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.connections[connection.ID].Status = model.StatusOnline
	service.mu.Unlock()
	before, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}

	err = service.ValidateConnections([]*model.ConnectionProfile{{
		Name:       "invalid",
		Endpoints:  "ns:9876",
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthACL},
		Secrets: map[string]string{
			model.SecretSecretKey: "missing-access-key",
		},
	}})
	if err == nil {
		t.Fatal("invalid replacement should fail validation")
	}
	after, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("validation changed the persisted connection file")
	}
	got, err := service.GetConnection(connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusOnline {
		t.Fatalf("validation changed runtime status: %s", got.Status)
	}
}

func TestReplaceConnectionsAllowsEmptyList(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("existing", "test", "ns:9876", 5, false, "", "", "")); err != nil {
		t.Fatal(err)
	}
	if err := service.ReplaceConnections(nil); err != nil {
		t.Fatal(err)
	}
	if got := service.GetConnections(); len(got) != 0 {
		t.Fatalf("connections were not cleared: %#v", got)
	}
}

func TestReplaceConnectionsDoesNotWriteOutsideMutationLock(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("baseline", "test", "baseline:9876", 5, false, "", "", "")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}

	service.mu.Lock()
	locked := true
	defer func() {
		if locked {
			service.mu.Unlock()
		}
	}()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- service.ReplaceConnections([]*model.ConnectionProfile{{
			Name:       "replacement",
			Endpoints:  "replacement:9876",
			TimeoutSec: 5,
		}})
	}()
	<-started
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		current, readErr := os.ReadFile(service.dataFilePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(before, current) {
			t.Fatal("replacement wrote the file without holding the mutation lock")
		}
		time.Sleep(time.Millisecond)
	}
	service.mu.Unlock()
	locked = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not complete after releasing the mutation lock")
	}
}

func TestConcurrentReplaceAndAddRemainLinearizable(t *testing.T) {
	for iteration := 0; iteration < 24; iteration++ {
		service := newTestService(t, nil)
		if _, err := service.AddConnection(profileOf("baseline", "test", "baseline:9876", 5, false, "", "", "")); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		replaceDone := make(chan error, 1)
		addDone := make(chan error, 1)
		go func() {
			<-start
			replaceDone <- service.ReplaceConnections([]*model.ConnectionProfile{{
				Name:       "replacement",
				Endpoints:  "replacement:9876",
				TimeoutSec: 5,
			}})
		}()
		go func() {
			<-start
			_, err := service.AddConnection(profileOf("added", "test", "added:9876", 5, false, "", "", ""))
			addDone <- err
		}()
		close(start)
		if err := <-replaceDone; err != nil {
			t.Fatal(err)
		}
		if err := <-addDone; err != nil {
			t.Fatal(err)
		}

		inMemory := connectionNames(service.GetConnections())
		persisted := connectionNames(New(service.dataFilePath, fakeSettings{connectTimeout: 3 * time.Second, autoConnect: true}, noopRuntime{}, addressedEndpoints{}).GetConnections())
		if !reflect.DeepEqual(inMemory, persisted) {
			t.Fatalf("iteration %d: memory=%v disk=%v", iteration, inMemory, persisted)
		}
		if reflect.DeepEqual(inMemory, []string{"added", "baseline"}) || !containsName(inMemory, "replacement") {
			t.Fatalf("iteration %d: non-linearizable final state %v", iteration, inMemory)
		}
	}
}

func connectionNames(connections []*model.ConnectionProfile) []string {
	names := make([]string, 0, len(connections))
	for _, connection := range connections {
		names = append(names, connection.Name)
	}
	sort.Strings(names)
	return names
}

func containsName(names []string, wanted string) bool {
	for _, name := range names {
		if name == wanted {
			return true
		}
	}
	return false
}
