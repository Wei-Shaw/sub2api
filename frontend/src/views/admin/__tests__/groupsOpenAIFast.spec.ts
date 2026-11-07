import { describe, expect, it } from "vitest";

import {
  normalizeGroupOpenAIFast,
  normalizeGroupOpenAIFastDowngrade,
  supportsGroupOpenAIFast,
  supportsGroupOpenAIFastDowngrade,
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

  it("supports Fast downgrade only for composite groups", () => {
    expect(supportsGroupOpenAIFastDowngrade("composite")).toBe(true);
    expect(supportsGroupOpenAIFastDowngrade("openai")).toBe(false);
    expect(supportsGroupOpenAIFastDowngrade("anthropic")).toBe(false);
  });

  it("never allows Force Fast to be downgraded", () => {
    expect(normalizeGroupOpenAIFastDowngrade("composite", true, false)).toBe(true);
    expect(normalizeGroupOpenAIFastDowngrade("composite", true, true)).toBe(false);
    expect(normalizeGroupOpenAIFastDowngrade("openai", true, false)).toBe(false);
  });

  it("provides localized switch labels", () => {
    expect(zh.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      allowDowngrade: expect.any(String),
      allowDowngradeHint: expect.stringContaining("Composite"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
    expect(en.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      allowDowngrade: expect.any(String),
      allowDowngradeHint: expect.stringContaining("Composite"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
  });
});
