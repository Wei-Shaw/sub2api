import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(resolve(currentDir, "../GroupsView.vue"), "utf8");

describe("groups Kiro cache emulation modes", () => {
  it("exposes uniform and independent ratio controls for create and edit", () => {
    expect(groupsViewSource).toContain("setCreateKiroCacheMode('uniform')");
    expect(groupsViewSource).toContain("setCreateKiroCacheMode('independent')");
    expect(groupsViewSource).toContain("setEditKiroCacheMode('uniform')");
    expect(groupsViewSource).toContain("setEditKiroCacheMode('independent')");
    expect(groupsViewSource).toContain("kiro_cache_creation_emulation_ratio");
    expect(groupsViewSource).toContain("kiro_cache_read_emulation_ratio");
    expect(groupsViewSource.match(/<KiroCacheRatioField/g)).toHaveLength(6);
    expect(groupsViewSource).toContain(
      ":aria-pressed=\"createForm.kiro_cache_emulation_mode === 'uniform'\"",
    );
    expect(groupsViewSource).toContain(
      ":aria-pressed=\"editForm.kiro_cache_emulation_mode === 'independent'\"",
    );
  });

  it("inherits the uniform ratio when switching to independent mode", () => {
    expect(groupsViewSource).toContain(
      "createForm.kiro_cache_creation_emulation_ratio = createForm.kiro_cache_emulation_ratio",
    );
    expect(groupsViewSource).toContain(
      "editForm.kiro_cache_read_emulation_ratio = editForm.kiro_cache_emulation_ratio",
    );
  });
});
