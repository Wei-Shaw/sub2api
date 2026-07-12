import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("groups models list layout", () => {
  it("keeps the toolbar outside of the scrolling list content", () => {
    expect(groupsViewSource).toContain("overflow-hidden rounded-lg border");
    expect(groupsViewSource).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(groupsViewSource).not.toContain("sticky top-0");
  });

  it("keeps platform badge colors centralized for Kiro", () => {
    expect(groupsViewSource).toContain("platformBadgeLightClass(value)");
    expect(groupsViewSource).toContain("platformBadgeLightClass(group.platform)");
    expect(groupsViewSource).not.toContain("value === 'anthropic' || value === 'kiro'");
    expect(groupsViewSource).not.toContain("group.platform === 'anthropic' || group.platform === 'kiro'");
  });
});
