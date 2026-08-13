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
    expect(source).toContain("overflow-hidden rounded-lg border");
    expect(source).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(source).not.toContain("sticky top-0");
  });
});
