import { describe, expect, it REDACTED from "vitest";

import {
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  validateReasoningEffortMappings,
REDACTED from "../groupsReasoningEffort";

describe("groupsReasoningEffort", () => {
  it("provides fixed OpenAI choices without none", () => {
    expect(
      reasoningEffortOptionsForPlatform("openai").map((option) => option.value),
    ).toEqual([
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ]);
    for (const platform of [
      "anthropic",
      "gemini",
      "antigravity",
      "grok",
    ] as const) {
      expect(reasoningEffortOptionsForPlatform(platform)).toEqual([]);
    REDACTED
  REDACTED);

  it("hydrates supported rows and drops stale custom values", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: " max ", to: " xhigh " REDACTED,
        { from: "ultra", to: "high" REDACTED,
      ],
      "openai",
    );

    expect(rows).toHaveLength(1);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" REDACTED,
    ]);
  REDACTED);

  it("clears values unsupported by OpenAI or used on another platform", () => {
    expect(normalizeReasoningEffortForPlatform("openai", " MAX ")).toBe("max");
    expect(normalizeReasoningEffortForPlatform("grok", "max")).toBe("");
    expect(normalizeReasoningEffortForPlatform("openai", "none")).toBe("");
  REDACTED);

  it("requires both sides of every mapping", () => {
    const first = createReasoningEffortMappingRow({ to: "low" REDACTED);
    const second = createReasoningEffortMappingRow({ from: "max" REDACTED);

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { from: "fromRequired" REDACTED,
      [second.id]: { to: "toRequired" REDACTED,
    REDACTED);
  REDACTED);

  it("rejects duplicate source values case insensitively", () => {
    const first = createReasoningEffortMappingRow({ from: "MAX", to: "xhigh" REDACTED);
    const second = createReasoningEffortMappingRow({ from: " max ", to: "high" REDACTED);

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { from: "duplicateFrom" REDACTED,
      [second.id]: { from: "duplicateFrom" REDACTED,
    REDACTED);
  REDACTED);

  it("rejects custom mappings", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" REDACTED);
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({
      [row.id]: { from: "unsupportedFrom" REDACTED,
    REDACTED);
  REDACTED);
REDACTED);
