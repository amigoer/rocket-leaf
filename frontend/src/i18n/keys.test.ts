import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import zh from "./locales/zh.json";

/**
 * Every key written as a literal, in both bundles.
 *
 * i18nCoverage renders the boards and catches an undefined key by the dotted
 * name it echoes back, but only for what a board draws. A dialog is mounted
 * through a Radix portal, which renders nothing without a DOM, so none of the
 * dialogs are in that net - and a missing key there shows up the first time
 * somebody opens the form, as `board.topics.rocketmq.nameLabel` where a label
 * belongs.
 *
 * This is the other half, and it needs no rendering: read the sources, take
 * every t("...") whose key is a literal, and look it up. A key built from a
 * template string is skipped rather than guessed at - those are the ones
 * i18nCoverage does reach, because a board renders them.
 */
const sources = import.meta.glob("../**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// A segment may carry a hyphen - google-pubsub is one family, not two -
// and a pattern without it silently matched nothing rather than failing.
const LITERAL_KEY = /\bt\(\s*"([a-zA-Z][\w.-]*)"/g;

function resolves(bundle: unknown, key: string): boolean {
  let node: unknown = bundle;
  for (const part of key.split(".")) {
    if (typeof node !== "object" || node === null || !(part in node)) return false;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === "string";
}

function usedKeys(): { file: string; key: string }[] {
  const out: { file: string; key: string }[] = [];
  for (const [file, source] of Object.entries(sources)) {
    if (file.includes(".test.")) continue;
    for (const match of source.matchAll(LITERAL_KEY)) {
      const key = match[1];
      if (key != null) out.push({ file, key });
    }
  }
  return out;
}

describe.each([
  ["zh", zh],
  ["en", en],
] as const)("the %s bundle", (_language, bundle) => {
  it("defines every key the sources ask for by name", () => {
    const missing = usedKeys()
      .filter(({ key }) => !resolves(bundle, key))
      .map(({ file, key }) => `${key} (${file})`);
    expect(missing).toEqual([]);
  });
});

describe("the two bundles", () => {
  it("cover enough of the app that this test is doing something", () => {
    // A glob that silently matched nothing would make the checks above pass by
    // having nothing to check.
    expect(usedKeys().length).toBeGreaterThan(1500);
  });
});

/**
 * A key taken with no arguments, whose text wants one.
 *
 * `created` is a toast - "Created {{name}}" - and `modified` beside it is a
 * label. Reusing the toast as the label reads as "Created {{name}}" with the
 * braces still in it, next to a timestamp: nothing throws, no key is missing,
 * and the two bundles agree, so every other check here passes. Three boards
 * had it, one of them since the family shipped.
 */
const KEY_WITH_NO_ARGS = /\bt\(\s*"([a-zA-Z][\w.-]*)"\s*\)/g;

function textOf(bundle: unknown, key: string): string | null {
  let node: unknown = bundle;
  for (const part of key.split(".")) {
    if (typeof node !== "object" || node === null || !(part in node)) return null;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === "string" ? node : null;
}

describe.each([
  ["zh", zh],
  ["en", en],
] as const)("the %s bundle", (_language, bundle) => {
  it("is never asked for an interpolated string without the values", () => {
    const leaking: string[] = [];
    for (const [file, source] of Object.entries(sources)) {
      if (file.includes(".test.")) continue;
      for (const match of source.matchAll(KEY_WITH_NO_ARGS)) {
        const key = match[1];
        if (key == null) continue;
        const text = textOf(bundle, key);
        if (text != null && text.includes("{{")) {
          leaking.push(`${key} -> ${text} (${file})`);
        }
      }
    }
    expect(leaking).toEqual([]);
  });
});
