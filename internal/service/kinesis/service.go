// Package kinesis orchestrates the operations only Amazon Kinesis has.
//
// It exists beside the canonical services rather than inside them because the
// vocabulary is this family's own. A stream is created with a capacity mode, a
// shard count that only one of the two modes uses, and a retention the service
// enforces by discarding - where TopicService.Create collects a broker
// address, two queue counts and a permission string, which is RocketMQ's
// vocabulary and none of which a stream has.
//
// The canonical services still serve Kinesis everything they can express: a
// stream is a destination, and listing and describing one go through them.
// Nothing here duplicates that.
package kinesis

import (
	"context"
	"fmt"
	"time"

	"github.com/amigoer/mq-studio/internal/driver"
	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/model"
)

// Settings exposes only what these operations need.
type Settings interface {
	GetRequestTimeout() time.Duration
}

// ConnSource yields the connection a request runs against.
type ConnSource func(connID int) (driver.Conn, error)

// Service is the orchestration layer between the bridge and the driver.
type Service struct {
	conns    ConnSource
	settings Settings
}

// New creates the service.
func New(conns ConnSource, settings Settings) *Service {
	return &Service{conns: conns, settings: settings}
}

func (s *Service) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.settings.GetRequestTimeout())
}

// kinesisConn resolves the connection and asserts it is this family's.
//
// The capability is checked first wherever there is one, because the reason a
// page gets back should name the capability rather than a Go type it has never
// heard of. A profile of another family reaching these methods is a bug in the
// renderer rather than an unsupported operation, and the error says so.
func (s *Service) kinesisConn(connID int, capability model.Capability) (*kinesisdriver.Conn, error) {
	conn, err := s.conns(connID)
	if err != nil {
		return nil, err
	}
	if capability != "" && !conn.Capabilities().Has(capability) {
		return nil, driver.Unsupported(conn, capability)
	}
	api, ok := conn.(*kinesisdriver.Conn)
	if !ok {
		return nil, fmt.Errorf("connection %d is %s, not kinesis", connID, conn.Kind())
	}
	return api, nil
}

// CreateStream declares a stream.
//
// Beside the canonical create rather than through it, for the reason the
// package comment gives: a stream's form is a capacity mode and a retention,
// and TopicService.Create has nowhere to put either.
func (s *Service) CreateStream(ctx context.Context, connID int, spec kinesisdriver.StreamSpec) error {
	api, err := s.kinesisConn(connID, model.CapDestinationCreate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.CreateStream(ctx, spec)
}

// UpdateStream changes a stream's capacity mode, shard count or retention.
//
// Each of the three is a separate asynchronous operation on the service, and
// each is refused while the stream is settling from the last one - so this can
// take longer than a request that changes one thing, and the driver waits
// between them rather than firing and hoping.
func (s *Service) UpdateStream(ctx context.Context, connID int, spec kinesisdriver.StreamSpec) error {
	api, err := s.kinesisConn(connID, model.CapDestinationUpdate)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.UpdateStream(ctx, spec)
}

// RemoveStream deletes a stream and every record in it. There is no undo, and
// a stream with registered consumers refuses until they are deregistered.
func (s *Service) RemoveStream(ctx context.Context, connID int, name string) error {
	api, err := s.kinesisConn(connID, model.CapDestinationDelete)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return api.RemoveDestination(ctx, model.DestinationRef{Name: name})
}
