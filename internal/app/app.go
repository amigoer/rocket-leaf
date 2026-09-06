// Package app assembles the business services and manages their lifecycle.
package app

import (
	"fmt"
	"log"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/driver/activemq"
	azureservicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
	googlepubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	"github.com/amigoer/mq-studio/internal/driver/kafka"
	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/driver/mqtt"
	natsdriver "github.com/amigoer/mq-studio/internal/driver/nats"
	nsqdriver "github.com/amigoer/mq-studio/internal/driver/nsq"
	"github.com/amigoer/mq-studio/internal/driver/pulsar"
	"github.com/amigoer/mq-studio/internal/driver/rabbitmq"
	"github.com/amigoer/mq-studio/internal/driver/redisstream"
	"github.com/amigoer/mq-studio/internal/driver/rocketmq"
	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
	"github.com/amigoer/mq-studio/internal/service/access"
	activemqservice "github.com/amigoer/mq-studio/internal/service/activemq"
	azureservicebusservice "github.com/amigoer/mq-studio/internal/service/azureservicebus"
	"github.com/amigoer/mq-studio/internal/service/cluster"
	"github.com/amigoer/mq-studio/internal/service/collector"
	"github.com/amigoer/mq-studio/internal/service/configuration"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	googlepubsubservice "github.com/amigoer/mq-studio/internal/service/googlepubsub"
	kafkaservice "github.com/amigoer/mq-studio/internal/service/kafka"
	"github.com/amigoer/mq-studio/internal/service/message"
	mqttservice "github.com/amigoer/mq-studio/internal/service/mqtt"
	natsservice "github.com/amigoer/mq-studio/internal/service/nats"
	nsqservice "github.com/amigoer/mq-studio/internal/service/nsq"
	pulsarservice "github.com/amigoer/mq-studio/internal/service/pulsar"
	rabbitmqservice "github.com/amigoer/mq-studio/internal/service/rabbitmq"
	redisstreamservice "github.com/amigoer/mq-studio/internal/service/redisstream"
	"github.com/amigoer/mq-studio/internal/service/routing"
	"github.com/amigoer/mq-studio/internal/service/scope"
	"github.com/amigoer/mq-studio/internal/service/settings"
	sqsservice "github.com/amigoer/mq-studio/internal/service/sqs"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

// Services aggregates business services required by the HTTP transport layer.
type Services struct {
	Connections  *connection.Service
	Cluster      *cluster.Service
	Topics       *destination.Service
	Consumers    *subscription.Service
	Messages     *message.Service
	Settings     *configuration.Service
	ACL          *access.Service
	Routing      *routing.Service
	Scopes       *scope.Service
	RabbitMQ     *rabbitmqservice.Service
	Kafka        *kafkaservice.Service
	MQTT         *mqttservice.Service
	Pulsar       *pulsarservice.Service
	RedisStream  *redisstreamservice.Service
	NATS         *natsservice.Service
	ActiveMQ     *activemqservice.Service
	NSQ          *nsqservice.Service
	SQS          *sqsservice.Service
	GooglePubSub *googlepubsubservice.Service
	ServiceBus   *azureservicebusservice.Service

	// Conns resolves a profile id to a live connection. The bridge needs it to
	// answer capability questions without going through a domain service.
	Conns func(connID int) (driver.Conn, error)

	// Collector keeps the TPS history filling in while the window is hidden.
	Collector *collector.Collector

	// registry owns every open connection, one per connected profile.
	registry *driver.Registry
}

// New initializes the local encryption key and assembles all business services.
func New() (*Services, error) {
	paths, err := layout.Default()
	if err != nil {
		return nil, err
	}
	if err := crypto.InitKey(paths.Directory); err != nil {
		return nil, fmt.Errorf("failed to initialize local encryption key: %w", err)
	}

	// Register the compiled-in drivers before anything asks the catalog what
	// families exist, or opens a connection against one.
	driver.Register(rocketmq.New())
	driver.Register(rabbitmq.New())
	driver.Register(kafka.New())
	driver.Register(mqtt.New())
	driver.Register(pulsar.New())
	driver.Register(redisstream.New())
	driver.Register(natsdriver.New())
	driver.Register(activemq.New())
	driver.Register(nsqdriver.New())
	driver.Register(sqsdriver.New())
	driver.Register(googlepubsubdriver.New())
	driver.Register(azureservicebusdriver.New())
	driver.Register(kinesisdriver.New())

	registry := driver.NewRegistry()
	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints())
	configurationService := configuration.New(paths, settingsService, connections)
	conns := newConnSource(registry)
	clusterService := cluster.New(paths.TPSHistoryFile, conns, settingsService)
	services := &Services{
		Connections:  connections,
		Cluster:      clusterService,
		Topics:       destination.New(conns, settingsService),
		Consumers:    subscription.New(conns, settingsService),
		Messages:     message.New(conns, settingsService),
		Settings:     configurationService,
		ACL:          access.New(conns, settingsService),
		Routing:      routing.New(conns, settingsService),
		Scopes:       scope.New(conns, settingsService),
		RabbitMQ:     rabbitmqservice.New(conns, settingsService),
		Kafka:        kafkaservice.New(conns, settingsService),
		MQTT:         mqttservice.New(conns, settingsService),
		Pulsar:       pulsarservice.New(conns, settingsService),
		RedisStream:  redisstreamservice.New(conns, settingsService),
		NATS:         natsservice.New(conns, settingsService),
		ActiveMQ:     activemqservice.New(conns, settingsService),
		NSQ:          nsqservice.New(conns, settingsService),
		SQS:          sqsservice.New(conns, settingsService),
		GooglePubSub: googlepubsubservice.New(conns, settingsService),
		ServiceBus:   azureservicebusservice.New(conns, settingsService),
		Conns:        conns,
		Collector:    collector.New(sampleActiveConnection(clusterService, registry), registry.HasActive),
		registry:     registry,
	}
	services.Collector.Start()

	// Reopening the last connection used to happen lazily, on whichever data
	// request first found no client. The registry never dials on its own, so
	// the reconnect is explicit - in the background, because a NameServer that
	// is down would otherwise hold the window shut for the dial timeout.
	// ConnectDefault is a no-op when the user has turned auto-connect off.
	go func() {
		if err := connections.ConnectDefault(); err != nil {
			log.Printf("[app] 自动连接默认连接失败: %v", err)
		}
	}()
	return services, nil
}

// Close stops background sampling and releases every open connection.
func (s *Services) Close() {
	if s.Collector != nil {
		s.Collector.Stop()
	}
	if s.registry != nil {
		s.registry.CloseAll()
	}
}
