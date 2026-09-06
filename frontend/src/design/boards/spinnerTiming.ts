/*
 * When a board says it is loading, and for how long it then has to keep
 * saying it.
 *
 * Most reads finish well inside the delay - a queue listing came back in 21ms
 * against LocalStack, a project's topics in 18ms, a stream listing in 36ms -
 * and a spinner drawn and taken away inside that reads as a flicker rather
 * than as loading. The minimum is the other half of the same problem: with a
 * delay alone, a 386ms read (an IBM MQ queue manager over mqweb) shows the
 * spinner for 186ms, which is the same flicker moved later.
 */
export const SPINNER_DELAY_MS = 200;
export const SPINNER_MIN_MS = 300;

/** What a load of this length would put on screen. */
export function spinnerWindow(loadMs: number): { shown: boolean; forMs: number } {
  if (loadMs <= SPINNER_DELAY_MS) return { shown: false, forMs: 0 };
  return { shown: true, forMs: Math.max(SPINNER_MIN_MS, loadMs - SPINNER_DELAY_MS) };
}
