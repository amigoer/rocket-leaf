/**
 * Solace's view of a dead message queue.
 *
 * The keys are a contract with internal/driver/solace/deadletter.go.
 *
 * Two facts arrive on the source rather than on the target, and both change
 * what an empty dead message queue means:
 *
 *   - which kind of endpoint points here. A queue and a topic endpoint are
 *     configured in different places, so a reader fixing one has to know which
 *     they are looking at.
 *   - whether that endpoint moves everything or only what a publisher marked.
 *     respectDmqEligible is on by default and most clients mark nothing, so an
 *     endpoint can point at a perfectly good queue and never move a message.
 */
import type { DeadLetterQueue, DeadLetterSource } from "@bindings/model/models";

/** What the driver writes into a source's exchange field. */
export const SOURCE_QUEUE = "queue";
export const SOURCE_TOPIC_ENDPOINT = "topicEndpoint";

/** What it writes into a source's subscription field. */
export const MOVES_EVERYTHING = "moves-everything";
export const MOVES_MARKED_ONLY = "moves-marked-only";

/**
 * Whether this queue exists at all.
 *
 * An unknown depth means exactly one thing here: the driver reads the depth of
 * every target it found among the Message VPN's queues, and fails the whole
 * call rather than half of it, so a target with no depth is a name nothing
 * holds. That is the ordinary state of "#DEAD_MSG_QUEUE", which every endpoint
 * points at until somebody changes it and no broker ever creates.
 */
export function targetExists(queue: DeadLetterQueue): boolean {
  return queue.depth >= 0;
}

/** How many attempts before this source gives up. Zero is the broker's own unlimited. */
export function redeliveryLimit(source: DeadLetterSource): number {
  const parsed = Number.parseInt(source.routingKey, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

/** Whether this source moves every message or only the marked ones. */
export function movesEverything(source: DeadLetterSource): boolean {
  return source.subscription === MOVES_EVERYTHING;
}

/**
 * The sources that would move nothing even though they point somewhere real.
 *
 * The pair of problems this page exists for: a pointer at a queue that does
 * not exist, and a pointer that is never followed. This is the second.
 */
export function silentSources(queue: DeadLetterQueue): DeadLetterSource[] {
  return (queue.sources ?? []).flatMap((source) =>
    source != null && !movesEverything(source) ? [source] : [],
  );
}
