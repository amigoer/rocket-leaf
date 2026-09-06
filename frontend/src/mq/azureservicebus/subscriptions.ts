/**
 * Service Bus's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/azureservicebus/subscription.go.
 *
 * A subscription here is where a topic's messages actually are: a send is
 * copied into every subscription whose rules let it through, so the backlog
 * belongs to the subscription and never to the topic. That is why this board
 * carries the delivery contract - the lock, the delivery limit, where dead
 * letters go - which on a queue sits on the queue itself.
 *
 * `backlog` is null against the Service Bus emulator, which reports no message
 * counts of any kind. The driver degrades the capability with a reason there
 * rather than printing a zero.
 */
import type { Subscription } from "@bindings/model/models";

const AttrTopic = "topic";
const AttrStatus = "status";
const AttrLockDurationSec = "lockDurationSec";
const AttrMaxDeliveryCount = "maxDeliveryCount";
const AttrTTLSec = "ttlSec";
const AttrAutoDeleteOnIdleSec = "autoDeleteOnIdleSec";
const AttrRequiresSession = "requiresSession";
const AttrDeadLetterOnExpiry = "deadLetterOnExpiry";
const AttrDeadLetterOnRuleFail = "deadLetterOnRuleError";
const AttrForwardTo = "forwardTo";
const AttrForwardDeadLettersTo = "forwardDeadLettersTo";
const AttrDeadLetterCount = "deadLetterCount";
const AttrRuleNames = "ruleNames";

export interface ServiceBusSubscription {
  name: string;
  /** The one topic it reads, chosen at creation and unchangeable. */
  topic: string;

  /** Messages waiting to be delivered. Null where the service reports none. */
  backlog: number | null;
  /** What it has given up on. Null wherever the backlog is. */
  deadLetterCount: number | null;

  /**
   * The rules deciding what reaches it. A new subscription has $Default, which
   * matches everything; an empty list means nothing can ever arrive.
   */
  ruleNames: string[];

  status: string | null;
  lockDurationSec: number | null;
  maxDeliveryCount: number | null;
  ttlSec: number | null;
  autoDeleteOnIdleSec: number | null;

  requiresSession: boolean;
  deadLetterOnExpiry: boolean;
  deadLetterOnRuleError: boolean;

  forwardTo: string | null;
  forwardDeadLettersTo: string | null;
}

function text(row: Subscription, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Subscription, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function flag(row: Subscription, key: string): boolean {
  return text(row, key) === "true";
}

export function subscription(row: Subscription): ServiceBusSubscription {
  const rules = text(row, AttrRuleNames);
  return {
    name: row.ref.name,
    // The driver puts the topic in the ref's namespace, which is where every
    // family whose subscription belongs to something puts its parent.
    topic: row.ref.namespace || (text(row, AttrTopic) ?? ""),

    backlog: row.backlog < 0 ? null : row.backlog,
    deadLetterCount: number(row, AttrDeadLetterCount),

    ruleNames: rules == null ? [] : rules.split(",").filter((part) => part !== ""),

    status: text(row, AttrStatus),
    lockDurationSec: number(row, AttrLockDurationSec),
    maxDeliveryCount: number(row, AttrMaxDeliveryCount),
    ttlSec: number(row, AttrTTLSec),
    autoDeleteOnIdleSec: number(row, AttrAutoDeleteOnIdleSec),

    requiresSession: flag(row, AttrRequiresSession),
    deadLetterOnExpiry: flag(row, AttrDeadLetterOnExpiry),
    deadLetterOnRuleError: flag(row, AttrDeadLetterOnRuleFail),

    forwardTo: text(row, AttrForwardTo),
    forwardDeadLettersTo: text(row, AttrForwardDeadLettersTo),
  };
}

/**
 * Whether nothing can ever reach this subscription.
 *
 * A subscription is created with a $Default rule matching everything, so an
 * empty rule list is something somebody did: they deleted the default without
 * adding a replacement. Every other figure on the board still looks healthy -
 * the subscription exists, it is Active, its backlog is zero - and no message
 * will ever arrive again.
 */
export function receivesNothing(row: ServiceBusSubscription): boolean {
  return row.ruleNames.length === 0;
}
