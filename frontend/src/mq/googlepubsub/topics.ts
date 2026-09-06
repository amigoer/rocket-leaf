/**
 * Pub/Sub's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/googlepubsub/destination.go.
 *
 * What is missing from this shape is the point of it. A Pub/Sub topic holds
 * nothing: a publish is fanned out to whatever subscriptions exist at that
 * instant and discarded if none do, so there is no depth, no rate and no
 * partition anywhere - the driver reports every one of them unknown rather
 * than zero, and a board that printed zero would be claiming an empty topic
 * where the truth is that no such number exists.
 *
 * `subscribers` is what replaces them. It is the one figure that separates a
 * topic doing its job from one silently throwing every message away.
 */
import type { Destination } from "@bindings/model/models";

const AttrPath = "path";
const AttrRetentionSec = "retentionSec";
const AttrSubscriptionNames = "subscriptionNames";
const AttrKmsKey = "kmsKey";
const AttrSchema = "schema";
const AttrSchemaEncoding = "schemaEncoding";
const AttrStorageRegions = "storageRegions";
const AttrState = "state";

/** Labels carry this prefix, so a user's key cannot collide with a field. */
const LabelPrefix = "label.";

export interface PubSubTopic {
  name: string;
  /** The full resource path every API call addresses it by. */
  path: string | null;

  /** How many subscriptions read it. Zero means every publish is discarded. */
  subscribers: number;
  /** Their names, capped by the driver; `subscribers` is the true count. */
  subscriptionNames: string[];

  /**
   * How long a published message stays available for a subscription to seek
   * back into. Null is the default: kept only until every subscription has
   * acknowledged it.
   */
  retentionSec: number | null;

  /** Customer-managed encryption key, when the topic was created with one. */
  kmsKey: string | null;
  /** The schema published messages are validated against, if any. */
  schema: string | null;
  schemaEncoding: string | null;
  /** Regions the messages may be persisted in, where a policy restricts them. */
  storageRegions: string[];
  /** ACTIVE, or INGESTION_RESOURCE_ERROR for an import topic that is stuck. */
  state: string | null;

  labels: [string, string][];
}

function text(row: Destination, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Destination, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function list(row: Destination, key: string): string[] {
  const value = text(row, key);
  return value == null ? [] : value.split(",").filter((part) => part !== "");
}

export function topic(row: Destination): PubSubTopic {
  return {
    name: row.ref.name,
    path: text(row, AttrPath),

    subscribers: row.subscribers,
    subscriptionNames: list(row, AttrSubscriptionNames),

    retentionSec: number(row, AttrRetentionSec),

    kmsKey: text(row, AttrKmsKey),
    schema: text(row, AttrSchema),
    schemaEncoding: text(row, AttrSchemaEncoding),
    storageRegions: list(row, AttrStorageRegions),
    state: text(row, AttrState),

    labels: Object.entries(row.attributes ?? {})
      .filter(([key]) => key.startsWith(LabelPrefix))
      .map(([key, value]) => [key.slice(LabelPrefix.length), value] as [string, string])
      .sort(([a], [b]) => a.localeCompare(b)),
  };
}

/**
 * Whether everything published to this topic is being thrown away.
 *
 * The state to point at rather than the raw zero. A topic with no subscription
 * accepts every publish, reports success, and discards the message - there is
 * no backlog anywhere afterwards and nothing else on any board says so.
 */
export function discardsEverything(entry: PubSubTopic): boolean {
  return entry.subscribers === 0;
}
