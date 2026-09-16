import { describe, expect, it } from "vitest";
import { normalizeSupportedModelScopesForPlatform } from "../groupsSupportedModelScopes";

describe("normalizeSupportedModelScopesForPlatform", () => {
  it("preserves model scopes for Antigravity groups", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("antigravity", [
        "claude",
        "gemini_text",
      ]),
    ).toEqual(["claude", "gemini_text"]);
  });

  it("returns an empty array for Antigravity groups without scopes", () => {
    expect(normalizeSupportedModelScopesForPlatform("antigravity", undefined)).toEqual([]);
  });

  it("drops hidden model scopes for OpenAI groups", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("openai", [
        "claude",
        "gemini_text",
        "gemini_image",
      ]),
    ).toEqual([]);
  });

  it("drops hidden model scopes for other non-Antigravity groups", () => {
    expect(normalizeSupportedModelScopesForPlatform("claude", ["claude"])).toEqual([]);
  });
});
