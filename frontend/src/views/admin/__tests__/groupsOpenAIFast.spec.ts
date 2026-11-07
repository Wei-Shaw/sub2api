import { describe, expect, it } from "vitest";

import {
  normalizeGroupOpenAIFast,
  supportsGroupOpenAIFast,
  groupOpenAIServiceTier,
  setGroupOpenAIServiceTier,
} from "../groupsOpenAIFast";
import en from "@/i18n/locales/en/admin/overview";
import zh from "@/i18n/locales/zh/admin/overview";

describe("groupsOpenAIFast", () => {
  it("reads legacy Fast settings and all three policies", () => {
    expect(groupOpenAIServiceTier({})).toBe("client");
    expect(groupOpenAIServiceTier({ force_openai_fast: true })).toBe("priority");
    expect(groupOpenAIServiceTier({ force_openai_ultrafast: true })).toBe("ultrafast");
  });

  it("keeps Fast and Ultrafast mutually exclusive when switching", () => {
    const form = { platform: "openai", force_openai_fast: false, force_openai_ultrafast: false };
    for (const tier of ["ultrafast", "priority", "client"] as const) {
      setGroupOpenAIServiceTier(form, tier);
      expect(groupOpenAIServiceTier(form)).toBe(tier);
      expect(form.force_openai_fast && form.force_openai_ultrafast).toBe(false);
    }
    form.platform = "anthropic";
    setGroupOpenAIServiceTier(form, "ultrafast");
    expect(groupOpenAIServiceTier(form)).toBe("client");
  });

  it("supports OpenAI and composite groups", () => {
    expect(supportsGroupOpenAIFast("openai")).toBe(true);
    expect(supportsGroupOpenAIFast("composite")).toBe(true);
    expect(supportsGroupOpenAIFast("anthropic")).toBe(false);
  });

  it("clears stale enabled state on unsupported platforms", () => {
    expect(normalizeGroupOpenAIFast("openai", true)).toBe(true);
    expect(normalizeGroupOpenAIFast("composite", true)).toBe(true);
    expect(normalizeGroupOpenAIFast("anthropic", true)).toBe(false);
    expect(normalizeGroupOpenAIFast("openai", false)).toBe(false);
  });

  it("provides localized switch labels", () => {
    expect(zh.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      forceUltrafast: expect.any(String),
      client: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
    expect(en.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      forceUltrafast: expect.any(String),
      client: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
  });
});
