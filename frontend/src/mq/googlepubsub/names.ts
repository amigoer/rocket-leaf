/**
 * What Pub/Sub will accept as a topic or subscription name.
 *
 * It lives here rather than inside the dialogs for the reason the connection
 * rules do: it is the part worth testing, and a rule that only exists inside a
 * component can only be reached by rendering one.
 *
 * The rule is the service's own, and every clause of it has caught somebody:
 * three to 255 characters, starting with a letter, made of letters, digits and
 * `-_.~+%`, and not starting with `goog`, which the service reserves for
 * itself. One name space covers topics, subscriptions and snapshots alike.
 *
 * Checking here rather than letting the API refuse is not politeness. The
 * refusal comes back as INVALID_ARGUMENT naming "Resource name" and no
 * character, and the reserved-prefix rule is not mentioned at all.
 */

const NAME_MIN = 3;
const NAME_MAX = 255;

/** Google keeps this prefix for its own resources, in any case. */
const RESERVED_PREFIX = /^goog/i;

const VALID = /^[a-zA-Z][a-zA-Z0-9\-_.~+%]*$/;

/** Why a name is unusable, or null when it is fine. */
export type NameProblem = "empty" | "tooShort" | "tooLong" | "charset" | "reserved";

export function nameProblem(typed: string): NameProblem | null {
  const name = typed.trim();
  if (name === "") return "empty";
  if (name.length < NAME_MIN) return "tooShort";
  if (name.length > NAME_MAX) return "tooLong";
  if (!VALID.test(name)) return "charset";
  if (RESERVED_PREFIX.test(name)) return "reserved";
  return null;
}

/** The name a submission carries, or null when there is nothing to submit. */
export function submittableName(typed: string): string | null {
  return nameProblem(typed) == null ? typed.trim() : null;
}
