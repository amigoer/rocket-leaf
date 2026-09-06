/**
 * What SQS will accept as a queue name, and what the .fifo suffix decides.
 *
 * It lives here rather than inside the dialog for the reason the connection
 * rules do: it is the part worth testing, and a rule that only exists inside a
 * component can only be reached by rendering one.
 *
 * The rule is the service's own: up to eighty characters of letters, digits,
 * hyphens and underscores. A FIFO queue's name ends in `.fifo`, and that is
 * the only place a dot may appear - the suffix is part of the eighty rather
 * than extra.
 *
 * Checking here rather than letting SQS refuse is not politeness. The refusal
 * comes back as InvalidParameterValue naming "QueueName" and no character, and
 * a FIFO mismatch comes back naming the FifoQueue attribute - a field the form
 * never drew and the user never typed.
 */

/** Eighty, and the .fifo suffix is inside it rather than extra. */
const NAME_MAX = 80;

/** The suffix that makes a queue FIFO. It is required, not conventional. */
export const FIFO_SUFFIX = ".fifo";

const VALID = /^[a-zA-Z0-9_-]+$/;

/** Why a name is unusable, or null when it is fine. */
export type NameProblem = "empty" | "tooLong" | "charset";

/** Whether a name asks for a FIFO queue, which is decided by the name alone. */
export function isFifoName(typed: string): boolean {
  return typed.trim().endsWith(FIFO_SUFFIX);
}

export function nameProblem(typed: string): NameProblem | null {
  const name = typed.trim();
  if (name === "") return "empty";
  if (name.length > NAME_MAX) return "tooLong";
  // The suffix is the one legal dot, so it comes off before the character set
  // is checked and its own emptiness is caught as an empty name.
  const stem = isFifoName(name) ? name.slice(0, -FIFO_SUFFIX.length) : name;
  if (stem === "") return "empty";
  if (!VALID.test(stem)) return "charset";
  return null;
}

/** The name a submission carries, or null when there is nothing to submit. */
export function submittableName(typed: string): string | null {
  return nameProblem(typed) == null ? typed.trim() : null;
}

/**
 * The name a queue must have for the FIFO switch to hold.
 *
 * Worth doing here rather than warning about it: whether a queue is FIFO is
 * fixed at creation and spelled in its name, so a switch that did not change
 * the name would be a control that silently did nothing.
 */
export function withFifo(typed: string, fifo: boolean): string {
  const name = typed.trim();
  if (fifo) return isFifoName(name) ? name : name + FIFO_SUFFIX;
  return isFifoName(name) ? name.slice(0, -FIFO_SUFFIX.length) : name;
}
