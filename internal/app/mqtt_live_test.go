package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	mqttdriver "github.com/amigoer/mq-studio/internal/driver/mqtt"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	mqttservice "github.com/amigoer/mq-studio/internal/service/mqtt"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The MQTT stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. This holds nothing: it stores a
 * profile, dials it, and then asks the service layer the way a page does -
 * with an integer. What that covers and the driver's tests do not is the
 * chain in between, where the two failures this project has actually had both
 * lived: a credential that did not survive being written to disk and read
 * back, and a capability the service checks before the type assertion.
 */

const (
	liveMosquitto  = "127.0.0.1:1883"
	liveEMQX       = "127.0.0.1:1884"
	liveEMQXAPI    = "http://127.0.0.1:18083"
	liveEMQXKey    = "mqstudio-e2e"
	liveEMQXSecret = "mqstudio-e2e-secret-key"
)

func requireLiveMosquitto(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Name:   "mosquitto",
		Family: e2e.MQTT,
		Start:  "npm run e2e:mqtt:up",
		Probe:  e2e.DialTCP(liveMosquitto),
	})
}

func requireLiveEMQX(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Name:   "emqx",
		Family: e2e.MQTT,
		Start:  "npm run e2e:mqtt:emqx:up",
		Probe:  e2e.HTTPGet(liveEMQXAPI + "/api/v5/status"),
	})
}

// liveMqttStack is the connection service, the MQTT service and the registry
// they share, on a config directory of this test's own.
func liveMqttStack(t *testing.T) (*connection.Service, *mqttservice.Service, *driver.Registry) {
	t.Helper()
	if _, ok := driver.Lookup(model.KindMQTT); !ok {
		driver.Register(mqttdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	connections := connection.New(
		paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints())
	return connections, mqttservice.New(newConnSource(registry), settingsService), registry
}

/*
 * The whole path in one go: store a profile, dial it, publish and read back
 * through the id a page would pass, then close it.
 *
 * Publishing and receiving on the one connection is the point rather than a
 * shortcut. It is what the send console and the subscribe workbench are - two
 * pages on one profile - and a 5.0 subscription with NoLocal set would never
 * deliver it, while 3.1.1 has no such option and always would. This is the
 * test that keeps the two versions behaving alike.
 */
func TestLiveMqttThroughAConnectionID(t *testing.T) {
	requireLiveMosquitto(t)
	connections, mqtt, _ := liveMqttStack(t)

	created, err := connections.AddConnection(model.ConnectionProfile{
		Name:       "mqtt-live",
		Kind:       model.KindMQTT,
		Endpoints:  liveMosquitto,
		TimeoutSec: 10,
	})
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = connections.Disconnect(created.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := "mqs-test/app/" + t.Name()
	subscription, err := mqtt.StartSubscription(ctx, created.ID, model.LiveSubscriptionSpec{
		Filters: []model.LiveFilter{{Pattern: topic}},
	})
	if err != nil {
		t.Fatalf("start subscription: %v", err)
	}
	t.Cleanup(func() { _ = mqtt.StopSubscription(context.Background(), created.ID, subscription.ID) })

	if _, err := mqtt.Publish(ctx, created.ID, mqttdriver.PublishRequest{
		Topic:   topic,
		Payload: "through-the-service",
		QoS:     1,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		batch, err := mqtt.PollSubscription(ctx, created.ID, subscription.ID, 0, 0)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if len(batch.Messages) > 0 {
			if batch.Messages[0].Body != "through-the-service" {
				t.Errorf("received %+v", batch.Messages[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("nothing arrived through the service layer")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

/*
 * A capability the endpoint does not have has to fail at the service, with the
 * capability named.
 *
 * The service checks the declared capability before the type assertion, which
 * matters here more than anywhere else: the MQTT driver does implement
 * ClientInspector, so a bare type assertion would succeed and the failure
 * would arrive from the broker as a confusing HTTP error instead of as "this
 * connection cannot do that".
 */
func TestLiveMqttRefusesTheManagementTierWithoutOne(t *testing.T) {
	requireLiveMosquitto(t)
	connections, mqtt, _ := liveMqttStack(t)

	created, err := connections.AddConnection(model.ConnectionProfile{
		Name:       "mqtt-plain",
		Kind:       model.KindMQTT,
		Endpoints:  liveMosquitto,
		TimeoutSec: 10,
	})
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = connections.Disconnect(created.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err = mqtt.Clients(ctx, created.ID)
	if err == nil {
		t.Fatal("a broker with no management api listed clients anyway")
	}
	var unsupported *driver.UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want an UnsupportedError naming the capability", err)
	}
	if unsupported.Capability != model.CapClientInspect {
		t.Errorf("capability = %q, want %q", unsupported.Capability, model.CapClientInspect)
	}
}

/*
 * The credential round trip, which is the failure this project has already
 * had once.
 *
 * internal/service/connection used to assume a connection's only credentials
 * were RocketMQ's access key pair: saving dropped every other secret and
 * forced the mechanism to none, and the connection form's test button probed
 * the submitted profile rather than the stored one, so it passed on the way in
 * and the connection failed afterwards.
 *
 * MQTT is the first family with two independent credentials, so it is the
 * first that can lose one and keep the other. This stores both, reloads the
 * service from disk, and dials what came back.
 */
func TestLiveMqttCredentialsSurviveDisk(t *testing.T) {
	requireLiveEMQX(t)
	connections, mqtt, _ := liveMqttStack(t)

	created, err := connections.AddConnection(model.ConnectionProfile{
		Name:       "emqx-live",
		Kind:       model.KindMQTT,
		Endpoints:  liveEMQX,
		TimeoutSec: 10,
		Options:    map[string]string{mqttdriver.OptionManagementURL: liveEMQXAPI},
		Secrets: map[string]string{
			mqttdriver.SecretManagementKey:  liveEMQXKey,
			mqttdriver.SecretManagementSalt: liveEMQXSecret,
		},
	})
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	// Everything above could pass against a service that never wrote the
	// secrets. Reading them back off disk is the assertion.
	stored, err := connections.GetConnection(created.ID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if stored.Secret(mqttdriver.SecretManagementKey) != liveEMQXKey {
		t.Errorf("the management api key did not survive being stored")
	}
	if stored.Secret(mqttdriver.SecretManagementSalt) != liveEMQXSecret {
		t.Errorf("the management secret did not survive being stored")
	}

	if err := connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = connections.Disconnect(created.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// And the proof that the stored credential is the one that was used: a
	// wrong key would have degraded the capability and this call would refuse.
	clients, err := mqtt.Clients(ctx, created.ID)
	if err != nil {
		t.Fatalf("clients through the stored credential: %v", err)
	}
	var self bool
	for _, client := range clients {
		if strings.HasPrefix(client.Name, "mq-studio") {
			self = true
			break
		}
	}
	if !self {
		t.Errorf("this connection is not in the broker's own list of %d clients", len(clients))
	}
}
