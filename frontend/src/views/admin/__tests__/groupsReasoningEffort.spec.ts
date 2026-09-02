import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingPair,
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  normalizeReasoningEffortMatchType,
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

  it("hydrates model scoped mappings", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        {
          from: "max",
          to: "low",
          match_type: "prefix",
          model: " gpt ",
        },
        {
          from: "max",
          to: "medium",
          match_type: "exact",
          model: "gpt-5.4",
        },
      ],
      "openai",
    );

    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "low", match_type: "prefix", model: "gpt" },
      { from: "max", to: "medium", match_type: "exact", model: "gpt-5.4" },
    ]);
  });

  it("groups multiple request mappings under the same type and model", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
        { from: "xhigh", to: "medium", match_type: "prefix", model: "GPT" },
      ],
      "openai",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].match_type).toBe("prefix");
    expect(rows[0].model).toBe("gpt");
    expect(rows[0].pairs.map((pair) => pair.from)).toEqual(["high", "xhigh"]);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
      { from: "xhigh", to: "medium", match_type: "prefix", model: "gpt" },
    ]);
  });

  it("omits empty model scopes from the API payload", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
    });
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "max", to: "low" },
    ]);
  });

  it("hydrates suffix mappings", () => {
    const rows = reasoningEffortMappingsToRows(
      [{ from: "max", to: "low", match_type: "suffix", model: " mini " }],
      "openai",
    );
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "low", match_type: "suffix", model: "mini" },
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

  it("normalizes match type to exact, prefix, suffix, or empty", () => {
    expect(normalizeReasoningEffortMatchType("PREFIX")).toBe("prefix");
    expect(normalizeReasoningEffortMatchType("suffix")).toBe("suffix");
    expect(normalizeReasoningEffortMatchType("exact")).toBe("exact");
    expect(normalizeReasoningEffortMatchType("")).toBe("");
    expect(normalizeReasoningEffortMatchType("wildcard")).toBe("");
  });

  it("requires both sides of every mapping", () => {
    const first = createReasoningEffortMappingRow({ to: "low" });
    const second = createReasoningEffortMappingRow({ from: "max" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "fromRequired" },
      [second.pairs[0].id]: { to: "toRequired" },
    });
  });

  it("rejects duplicate source values case insensitively", () => {
    const first = createReasoningEffortMappingRow({ from: "MAX", to: "xhigh" });
    const second = createReasoningEffortMappingRow({ from: " max ", to: "high" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "duplicateFrom" },
      [second.pairs[0].id]: { from: "duplicateFrom" },
    });
  });

  it("allows the same source across different model scopes", () => {
    const prefix = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
      model: "gpt",
    });
    const exact = createReasoningEffortMappingRow({
      from: "max",
      to: "medium",
      match_type: "exact",
      model: "gpt-5.4",
    });
    const global = createReasoningEffortMappingRow({ from: "max", to: "high" });

    expect(validateReasoningEffortMappings([prefix, exact, global])).toEqual({});
  });

  it("allows multiple request values in one model scope", () => {
    const row = createReasoningEffortMappingRow({
      from: "high",
      to: "medium",
      match_type: "prefix",
      model: "gpt",
    });
    row.pairs.push(
      createReasoningEffortMappingPair({ from: "xhigh", to: "medium" }),
    );

    expect(validateReasoningEffortMappings([row])).toEqual({});
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
      { from: "xhigh", to: "medium", match_type: "prefix", model: "gpt" },
    ]);
  });

  it("rejects duplicate source values within the same model scope", () => {
    const first = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
      model: "GPT",
    });
    const second = createReasoningEffortMappingRow({
      from: "MAX",
      to: "high",
      match_type: "prefix",
      model: " gpt ",
    });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "duplicateFrom" },
      [second.pairs[0].id]: { from: "duplicateFrom" },
    });
  });

  it("allows empty type and model as a global mapping", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
    });
    expect(row.match_type).toBe("");
    expect(row.model).toBe("");
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({});
  });

  it("treats prefix without a model as a global mapping", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
    });
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({});
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "max", to: "low" },
    ]);
  });

  it("rejects custom mappings", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({
      [row.pairs[0].id]: { from: "unsupportedFrom" },
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
