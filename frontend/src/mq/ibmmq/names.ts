/**
 * IBM MQ's rule for what an object may be called.
 *
 * It is applied in the renderer rather than left to the queue manager because
 * of how the queue manager refuses one: the command server answers with a
 * syntax error naming a character position, which reads as a broken app rather
 * than as a name that was never allowed. internal/driver/ibmmq applies the same
 * rule, for callers that never pass through this form.
 *
 * Forty-eight characters of letters, digits and . _ / % - and nothing else,
 * including no space. It is the same rule for a queue, a topic, a channel and
 * a queue manager, which is why this is one function rather than four.
 */

/** Which object a name is for. They share the rule and nothing else. */
export type IbmMqObjectKind = "queue" | "topic";

export const MAX_OBJECT_NAME = 48;

/** What is wrong with a name, or null when nothing is. */
export type ObjectNameProblem = "empty" | "tooLong" | "characters";

export function objectNameProblem(name: string): ObjectNameProblem | null {
  const trimmed = name.trim();
  if (trimmed === "") return "empty";
  if (trimmed.length > MAX_OBJECT_NAME) return "tooLong";
  if (!/^[A-Za-z0-9._/%]+$/.test(trimmed)) return "characters";
  return null;
}

/** The name to submit, or null when it would be refused. */
export function submittableObjectName(name: string): string | null {
  return objectNameProblem(name) == null ? name.trim() : null;
}

/**
 * Whether a name is one the queue manager made for itself.
 *
 * SYSTEM. is IBM's reserved prefix and the queue manager enforces it - it will
 * not define an object there - so a form that let a user type one would be
 * offering a create that is always refused.
 */
export function reservedName(name: string): boolean {
  return name.trim().toUpperCase().startsWith("SYSTEM.");
}
