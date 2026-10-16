import { describe, expect, it } from "vitest";

import {
  cloneCapabilityAllowlist,
  createCapabilityAllowlistDefaults,
  normalizeCapabilityAllowlistForPlatform,
} from "../groupsCapabilityAllowlist";
import en from "@/i18n/locales/en/admin/settings";
import zh from "@/i18n/locales/zh/admin/settings";

describe("groupsCapabilityAllowlist", () => {
  it("clones without sharing arrays and tolerates missing fields", () => {
    const source = { enabled: true, cyber_programs: ["daybreak_blue"], service_tiers: ["priority"] };
    const cloned = cloneCapabilityAllowlist(source);
    cloned.cyber_programs.push("daybreak_red");
    expect(source.cyber_programs).toEqual(["daybreak_blue"]);
    expect(cloneCapabilityAllowlist(undefined)).toEqual(createCapabilityAllowlistDefaults());
    expect(cloneCapabilityAllowlist({ enabled: true } as never)).toEqual({ enabled: true, cyber_programs: [], service_tiers: [] });
  });

  it("zeroes the config for unsupported platforms", () => {
    const source = { enabled: true, cyber_programs: ["daybreak_blue"], service_tiers: ["ultrafast"] };
    expect(normalizeCapabilityAllowlistForPlatform("gemini", source)).toEqual(createCapabilityAllowlistDefaults());
    expect(normalizeCapabilityAllowlistForPlatform("openai", source)).toEqual(source);
  });

  it("notes that the experimental scheduler does not support capability routing", () => {
    expect(zh.settings.openaiExperimentalScheduler.description).toContain("能力路由");
    expect(en.settings.openaiExperimentalScheduler.description).toContain("capability routing");
  });
});
