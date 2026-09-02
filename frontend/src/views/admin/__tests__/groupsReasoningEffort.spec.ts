import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  normalizeReasoningEffortOverLimit,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  reasoningEffortOverLimitDeny,
  reasoningEffortOverLimitDowngrade,
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

  it("normalizes over-limit access control to downgrade or deny", () => {
    expect(normalizeReasoningEffortOverLimit(undefined)).toBe(
      reasoningEffortOverLimitDowngrade,
    );
    expect(normalizeReasoningEffortOverLimit("")).toBe(
      reasoningEffortOverLimitDowngrade,
    );
    expect(normalizeReasoningEffortOverLimit(" downgrade ")).toBe(
      reasoningEffortOverLimitDowngrade,
    );
    expect(normalizeReasoningEffortOverLimit("DENY")).toBe(
      reasoningEffortOverLimitDeny,
    );
    expect(normalizeReasoningEffortOverLimit("block")).toBe(
      reasoningEffortOverLimitDowngrade,
    );
  });
});
