import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingRow,
  createModelReasoningEffortRuleRow,
  modelReasoningEffortRulesToAPI,
  modelReasoningEffortRulesToRows,
  normalizeReasoningEffortForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  validateReasoningEffortMappings,
  validateModelReasoningEffortRules,
} from "../groupsReasoningEffort";

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
    }
  });

  it("hydrates supported rows and drops stale custom values", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: " max ", to: " xhigh " },
        { from: "ultra", to: "high" },
      ],
      "openai",
    );

    expect(rows).toHaveLength(1);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" },
    ]);
  });

  it("clears values unsupported by OpenAI or used on another platform", () => {
    expect(normalizeReasoningEffortForPlatform("openai", " MAX ")).toBe("max");
    expect(normalizeReasoningEffortForPlatform("grok", "max")).toBe("");
    expect(normalizeReasoningEffortForPlatform("openai", "none")).toBe("");
  });

  it("requires both sides of every mapping", () => {
    const first = createReasoningEffortMappingRow({ to: "low" });
    const second = createReasoningEffortMappingRow({ from: "max" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { from: "fromRequired" },
      [second.id]: { to: "toRequired" },
    });
  });

  it("rejects duplicate source values case insensitively", () => {
    const first = createReasoningEffortMappingRow({ from: "MAX", to: "xhigh" });
    const second = createReasoningEffortMappingRow({ from: " max ", to: "high" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { from: "duplicateFrom" },
      [second.id]: { from: "duplicateFrom" },
    });
  });

  it("rejects custom mappings", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({
      [row.id]: { from: "unsupportedFrom" },
    });
  });

  it("round trips exact model rules including explicit unlimited overrides", () => {
    const rows = modelReasoningEffortRulesToRows([
      {
        model: " gpt-5.6-luna ",
        max_reasoning_effort: "",
        reasoning_effort_mappings: [],
      },
      {
        model: "gpt-5.6-sol",
        max_reasoning_effort: "medium",
        reasoning_effort_mappings: [{ from: "max", to: "xhigh" }],
      },
    ]);

    expect(modelReasoningEffortRulesToAPI(rows)).toEqual([
      {
        model: "gpt-5.6-luna",
        max_reasoning_effort: "",
        reasoning_effort_mappings: [],
      },
      {
        model: "gpt-5.6-sol",
        max_reasoning_effort: "medium",
        reasoning_effort_mappings: [{ from: "max", to: "xhigh" }],
      },
    ]);
  });

  it("requires unique exact model names and validates nested mappings", () => {
    const first = createModelReasoningEffortRuleRow({
      model: "gpt-5.6-luna",
    });
    first.reasoning_effort_mappings = [
      createReasoningEffortMappingRow({ from: "max" }),
    ];
    const second = createModelReasoningEffortRuleRow({
      model: " gpt-5.6-luna ",
    });
    const third = createModelReasoningEffortRuleRow();

    expect(
      validateModelReasoningEffortRules([first, second, third]),
    ).toEqual({
      [first.id]: {
        model: "duplicateModel",
        mappings: {
          [first.reasoning_effort_mappings[0].id]: { to: "toRequired" },
        },
      },
      [second.id]: {
        model: "duplicateModel",
        mappings: {},
      },
      [third.id]: {
        model: "modelRequired",
        mappings: {},
      },
    });
  });
});
