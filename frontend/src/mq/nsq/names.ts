/**
 * What NSQ will accept as the name of a topic or a channel.
 *
 * It lives here rather than inside the dialog for the reason the connection
 * rules do: it is the part worth testing, and a rule that only exists inside a
 * component can only be reached by rendering one.
 *
 * The rule is nsqd's own, read off a running 1.3.0 rather than off the docs:
 * one to sixty-four characters from a small set, with an optional #ephemeral
 * suffix that counts towards the sixty-four. Topics and channels share it
 * exactly - there is one regular expression in nsqd for both.
 *
 * Checking here rather than letting nsqd refuse is not politeness. The refusal
 * comes back as INVALID_TOPIC, or as INVALID_REQUEST when the name contains a
 * character that breaks the URL before nsqd sees it at all, and neither says
 * which character was the problem.
 */

/** Sixty-four, and the #ephemeral suffix is inside it rather than extra. */
const NAME_MAX = 64;

const VALID = /^[.a-zA-Z0-9_-]+(#ephemeral)?$/;

/** Why a name is unusable, or null when it is fine. */
export type NameProblem = "empty" | "tooLong" | "charset";

export function nameProblem(typed: string): NameProblem | null {
  const name = typed.trim();
  if (name === "") return "empty";
  if (name.length > NAME_MAX) return "tooLong";
  if (!VALID.test(name)) return "charset";
  return null;
}

/** The name a submission carries, or null when there is nothing to submit. */
export function submittableName(typed: string): string | null {
  return nameProblem(typed) == null ? typed.trim() : null;
}

/**
 * Whether a name asks nsqd for an object that disappears with its last client.
 *
 * Worth its own reader because the consequence is severe and invisible: an
 * ephemeral topic keeps nothing on disk and is deleted outright when the last
 * client goes, so a create typed with the suffix by habit produces something
 * that will not be there tomorrow.
 */
export function isEphemeral(typed: string): boolean {
  return typed.trim().endsWith("#ephemeral");
}
