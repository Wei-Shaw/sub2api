import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  supportsReasoningEffortPolicyPlatform,
  validateReasoningEffortMappings,
} from "../groupsReasoningEffort";

describe("groupsReasoningEffort", () => {
  it("provides fixed OpenAI choices to OpenAI and Composite groups", () => {
    const expected = [
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ];
    for (const platform of ["openai", "composite"] as const) {
      expect(
        reasoningEffortOptionsForPlatform(platform).map(
          (option) => option.value,
        ),
      ).toEqual(expected);
      expect(supportsReasoningEffortPolicyPlatform(platform)).toBe(true);
    }
    for (const platform of [
      "anthropic",
      "gemini",
      "antigravity",
      "grok",
    ] as const) {
      expect(reasoningEffortOptionsForPlatform(platform)).toEqual([]);
      expect(supportsReasoningEffortPolicyPlatform(platform)).toBe(false);
    }
  });

  it("hydrates and serializes model-scoped custom sources", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: " max ", to: " xhigh " },
        { model: " GPT-5.6-SOL ", from: " Ultra ", to: "high" },
      ],
      "openai",
    );

    expect(rows).toHaveLength(2);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" },
      { model: "gpt-5.6-sol", from: "ultra", to: "high" },
    ]);
  });

  it("clears values unsupported by OpenAI or used on another platform", () => {
    expect(normalizeReasoningEffortForPlatform("openai", " MAX ")).toBe("max");
    expect(normalizeReasoningEffortForPlatform("composite", " MAX ")).toBe(
      "max",
    );
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

  it("rejects duplicate model and source pairs case insensitively", () => {
    const first = createReasoningEffortMappingRow({
      model: "gpt-5.6-sol",
      from: "ULTRA",
      to: "xhigh",
    });
    const second = createReasoningEffortMappingRow({
      model: " GPT-5.6-SOL ",
      from: " ultra ",
      to: "high",
    });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { from: "duplicateFrom" },
      [second.id]: { from: "duplicateFrom" },
    });
  });

  it("accepts custom sources but keeps targets fixed", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({});

    const invalidTarget = createReasoningEffortMappingRow({
      from: "ultra",
      to: "future",
    });
    expect(validateReasoningEffortMappings([invalidTarget], "openai")).toEqual({
      [invalidTarget.id]: { to: "unsupportedTo" },
    });

    const noneSource = createReasoningEffortMappingRow({
      from: "none",
      to: "low",
    });
    expect(validateReasoningEffortMappings([noneSource], "openai")).toEqual({
      [noneSource.id]: { from: "unsupportedFrom" },
    });
  });

  it("allows the same source in different model scopes", () => {
    const global = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    const scoped = createReasoningEffortMappingRow({
      model: "gpt-5.6-sol",
      from: "ultra",
      to: "xhigh",
    });
    expect(validateReasoningEffortMappings([global, scoped], "openai")).toEqual({});
  });
});
