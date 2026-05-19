import { describe, expect, it REDACTED from "vitest";
import { normalizeSupportedModelScopesForPlatform REDACTED from "../groupsSupportedModelScopes";

describe("normalizeSupportedModelScopesForPlatform", () => {
  it("preserves model scopes for Antigravity groups", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("antigravity", [
        "claude",
        "gemini_text",
      ]),
    ).toEqual(["claude", "gemini_text"]);
  REDACTED);

  it("returns an empty array for Antigravity groups without scopes", () => {
    expect(normalizeSupportedModelScopesForPlatform("antigravity", undefined)).toEqual([]);
  REDACTED);

  it("drops hidden model scopes for OpenAI groups", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("openai", [
        "claude",
        "gemini_text",
        "gemini_image",
      ]),
    ).toEqual([]);
  REDACTED);

  it("drops hidden model scopes for other non-Antigravity groups", () => {
    expect(normalizeSupportedModelScopesForPlatform("claude", ["claude"])).toEqual([]);
  REDACTED);
REDACTED);
