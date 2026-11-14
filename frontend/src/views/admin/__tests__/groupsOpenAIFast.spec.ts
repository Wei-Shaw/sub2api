import { describe, expect, it } from "vitest";

import {
  groupOpenAIForcedTier,
  normalizeGroupOpenAIFast,
  setGroupOpenAIForcedTier,
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

  it("derives the forced tier from the two mutually exclusive flags", () => {
    expect(groupOpenAIForcedTier({})).toBe("client");
    expect(
      groupOpenAIForcedTier({
        force_openai_fast: false,
        force_openai_ultrafast: false,
      }),
    ).toBe("client");
    expect(groupOpenAIForcedTier({ force_openai_fast: true })).toBe("priority");
    expect(groupOpenAIForcedTier({ force_openai_ultrafast: true })).toBe(
      "ultrafast",
    );
  });

  it("never leaves both forced-tier flags enabled", () => {
    const flags = { force_openai_fast: true, force_openai_ultrafast: false };

    setGroupOpenAIForcedTier(flags, "ultrafast");
    expect(flags).toEqual({
      force_openai_fast: false,
      force_openai_ultrafast: true,
    });

    setGroupOpenAIForcedTier(flags, "priority");
    expect(flags).toEqual({
      force_openai_fast: true,
      force_openai_ultrafast: false,
    });

    setGroupOpenAIForcedTier(flags, "client");
    expect(flags).toEqual({
      force_openai_fast: false,
      force_openai_ultrafast: false,
    });

    setGroupOpenAIForcedTier(flags, "ultrafast");
    setGroupOpenAIForcedTier(flags, null);
    expect(flags).toEqual({
      force_openai_fast: false,
      force_openai_ultrafast: false,
    });
  });

  it("provides localized forced-tier labels", () => {
    for (const locale of [zh, en]) {
      expect(locale.groups.openaiFast).toMatchObject({
        forcedTier: expect.any(String),
        followClient: expect.any(String),
        forceUltrafast: expect.stringContaining("ultrafast"),
        hint: expect.stringContaining("service_tier=ultrafast"),
      });
    }
  });

  it("provides localized switch labels", () => {
    expect(zh.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
    expect(en.groups.openaiFast).toMatchObject({
      title: expect.any(String),
      force: expect.any(String),
      hint: expect.stringContaining("service_tier=priority"),
      free: expect.any(String),
      freeHint: expect.stringContaining("Standard"),
    });
  });
});
