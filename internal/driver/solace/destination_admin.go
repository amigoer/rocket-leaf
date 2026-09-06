package solace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Creating and deleting a queue, which SEMP's configuration half spells as an
 * ordinary REST resource: a POST to the collection and a DELETE on the object.
 *
 * A create names more than a name. A Solace queue has an access type that
 * decides whether one consumer gets everything or several share it, and a
 * permission that decides what a client bound to it may do - and neither has a
 * sensible silent default, because "exclusive" and "consume" are the settings
 * an operator would pick and also the settings that quietly break a fan-out
 * design. So the form collects both and this fills in the broker's own
 * defaults only where it was given nothing.
 */

// errNoUpdate is why this driver offers no edit.
//
// Not "not implemented". A queue's settings change underneath whatever has it
// open, and the ones worth changing - its access type, its dead message queue,
// whether ingress is enabled - each have their own consequences for consumers
// already bound. This driver reads them and does not offer one control that
// writes them all, so CapDestinationUpdate is not declared and this method
// exists only because DestinationAdmin requires it.
var errNoUpdate = errors.New(
	"this driver does not alter a queue: change it in the broker's own manager, " +
		"where each setting is its own control")

// queueNameRule is the broker's, quoted from the message it refuses a bad name
// with: no *, ?, ', <, > , & or ;, and at most 200 characters.
//
// Checked here as well as by the broker because the two failures read
// differently: this one names the field, and the broker's arrives as a SEMP
// envelope in the middle of a create the user thought had worked.
var queueNameRule = regexp.MustCompile(`^[^*?'<>&;]+$`)

const maxQueueNameLength = 200

// CreateDestination declares a queue in this Message VPN.
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(spec.Ref.Name)
	if err := validQueueName(name); err != nil {
		return err
	}

	body := map[string]any{
		"queueName":      name,
		"accessType":     attributeOr(spec.Attributes, AttrAccessType, "exclusive"),
		"permission":     attributeOr(spec.Attributes, AttrPermission, "consume"),
		"ingressEnabled": attributeBool(spec.Attributes, AttrIngress, true),
		"egressEnabled":  attributeBool(spec.Attributes, AttrEgress, true),
	}
	if owner := strings.TrimSpace(spec.Attributes[AttrOwner]); owner != "" {
		body["owner"] = owner
	}
	if dmq := strings.TrimSpace(spec.Attributes[AttrDeadMsgQueue]); dmq != "" {
		body["deadMsgQueue"] = dmq
		// Without this the broker only moves a message the publisher marked
		// eligible, and nothing this app sends is marked: a queue configured
		// with a dead message queue would report one and never fill it.
		body["respectDmqEligibleEnabled"] = attributeBool(spec.Attributes, AttrRespectDmq, false)
	}
	if redelivery, ok := attributeInt(spec.Attributes, AttrMaxRedelivery); ok {
		body["maxRedeliveryCount"] = redelivery
	}
	if quota, ok := attributeInt(spec.Attributes, AttrMaxSpool); ok {
		body["maxMsgSpoolUsage"] = quota
	}

	err := c.semp.configSend(ctx, http.MethodPost, "/msgVpns/"+segment(c.vpn)+"/queues", body)
	if alreadyExists(err) {
		return fmt.Errorf("%s already has a queue named %s", c.vpn, name)
	}
	return err
}

// UpdateDestination is not offered. See errNoUpdate.
func (c *Conn) UpdateDestination(_ context.Context, _ model.DestinationSpec) error {
	return errNoUpdate
}

/*
 * RemoveDestination deletes a queue, and whatever it was holding.
 *
 * There is no guard and none is offered, because the broker offers none: SEMP
 * deletes a queue with a quarter of a million messages on it as readily as an
 * empty one, and a check made here would be this app's opinion presented as
 * the broker's. What the caller gets instead is the count on the row it
 * deleted from, and a confirmation that says so.
 */
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return errors.New("no name given")
	}

	err := c.semp.configSend(ctx, http.MethodDelete,
		"/msgVpns/"+segment(c.vpn)+"/queues/"+segment(name), nil)
	if notFound(err) {
		return fmt.Errorf("%s has no queue named %s", c.vpn, name)
	}
	return err
}

func validQueueName(name string) error {
	if name == "" {
		return errors.New("no name given")
	}
	if len(name) > maxQueueNameLength {
		return fmt.Errorf("a queue name is at most %d characters; %q is %d",
			maxQueueNameLength, name, len(name))
	}
	if !queueNameRule.MatchString(name) {
		return fmt.Errorf("a queue name cannot contain any of * ? ' < > & ; and %q does", name)
	}
	return nil
}

func attributeOr(attributes map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(attributes[key]); value != "" {
		return value
	}
	return fallback
}

func attributeBool(attributes map[string]string, key string, fallback bool) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(attributes[key]))
	if err != nil {
		return fallback
	}
	return value
}

// attributeInt reports whether the attribute was there as well as its value,
// because zero is a real setting for several of these and "leave the broker's
// default alone" is not the same request.
func attributeInt(attributes map[string]string, key string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(attributes[key]))
	if err != nil {
		return 0, false
	}
	return value, true
}
