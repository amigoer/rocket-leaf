// Command e2e-azure-servicebus-seed fills the Service Bus emulator with a
// topology worth looking at.
//
// The live tests do not need it: each one creates what it needs and removes it
// again, which is what keeps them independent. This is for the other half of
// verification - the cross-check, which compares figures the app computes
// against the same figures read straight out of the Service Bus API, and
// opening the app to see whether the boards say true things. Comparing zero
// against zero proves nothing.
//
// It is Go rather than the shell-and-python every other seed here is written
// in, and that is the emulator's doing. Service Bus accepts a message over
// AMQP 1.0 and nothing else - its REST surface answers "the specified
// MessagingOperation Send is not recognized" - so a seed that publishes has to
// speak AMQP, and doing that in shell would mean implementing a protocol.
//
// It deliberately shares no code with internal/driver/azureservicebus: it
// talks to the Azure SDK directly, which is the same independence the
// cross-check needs.
//
// The entities themselves are declared in tests/e2e/azure-servicebus/config.json
// and created by the emulator at startup, because the emulator reads its
// topology from that file. One is not: the emulator refuses a config naming a
// topic with no subscriptions, so mqs-seed-orphaned is created here through
// the management API - which is also what proves that path works at all.
//
// Safe to re-run: every seeded entity is drained first, so the counts below
// are what the namespace holds afterwards rather than what has accumulated.
//
// The shape it builds is chosen for the five things one empty queue cannot
// show:
//
//   - mqs-seed-orders is an ordinary queue holding messages, one of them
//     scheduled for later - a state no consumer would be offered and a peek
//     still reaches.
//   - mqs-seed-failures has its messages dead-lettered on purpose, so the dead
//     letter board reads a real $DeadLetterQueue rather than an empty one.
//   - mqs-seed-events fans out to three subscriptions with three different
//     rules, so the routing board has a SQL filter, a correlation filter and
//     the $Default that matches everything - and each subscription holds a
//     different number of messages because of them.
//   - mqs-seed-orphaned is a topic with no subscription at all. Every message
//     sent to it is discarded on the spot, which is the fault this family
//     alerts on.
//   - mqs-seed-quiet is an ordinary queue with nothing in it, so a board has a
//     genuinely empty one to draw beside the busy ones.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
)

// The emulator, as tests/e2e/azure-servicebus/compose.yaml publishes it. Both
// are overridable for a run against a real namespace, which is the only other
// way to seed one.
var (
	amqpHost       = env("SERVICEBUS_AMQP_HOST", "localhost:5672")
	managementHost = env("SERVICEBUS_MANAGEMENT_HOST", "127.0.0.1:5300")
	keyName        = env("SERVICEBUS_KEY_NAME", "RootManageSharedAccessKey")
	key            = env("SERVICEBUS_KEY", "SAS_KEY_VALUE")
)

// The entities config.json declares, and the one this seed creates.
const (
	orders   = "mqs-seed-orders"
	failures = "mqs-seed-failures"
	quiet    = "mqs-seed-quiet"
	events   = "mqs-seed-events"
	orphaned = "mqs-seed-orphaned"

	subAll    = "mqs-seed-events-all"
	subRed    = "mqs-seed-events-red"
	subOrders = "mqs-seed-events-orders"
)

func main() {
	if err := seed(); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
}

func seed() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	connectionString := fmt.Sprintf(
		"Endpoint=sb://%s;SharedAccessKeyName=%s;SharedAccessKey=%s;UseDevelopmentEmulator=true",
		amqpHost, keyName, key)

	management, err := admin.NewClientFromConnectionString(connectionString, &admin.ClientOptions{
		ClientOptions: policy.ClientOptions{Transport: rewrite{host: managementHost}},
	})
	if err != nil {
		return fmt.Errorf("building the management client: %w", err)
	}
	if _, err := management.GetNamespaceProperties(ctx, nil); err != nil {
		return fmt.Errorf("%s is not answering; start it with: npm run e2e:azure-servicebus:up (%w)",
			managementHost, err)
	}

	data, err := azservicebus.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return fmt.Errorf("building the data client: %w", err)
	}
	defer func() { _ = data.Close(ctx) }()

	// The emulator created these from config.json. Checking rather than
	// assuming, because a config that failed validation leaves the emulator
	// running with an older topology and every count below would then be
	// seeded into the wrong place.
	fmt.Println("==> what the emulator started with")
	for _, name := range []string{orders, failures, quiet} {
		if _, err := management.GetQueue(ctx, name, nil); err != nil {
			return fmt.Errorf("queue %s is missing; the emulator did not read config.json: %w", name, err)
		}
		fmt.Printf("    queue %s\n", name)
	}
	for _, name := range []string{subAll, subRed, subOrders} {
		if _, err := management.GetSubscription(ctx, events, name, nil); err != nil {
			return fmt.Errorf("subscription %s/%s is missing: %w", events, name, err)
		}
		rules, err := listRules(ctx, management, events, name)
		if err != nil {
			return err
		}
		fmt.Printf("    subscription %s/%s · rules %s\n", events, name, strings.Join(rules, ", "))
	}

	// The emulator refuses a config declaring a topic with no subscriptions,
	// so the one topic this family alerts on has to be made at runtime.
	fmt.Println("==> creating what the emulator's config cannot declare")
	if _, err := management.DeleteTopic(ctx, orphaned, nil); err != nil && !gone(err) {
		return fmt.Errorf("removing %s from a previous run: %w", orphaned, err)
	}
	if _, err := management.CreateTopic(ctx, orphaned, nil); err != nil {
		return fmt.Errorf("creating %s: %w", orphaned, err)
	}
	fmt.Printf("    topic %s · no subscription, so every send is discarded\n", orphaned)

	fmt.Println("==> removing anything left from a previous run")
	for _, entity := range []string{orders, failures, quiet} {
		if err := drainQueue(ctx, data, entity); err != nil {
			return err
		}
	}
	for _, name := range []string{subAll, subRed, subOrders} {
		if err := drainSubscription(ctx, data, events, name); err != nil {
			return err
		}
	}

	fmt.Printf("==> %s: 12 messages and one scheduled for an hour from now\n", orders)
	if err := send(ctx, data, orders, 12, "order", nil); err != nil {
		return err
	}
	if err := schedule(ctx, data, orders, "held back until later"); err != nil {
		return err
	}

	fmt.Printf("==> %s: 4 messages, dead-lettered on purpose\n", failures)
	if err := send(ctx, data, failures, 4, "gave-up", nil); err != nil {
		return err
	}
	if err := deadLetterAll(ctx, data, failures); err != nil {
		return err
	}

	fmt.Printf("==> %s: 9 messages across three rules\n", events)
	if err := sendEvents(ctx, data); err != nil {
		return err
	}

	fmt.Printf("==> %s: 3 messages nothing subscribes to\n", orphaned)
	if err := send(ctx, data, orphaned, 3, "into-the-void", nil); err != nil {
		return err
	}

	fmt.Printf("==> %s: nothing sent, so a board has an empty queue to draw\n", quiet)

	return report(ctx, data, management)
}

// report counts what the namespace actually holds, by peeking.
//
// Peeking rather than reading a message count, and that is the emulator rather
// than a preference: a queue's and a topic's Atom description carry no
// CountDetails element there at all, and a subscription's carries one whose
// children the SDK cannot read. A peek is non-destructive, so counting this
// way costs the boards nothing.
func report(ctx context.Context, data *azservicebus.Client, management *admin.Client) error {
	fmt.Println("==> what the namespace holds now")

	expected := map[string]int{orders: 13, failures: 0, quiet: 0}
	for _, name := range []string{orders, failures, quiet} {
		held, err := peekCount(ctx, data, name, "")
		if err != nil {
			return err
		}
		if held != expected[name] {
			return fmt.Errorf("%s holds %d messages, seeded %d", name, held, expected[name])
		}
		fmt.Printf("    queue %s · %d message(s)\n", name, held)
	}

	// The orphaned topic is checked rather than counted, and it has to be: a
	// topic holds nothing readable. Its three messages were accepted and
	// discarded on arrival because no subscription existed to copy them into,
	// which is the whole reason it is here.
	attached, err := listSubscriptions(ctx, management, orphaned)
	if err != nil {
		return err
	}
	if len(attached) != 0 {
		return fmt.Errorf("%s has %d subscription(s); it exists to have none", orphaned, len(attached))
	}
	fmt.Printf("    topic %s · 0 subscriptions, so its 3 messages were discarded on arrival\n", orphaned)

	dead, err := peekDeadLetters(ctx, data, failures)
	if err != nil {
		return err
	}
	if dead != 4 {
		return fmt.Errorf("%s holds %d dead letters, dead-lettered 4", failures, dead)
	}
	fmt.Printf("    queue %s · %d dead letter(s)\n", failures, dead)

	// Each subscription's rules decide what reached it, which is the whole
	// point of the topic: 9 sent, 9 to the unfiltered one, 4 red, 3 orders.
	expectedSubs := map[string]int{subAll: 9, subRed: 4, subOrders: 3}
	for _, name := range []string{subAll, subRed, subOrders} {
		held, err := peekCount(ctx, data, events, name)
		if err != nil {
			return err
		}
		if held != expectedSubs[name] {
			return fmt.Errorf("%s/%s holds %d messages, its rules should have let %d through",
				events, name, held, expectedSubs[name])
		}
		rules, err := listRules(ctx, management, events, name)
		if err != nil {
			return err
		}
		fmt.Printf("    subscription %s/%s · %d message(s) · rules %s\n",
			events, name, held, strings.Join(rules, ", "))
	}

	fmt.Println("==> done")
	return nil
}

func send(ctx context.Context, data *azservicebus.Client, entity string, count int, prefix string,
	properties map[string]any,
) error {
	sender, err := data.NewSender(entity, nil)
	if err != nil {
		return fmt.Errorf("opening a sender on %s: %w", entity, err)
	}
	defer func() { _ = sender.Close(ctx) }()

	for index := 1; index <= count; index++ {
		message := &azservicebus.Message{
			Body:                  []byte(fmt.Sprintf("%s-%d", prefix, index)),
			ApplicationProperties: map[string]any{"seedIndex": int64(index)},
		}
		for name, value := range properties {
			message.ApplicationProperties[name] = value
		}
		if err := sender.SendMessage(ctx, message, nil); err != nil {
			return fmt.Errorf("sending to %s: %w", entity, err)
		}
	}
	return nil
}

// sendEvents fills the topic so each subscription's rule lets a different
// number through: nine in total, four red, three carrying the order subject.
func sendEvents(ctx context.Context, data *azservicebus.Client) error {
	sender, err := data.NewSender(events, nil)
	if err != nil {
		return fmt.Errorf("opening a sender on %s: %w", events, err)
	}
	defer func() { _ = sender.Close(ctx) }()

	kinds := []struct {
		colour  string
		subject string
	}{
		{"red", "order"}, {"red", "order"}, {"red", "shipment"}, {"red", "shipment"},
		{"blue", "order"}, {"blue", "shipment"}, {"blue", "shipment"},
		{"green", "shipment"}, {"green", "shipment"},
	}
	for index, kind := range kinds {
		subject := kind.subject
		if err := sender.SendMessage(ctx, &azservicebus.Message{
			Body:                  []byte(fmt.Sprintf("event-%d", index+1)),
			Subject:               &subject,
			ApplicationProperties: map[string]any{"colour": kind.colour, "seedIndex": int64(index + 1)},
		}, nil); err != nil {
			return fmt.Errorf("sending to %s: %w", events, err)
		}
	}
	return nil
}

// schedule holds one message back, which is a state no consumer is offered and
// a peek still reaches.
func schedule(ctx context.Context, data *azservicebus.Client, entity, body string) error {
	sender, err := data.NewSender(entity, nil)
	if err != nil {
		return fmt.Errorf("opening a sender on %s: %w", entity, err)
	}
	defer func() { _ = sender.Close(ctx) }()

	_, err = sender.ScheduleMessages(ctx,
		[]*azservicebus.Message{{Body: []byte(body)}}, time.Now().Add(time.Hour), nil)
	if err != nil {
		return fmt.Errorf("scheduling on %s: %w", entity, err)
	}
	return nil
}

// deadLetterAll moves everything a queue holds into its $DeadLetterQueue.
//
// By receiving and dead-lettering each message deliberately rather than by
// letting the delivery count run out: the second would take MaxDeliveryCount
// lock expiries per message, which is minutes of waiting for the same result.
func deadLetterAll(ctx context.Context, data *azservicebus.Client, entity string) error {
	receiver, err := data.NewReceiverForQueue(entity, nil)
	if err != nil {
		return fmt.Errorf("opening a receiver on %s: %w", entity, err)
	}
	defer func() { _ = receiver.Close(ctx) }()

	reason, description := "seeded", "dead-lettered by the seed so the board has something to read"
	for {
		batch, err := receiveBatch(ctx, receiver)
		if err != nil {
			return fmt.Errorf("receiving from %s: %w", entity, err)
		}
		if len(batch) == 0 {
			return nil
		}
		for _, message := range batch {
			if err := receiver.DeadLetterMessage(ctx, message, &azservicebus.DeadLetterOptions{
				Reason: &reason, ErrorDescription: &description,
			}); err != nil {
				return fmt.Errorf("dead-lettering on %s: %w", entity, err)
			}
		}
	}
}

func drainQueue(ctx context.Context, data *azservicebus.Client, entity string) error {
	// The scheduled ones first, and they need their own call: a scheduled
	// message cannot be received until its enqueue time, so a receive loop
	// leaves it exactly where it is and a re-run would seed a second one.
	if err := cancelScheduled(ctx, data, entity); err != nil {
		return err
	}
	if err := drain(ctx, data, entity, "", false); err != nil {
		return err
	}
	// The dead letters too: they are a sub-entity of their own and emptying
	// the queue leaves them exactly where they were.
	return drain(ctx, data, entity, "", true)
}

// cancelScheduled unschedules everything an entity is holding back.
//
// Found by peeking, which is the only way to see one: a scheduled message is
// not offered to any receiver until its time comes.
func cancelScheduled(ctx context.Context, data *azservicebus.Client, entity string) error {
	receiver, err := openReceiver(data, entity, "", false)
	if err != nil {
		return err
	}
	defer func() { _ = receiver.Close(ctx) }()

	from := int64(0)
	var sequences []int64
	for {
		peeked, err := receiver.PeekMessages(ctx, 100, &azservicebus.PeekMessagesOptions{
			FromSequenceNumber: &from,
		})
		if err != nil {
			return fmt.Errorf("peeking %s: %w", entity, err)
		}
		if len(peeked) == 0 {
			break
		}
		for _, message := range peeked {
			if message.State == azservicebus.MessageStateScheduled {
				sequences = append(sequences, *message.SequenceNumber)
			}
		}
		from = *peeked[len(peeked)-1].SequenceNumber + 1
	}
	if len(sequences) == 0 {
		return nil
	}

	sender, err := data.NewSender(entity, nil)
	if err != nil {
		return fmt.Errorf("opening a sender on %s: %w", entity, err)
	}
	defer func() { _ = sender.Close(ctx) }()
	if err := sender.CancelScheduledMessages(ctx, sequences, nil); err != nil {
		return fmt.Errorf("cancelling scheduled messages on %s: %w", entity, err)
	}
	return nil
}

func drainSubscription(ctx context.Context, data *azservicebus.Client, topic, subscription string) error {
	if err := drain(ctx, data, topic, subscription, false); err != nil {
		return err
	}
	return drain(ctx, data, topic, subscription, true)
}

// drain empties an entity by receiving and completing everything in it.
//
// Service Bus has no purge call of any kind: emptying a queue means taking its
// messages, which is what the portal's own purge does too.
func drain(ctx context.Context, data *azservicebus.Client, entity, subscription string, deadLetters bool) error {
	receiver, err := openReceiver(data, entity, subscription, deadLetters)
	if err != nil {
		return err
	}
	defer func() { _ = receiver.Close(ctx) }()

	for {
		batch, err := receiveBatch(ctx, receiver)
		if err != nil {
			return fmt.Errorf("draining %s: %w", entity, err)
		}
		if len(batch) == 0 {
			return nil
		}
		for _, message := range batch {
			if err := receiver.CompleteMessage(ctx, message, nil); err != nil {
				return fmt.Errorf("draining %s: %w", entity, err)
			}
		}
	}
}

// receiveBatch takes what is there now and does not wait for more. The short
// window is the stop condition: an empty batch means the entity is empty.
func receiveBatch(ctx context.Context, receiver *azservicebus.Receiver) ([]*azservicebus.ReceivedMessage, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	batch, err := receiver.ReceiveMessages(waitCtx, 50, nil)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	return batch, nil
}

// peekCount counts what an entity holds, from the first sequence number.
//
// The explicit start matters: a receiver's peek carries a cursor that advances
// with every call, so a second peek without one would report what is left
// after the first rather than what is there.
func peekCount(ctx context.Context, data *azservicebus.Client, entity, subscription string) (int, error) {
	receiver, err := openReceiver(data, entity, subscription, false)
	if err != nil {
		return 0, err
	}
	defer func() { _ = receiver.Close(ctx) }()
	return countPeeked(ctx, receiver, entity)
}

func peekDeadLetters(ctx context.Context, data *azservicebus.Client, entity string) (int, error) {
	receiver, err := openReceiver(data, entity, "", true)
	if err != nil {
		return 0, err
	}
	defer func() { _ = receiver.Close(ctx) }()
	return countPeeked(ctx, receiver, entity)
}

func countPeeked(ctx context.Context, receiver *azservicebus.Receiver, entity string) (int, error) {
	from := int64(0)
	total := 0
	for {
		peeked, err := receiver.PeekMessages(ctx, 100, &azservicebus.PeekMessagesOptions{
			FromSequenceNumber: &from,
		})
		if err != nil {
			return 0, fmt.Errorf("peeking %s: %w", entity, err)
		}
		if len(peeked) == 0 {
			return total, nil
		}
		total += len(peeked)
		from = *peeked[len(peeked)-1].SequenceNumber + 1
	}
}

func openReceiver(data *azservicebus.Client, entity, subscription string, deadLetters bool,
) (*azservicebus.Receiver, error) {
	options := &azservicebus.ReceiverOptions{ReceiveMode: azservicebus.ReceiveModePeekLock}
	if deadLetters {
		options.SubQueue = azservicebus.SubQueueDeadLetter
	}
	if subscription == "" {
		receiver, err := data.NewReceiverForQueue(entity, options)
		if err != nil {
			return nil, fmt.Errorf("opening a receiver on %s: %w", entity, err)
		}
		return receiver, nil
	}
	receiver, err := data.NewReceiverForSubscription(entity, subscription, options)
	if err != nil {
		return nil, fmt.Errorf("opening a receiver on %s/%s: %w", entity, subscription, err)
	}
	return receiver, nil
}

func listRules(ctx context.Context, management *admin.Client, topic, subscription string) ([]string, error) {
	pager := management.NewListRulesPager(topic, subscription, nil)
	var names []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing rules on %s/%s: %w", topic, subscription, err)
		}
		for _, rule := range page.Rules {
			names = append(names, rule.Name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s/%s has no rules at all, so nothing can reach it", topic, subscription)
	}
	return names, nil
}

func listSubscriptions(ctx context.Context, management *admin.Client, topic string) ([]string, error) {
	pager := management.NewListSubscriptionsPager(topic, nil)
	names := []string{}
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing subscriptions on %s: %w", topic, err)
		}
		for _, subscription := range page.Subscriptions {
			names = append(names, subscription.SubscriptionName)
		}
	}
	return names, nil
}

func gone(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// rewrite points the management client at the emulator's plain-HTTP Atom port.
//
// The same seam internal/driver/azureservicebus/client.go uses, and written
// out again here rather than imported: the seed is what the cross-check
// compares the driver against, so it must not share the driver's code.
type rewrite struct{ host string }

func (r rewrite) Do(request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = r.host
	request.Host = r.host
	return http.DefaultClient.Do(request)
}
