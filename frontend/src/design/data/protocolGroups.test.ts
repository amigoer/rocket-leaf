import { describe, expect, it } from "vitest";
import {
  PROTOCOL_GROUPS,
  PROTOCOL_ORDER,
  groupOf,
  protocolsIn,
} from "./protocols";

/*
 * The families the connection dialog's first step lists.
 *
 * The dialog itself is not tested here: it mounts through a Radix portal,
 * which renders nothing without a DOM, so a server-rendered assertion about
 * its markup passes against an empty string and proves nothing. What is
 * testable is the data the step is built from, and that is where a protocol
 * would go missing.
 *
 * A Record keyed by ProtocolId already makes a missing family a build error.
 * These cover what the type cannot: that the picker lists every protocol
 * exactly once, and that no group is empty and therefore drawn as a heading
 * with nothing under it.
 */
describe("the protocol families", () => {
  it("list every protocol exactly once between them", () => {
    const listed = PROTOCOL_GROUPS.flatMap((group) => protocolsIn(group.id));
    expect([...listed].sort()).toEqual([...PROTOCOL_ORDER].sort());
    expect(new Set(listed).size).toBe(listed.length);
  });

  it("keep each protocol in the order the picker uses", () => {
    for (const group of PROTOCOL_GROUPS) {
      const members = protocolsIn(group.id);
      const positions = members.map((p) => PROTOCOL_ORDER.indexOf(p));
      expect(positions).toEqual([...positions].sort((a, b) => a - b));
    }
  });

  it("are none of them empty", () => {
    for (const group of PROTOCOL_GROUPS) {
      expect(protocolsIn(group.id).length).toBeGreaterThan(0);
    }
  });

  it("agree with groupOf", () => {
    for (const group of PROTOCOL_GROUPS) {
      for (const protocol of protocolsIn(group.id)) {
        expect(groupOf(protocol)).toBe(group.id);
      }
    }
  });
});
