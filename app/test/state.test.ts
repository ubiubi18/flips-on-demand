import { test } from "node:test";
import assert from "node:assert/strict";
import { snapshotIsRecent } from "../src/state.ts";

test("purchase preparation requires a recent, readable snapshot time", () => {
  const now = Date.parse("2026-10-10T12:00:00Z");
  assert.equal(snapshotIsRecent("2026-10-10T11:45:00Z", now), true);
  assert.equal(snapshotIsRecent("2026-10-10T11:44:59Z", now), false);
  assert.equal(snapshotIsRecent("2026-10-10T12:01:00Z", now), true);
  assert.equal(snapshotIsRecent("2026-10-10T12:01:01Z", now), false);
  assert.equal(snapshotIsRecent("invalid", now), false);
});
