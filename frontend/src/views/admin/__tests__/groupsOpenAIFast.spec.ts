import { describe, expect, it } from "vitest";

import {
  normalizeGroupOpenAIFast,
  supportsGroupOpenAIFast,
} from "../groupsOpenAIFast";
import en from "@/i18n/locales/en/admin/overview";
import zh from "@/i18n/locales/zh/admin/overview";

describe("groupsOpenAIFast", () => {
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
    const labels = {
      title: expect.any(String),
      tier: expect.any(String),
      tierFollow: expect.any(String),
      tierStandard: expect.any(String),
      tierFast: expect.any(String),
      tierUltrafast: expect.any(String),
      hint: expect.stringContaining("service_tier"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
      freeUltrafast: expect.any(String),
      freeUltrafastHint: expect.stringContaining("Standard"),
    };
    expect(zh.groups.openaiFast).toMatchObject(labels);
    expect(en.groups.openaiFast).toMatchObject(labels);
  });
});
