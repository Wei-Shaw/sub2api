import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));

// The models-list block lives in both group dialogs. Asserting per file rather
// than over one concatenated blob keeps create and edit honest independently —
// the old single-file assertion could be satisfied by either half alone.
const dialogs = [
  "GroupCreateDialog",
  "GroupEditDialog",
] as const;

describe.each(dialogs)("groups models list layout (%s)", (dialog) => {
  const source = readFileSync(
    resolve(currentDir, `../groups/${dialog}.vue`),
    "utf8",
  );

  it("keeps the toolbar outside of the scrolling list content", () => {
    // The radius is deliberately not pinned. What this contract is about is
    // that the wrapper clips and is a bordered container while the *inner*
    // list scrolls; pinning `rounded-lg` made a paint decision load-bearing,
    // and the Pass B radius change failed a layout test for no layout reason.
    expect(source).toMatch(/overflow-hidden[\w\- ]*border border-line/);
    expect(source).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(source).not.toContain("sticky top-0");
  });
});
