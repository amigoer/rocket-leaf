/**
 * What Service Bus will accept as an entity name.
 *
 * It lives here rather than inside the dialogs for the reason the connection
 * rules do: it is the part worth testing, and a rule that only exists inside a
 * component can only be reached by rendering one.
 *
 * The rule is the service's own. A queue or topic name is at most 260
 * characters, a subscription or rule name at most 50, and both are made of
 * letters, digits and `-._~` with an optional `/` between path segments. What
 * is refused here beyond that is a leading or trailing separator, which the
 * service rejects with a message about a URI, and a name beginning with `$` -
 * that prefix addresses a sub-entity such as `$DeadLetterQueue`, so a name
 * carrying one would silently reach a different object.
 *
 * Checking here rather than letting the API refuse is not politeness. The
 * refusal comes back as a 400 whose detail is "SubCode=40000" and a tracking
 * id, and it names neither the character nor the field.
 */

/** Queues and topics may be longer than the objects inside them. */
const ENTITY_MAX = 260;
const CHILD_MAX = 50;

const VALID = /^[A-Za-z0-9][A-Za-z0-9._\-~/]*$/;

/** Why a name is unusable, or null when it is fine. */
export type NameProblem = "empty" | "tooLong" | "charset" | "reserved" | "separator";

export type NameScope = "entity" | "child";

export function nameProblem(typed: string, scope: NameScope = "entity"): NameProblem | null {
  const name = typed.trim();
  if (name === "") return "empty";
  if (name.length > (scope === "entity" ? ENTITY_MAX : CHILD_MAX)) return "tooLong";
  // Checked before the charset rule so the message is about the sub-entity
  // rather than about a dollar sign: the two are different mistakes.
  if (name.startsWith("$") || name.includes("/$")) return "reserved";
  if (name.startsWith("/") || name.endsWith("/") || name.includes("//")) return "separator";
  if (!VALID.test(name)) return "charset";
  // A subscription and a rule live inside their parent and cannot be nested
  // further, so a slash in one addresses something that does not exist.
  if (scope === "child" && name.includes("/")) return "charset";
  return null;
}

/** The name a submission carries, or null when there is nothing to submit. */
export function submittableName(typed: string, scope: NameScope = "entity"): string | null {
  return nameProblem(typed, scope) == null ? typed.trim() : null;
}
