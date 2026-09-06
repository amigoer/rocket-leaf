import { describe, expect, it } from "vitest";
import { SPINNER_DELAY_MS, SPINNER_MIN_MS, spinnerWindow } from "./spinnerTiming";

/*
 * The policy, in the numbers the app actually sees.
 *
 * The figures below were read off the status bar while walking every new
 * driver against its container, so they are what a page switch costs in
 * practice rather than a guess. What must hold is that none of them puts a
 * spinner on screen, and that the one read slow enough to warrant one does
 * not get it for a moment and then lose it again.
 */
describe("the loading spinner", () => {
  it.each([
    ["Google Pub/Sub", 18],
    ["Amazon SQS", 21],
    ["Amazon Kinesis", 36],
    ["NSQ", 14],
    ["Solace", 100],
  ])("stays away for a %s read (%ims)", (_family, ms) => {
    expect(spinnerWindow(ms).shown).toBe(false);
  });

  it("stays away right up to the delay", () => {
    expect(spinnerWindow(SPINNER_DELAY_MS).shown).toBe(false);
    expect(spinnerWindow(SPINNER_DELAY_MS + 1).shown).toBe(true);
  });

  it("never shows for less than the minimum, which is the flicker again", () => {
    // An IBM MQ queue manager over mqweb: slow enough to say something, fast
    // enough that a delay on its own would flash.
    const held = spinnerWindow(386);
    expect(held.shown).toBe(true);
    expect(held.forMs).toBeGreaterThanOrEqual(SPINNER_MIN_MS);
  });

  it("tracks the read once it is past both bounds", () => {
    expect(spinnerWindow(1200).forMs).toBe(1200 - SPINNER_DELAY_MS);
  });

  it("is tuned so the minimum outlasts the delay", () => {
    // A minimum below the delay would let a read just past the delay show a
    // spinner for less time than it waited to show one.
    expect(SPINNER_MIN_MS).toBeGreaterThan(SPINNER_DELAY_MS);
  });
});
