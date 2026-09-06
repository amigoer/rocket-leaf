/**
 * Solace's rule for what a queue may be called.
 *
 * It is applied in the renderer rather than left to the broker because of what
 * the broker answers with: a SEMP envelope quoting the regular expression the
 * name failed, which is accurate and unreadable.
 * internal/driver/solace/destination_admin.go applies the same rule, for
 * callers that never pass through this form.
 *
 * Two hundred characters and anything except * ? ' < > & and ; - which is a
 * much wider rule than most families here have, and deliberately so: a Solace
 * queue is usually named like the topics it subscribes to, so slashes, dots
 * and even spaces are ordinary rather than exotic.
 */

export const MAX_QUEUE_NAME = 200;

/** The characters the broker's own pattern excludes. */
const FORBIDDEN = /[*?'<>&;]/;

/** What is wrong with a name, or null when nothing is. */
export type QueueNameProblem = "empty" | "tooLong" | "characters";

export function queueNameProblem(name: string): QueueNameProblem | null {
  const trimmed = name.trim();
  if (trimmed === "") return "empty";
  if (trimmed.length > MAX_QUEUE_NAME) return "tooLong";
  if (FORBIDDEN.test(trimmed)) return "characters";
  return null;
}

/** The name to submit, or null when it would be refused. */
export function submittableQueueName(name: string): string | null {
  return queueNameProblem(name) == null ? name.trim() : null;
}

/**
 * Whether a name is one the broker made for itself.
 *
 * A leading # marks the broker's own objects - #DEAD_MSG_QUEUE, #MQTT/*, the
 * replication queues - and creating one there is refused or, worse, taken and
 * then treated as internal. The form does not offer it.
 */
export function reservedName(name: string): boolean {
  return name.trim().startsWith("#");
}

/**
 * Whether an access type shares the queue.
 *
 * Exclusive is the broker's default and hands every message to the first
 * consumer that binds, keeping the rest as standbys. That is right for
 * ordered processing and wrong for everything somebody scaled out, and it is
 * the create setting most likely to be regretted - so a form that offers it
 * should be able to say which one shares.
 */
export function sharesConsumers(accessType: string): boolean {
  return accessType === "non-exclusive";
}
