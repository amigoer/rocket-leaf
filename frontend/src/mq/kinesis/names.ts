/**
 * What Kinesis will accept as a stream name, and what a shard count may be.
 *
 * They live here rather than inside the dialog for the reason the connection
 * rules do: they are the part worth testing, and a rule that only exists
 * inside a component can only be reached by rendering one.
 *
 * Checking here rather than letting the service refuse is not politeness. A
 * bad name comes back as ValidationException naming a regular expression, and
 * a bad shard count comes back as InvalidArgumentException naming
 * "ShardCount" - neither says which of the two the form got wrong when both
 * were typed at once.
 */

/** The service's own limit. Names are unique per account and region. */
const NAME_MAX = 128;

/** Letters, digits, underscores, hyphens and dots. No spaces, no slashes. */
const VALID = /^[a-zA-Z0-9_.-]+$/;

/** Retention is a day at the least and a year at the most. */
export const MIN_RETENTION_HOURS = 24;
export const MAX_RETENTION_HOURS = 8760;

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

/** Why a retention is unusable, or null when it is fine. */
export type RetentionProblem = "tooShort" | "tooLong";

export function retentionProblem(hours: number): RetentionProblem | null {
  if (hours < MIN_RETENTION_HOURS) return "tooShort";
  if (hours > MAX_RETENTION_HOURS) return "tooLong";
  return null;
}

/**
 * Whether a provisioned stream's shard count is usable.
 *
 * Only the floor is checked. There is a ceiling, but it is a per-account quota
 * rather than a constant - it differs by region and is raised on request - so
 * a number refused here would be one this app invented.
 */
export function shardsProblem(shards: number): "tooFew" | null {
  return Number.isInteger(shards) && shards >= 1 ? null : "tooFew";
}
