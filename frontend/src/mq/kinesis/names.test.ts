import { describe, expect, it } from "vitest";
import {
  MAX_RETENTION_HOURS,
  MIN_RETENTION_HOURS,
  nameProblem,
  retentionProblem,
  shardsProblem,
  submittableName,
} from "./names";

/**
 * The service's own rules, pinned where the dialog can be tested without
 * rendering it.
 *
 * A name Kinesis refuses comes back as a ValidationException quoting a regular
 * expression, which names no character the user typed. A retention it refuses
 * names "RetentionPeriodHours" without saying which end it missed.
 */
describe("a Kinesis stream name", () => {
  it("takes letters, digits, underscores, hyphens and dots", () => {
    expect(nameProblem("orders")).toBeNull();
    expect(nameProblem("team.orders-2024_v1")).toBeNull();
  });

  it("refuses what the service refuses, before the service does", () => {
    expect(nameProblem("")).toBe("empty");
    expect(nameProblem("   ")).toBe("empty");
    expect(nameProblem("orders/eu")).toBe("charset");
    expect(nameProblem("team orders")).toBe("charset");
    expect(nameProblem("x".repeat(129))).toBe("tooLong");
    expect(nameProblem("x".repeat(128))).toBeNull();
  });

  it("submits the trimmed name, or nothing at all", () => {
    expect(submittableName("  orders  ")).toBe("orders");
    expect(submittableName("orders/eu")).toBeNull();
  });
});

describe("a stream's retention and capacity", () => {
  it("holds the retention to the day-to-year window the service enforces", () => {
    expect(retentionProblem(MIN_RETENTION_HOURS)).toBeNull();
    expect(retentionProblem(MAX_RETENTION_HOURS)).toBeNull();
    expect(retentionProblem(MIN_RETENTION_HOURS - 1)).toBe("tooShort");
    expect(retentionProblem(MAX_RETENTION_HOURS + 1)).toBe("tooLong");
  });

  // Only the floor. The ceiling is a per-account quota that differs by region
  // and is raised on request, so a number refused here would be invented.
  it("asks a provisioned stream for at least one shard and caps nothing", () => {
    expect(shardsProblem(1)).toBeNull();
    expect(shardsProblem(500)).toBeNull();
    expect(shardsProblem(0)).toBe("tooFew");
    expect(shardsProblem(1.5)).toBe("tooFew");
  });
});
