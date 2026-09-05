package nsq

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

// Creating and removing topics.
//
// Both are cluster-wide, because a topic is not: it exists on the nsqd it was
// created on and nowhere else, so creating it on one daemon would leave a
// producer that connected to another either failing or auto-creating it later
// with none of the channels.

// errNoUpdate is what UpdateDestination reports.
//
// The method exists because DestinationAdmin is one interface; the capability
// is not declared, so nothing in the UI reaches this. See the reason in
// conformance_test.go.
var errNoUpdate = errors.New("an nsq topic has no configuration to change; its name is the whole of it")

// CreateDestination declares a topic on every nsqd in the connection.
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	if spec.Ref.Name == "" {
		return errors.New("a topic needs a name")
	}
	_, err := eachNode(ctx, c.nodes, func(ctx context.Context, n node) (struct{}, error) {
		return struct{}{}, c.client.post(ctx, n.address, "/topic/create",
			url.Values{"topic": {spec.Ref.Name}}, nil, nil)
	})
	return err
}

// UpdateDestination is not offered. See errNoUpdate.
func (c *Conn) UpdateDestination(_ context.Context, _ model.DestinationSpec) error {
	return errNoUpdate
}

/*
 * RemoveDestination deletes a topic and everything under it.
 *
 * Two tiers, and both are required for the delete to hold. nsqd forgets the
 * topic; nsqlookupd does not, because a producer's registration is separate
 * from the topic's existence - so a delete that only reached nsqd leaves the
 * name in nsqlookupd's /topics, where nsqadmin still lists it and a consumer
 * looking it up still finds it. Confirmed against 1.3.0: after deleting on
 * nsqd alone, /topics still names the topic and /lookup answers with an empty
 * producer list rather than a 404.
 *
 * The discovery tier goes first, so nothing can re-register between the two
 * calls.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	if ref.Name == "" {
		return errors.New("a topic needs a name")
	}

	query := url.Values{"topic": {ref.Name}}
	if err := c.forgetAtLookupd(ctx, "/topic/delete", query); err != nil {
		return err
	}
	return c.onEveryCarrier(ctx, "/topic/delete", query,
		fmt.Sprintf("no nsqd in this connection was carrying the topic %q", ref.Name))
}

// forgetAtLookupd runs a delete against every configured nsqlookupd.
//
// An object the discovery tier has never heard of is not an error: a topic
// created while the tier was down is registered nowhere, and refusing to
// delete it would leave it undeletable.
func (c *Conn) forgetAtLookupd(ctx context.Context, path string, query url.Values) error {
	group, groupCtx := errgroup.WithContext(ctx)
	for _, address := range c.config.lookupd {
		group.Go(func() error {
			err := c.client.post(groupCtx, address, path, query, nil, nil)
			if err != nil && !notFound(err) {
				return fmt.Errorf("%s: %w", hostPort(address), err)
			}
			return nil
		})
	}
	return group.Wait()
}
