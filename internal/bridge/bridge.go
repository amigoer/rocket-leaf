// Package bridge exposes the business services to the frontend as Wails services.
//
// Every exported method on a bridge service becomes a callable binding in the
// renderer. The bridge is the only place allowed to reshape business data for
// the UI: it redacts secrets that must never leave the Go process and resolves
// the credential modes the settings and connection forms rely on.
package bridge

import (
	"github.com/amigoer/mq-studio/internal/app"
	"github.com/amigoer/mq-studio/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Services returns every bridge service in registration order.
//
// The shell service and the update manager are passed in rather than built
// here. Both outlive a single call: the shell's consumer is the system tray,
// which cannot exist until the application is running, and the updater keeps a
// background schedule the caller has to be able to stop.
func Services(
	services *app.Services,
	version string,
	shell *ShellService,
	updates *update.Manager,
) []application.Service {
	return []application.Service{
		application.NewService(&SystemService{settings: services.Settings, version: version}),
		application.NewService(NewUpdateService(updates)),
		application.NewService(&WindowService{}),
		application.NewService(shell),
		application.NewService(&ConnectionService{service: services.Connections, scopes: services.Scopes}),
		application.NewService(&SettingsService{service: services.Settings}),
		application.NewService(&ClusterService{service: services.Cluster}),
		application.NewService(&TopicService{service: services.Topics}),
		application.NewService(&ConsumerService{service: services.Consumers}),
		application.NewService(&MessageService{service: services.Messages}),
		application.NewService(&ACLService{service: services.ACL}),
		application.NewService(&DriverService{conns: services.Conns}),
		application.NewService(&RoutingService{service: services.Routing}),
		application.NewService(&RabbitMQService{service: services.RabbitMQ}),
		application.NewService(&KafkaService{service: services.Kafka}),
		application.NewService(&MQTTService{service: services.MQTT}),
		application.NewService(&PulsarService{service: services.Pulsar}),
		application.NewService(&RedisStreamService{service: services.RedisStream}),
		application.NewService(&NATSService{service: services.NATS}),
		application.NewService(&ActiveMQService{service: services.ActiveMQ}),
		application.NewService(&NSQService{service: services.NSQ}),
		application.NewService(&SQSService{service: services.SQS}),
		application.NewService(&GooglePubSubService{service: services.GooglePubSub}),
		application.NewService(&AzureServiceBusService{service: services.ServiceBus}),
		application.NewService(&KinesisService{service: services.Kinesis}),
		application.NewService(&IBMMQService{service: services.IBMMQ}),
		application.NewService(&SolaceService{service: services.Solace}),
		// Wails' own service, registered rather than wrapped: it is a platform
		// capability like WindowService, not business data to reshape. On macOS
		// it only delivers from a packaged, signed bundle -- the renderer falls
		// back to the Web API when a call fails, so dev builds still notify.
		application.NewService(notifications.New()),
	}
}
