import type {
  GroupPlatform,
  ModelReasoningEffortRule,
  ReasoningEffortMapping,
} from "@/types";

const openAIReasoningEffortValues = [
  "minimal",
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
] as const;

const reasoningEffortValuesForPlatform = (
  platform: GroupPlatform,
): readonly string[] =>
  platform === "openai" ? openAIReasoningEffortValues : [];

export function reasoningEffortOptionsForPlatform(platform: GroupPlatform) {
  return reasoningEffortValuesForPlatform(platform).map((value) => ({
    value,
    label: value,
  }));
}

export function normalizeReasoningEffortForPlatform(
  platform: GroupPlatform,
  value: string | null | undefined,
): string {
  const normalized = value?.trim().toLowerCase() ?? "";
  return reasoningEffortValuesForPlatform(platform).some(
    (allowed) => allowed === normalized,
  )
    ? normalized
    : "";
}

export interface ReasoningEffortMappingRow extends ReasoningEffortMapping {
  id: string;
}

export type ReasoningEffortMappingErrorCode =
  | "fromRequired"
  | "toRequired"
  | "duplicateFrom"
  | "unsupportedFrom"
  | "unsupportedTo";

export type ReasoningEffortMappingErrors = Record<
  string,
  Partial<Record<"from" | "to", ReasoningEffortMappingErrorCode>>
>;

let nextMappingRowID = 0;
let nextModelRuleRowID = 0;

export function createReasoningEffortMappingRow(
  mapping: Partial<ReasoningEffortMapping> = {},
): ReasoningEffortMappingRow {
  nextMappingRowID += 1;
  return {
    id: `reasoning-effort-mapping-${nextMappingRowID}`,
    from: mapping.from ?? "",
    to: mapping.to ?? "",
  };
}

export function reasoningEffortMappingsToRows(
  mappings?: ReasoningEffortMapping[] | null,
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingRow[] {
  return (mappings ?? []).flatMap((mapping) => {
    const from = normalizeReasoningEffortForPlatform(platform, mapping.from);
    const to = normalizeReasoningEffortForPlatform(platform, mapping.to);
    return from && to
      ? [createReasoningEffortMappingRow({ from, to })]
      : [];
  });
}

export function reasoningEffortMappingsToAPI(
  rows: ReasoningEffortMappingRow[],
): ReasoningEffortMapping[] {
  return rows.map((row) => ({
    from: row.from.trim(),
    to: row.to.trim(),
  }));
}

export function validateReasoningEffortMappings(
  rows: ReasoningEffortMappingRow[],
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingErrors {
  const errors: ReasoningEffortMappingErrors = {};
  const sourceRows = new Map<string, ReasoningEffortMappingRow[]>();

  rows.forEach((row) => {
    const from = row.from.trim();
    const to = row.to.trim();
    if (!from) {
      errors[row.id] = { ...errors[row.id], from: "fromRequired" };
    } else if (!normalizeReasoningEffortForPlatform(platform, from)) {
      errors[row.id] = { ...errors[row.id], from: "unsupportedFrom" };
    } else {
      const key = from.toLowerCase();
      sourceRows.set(key, [...(sourceRows.get(key) ?? []), row]);
    }
    if (!to) {
      errors[row.id] = { ...errors[row.id], to: "toRequired" };
    } else if (!normalizeReasoningEffortForPlatform(platform, to)) {
      errors[row.id] = { ...errors[row.id], to: "unsupportedTo" };
    }
  });

  sourceRows.forEach((duplicateRows) => {
    if (duplicateRows.length < 2) return;
    duplicateRows.forEach((row) => {
      errors[row.id] = { ...errors[row.id], from: "duplicateFrom" };
    });
  });

  return errors;
}

export interface ModelReasoningEffortRuleRow {
  id: string;
  model: string;
  max_reasoning_effort: string;
  reasoning_effort_mappings: ReasoningEffortMappingRow[];
}

export type ModelReasoningEffortRuleErrorCode =
  | "modelRequired"
  | "duplicateModel";

export interface ModelReasoningEffortRuleErrors {
  model?: ModelReasoningEffortRuleErrorCode;
  mappings: ReasoningEffortMappingErrors;
}

export function createModelReasoningEffortRuleRow(
  rule: Partial<ModelReasoningEffortRule> = {},
  platform: GroupPlatform = "openai",
): ModelReasoningEffortRuleRow {
  nextModelRuleRowID += 1;
  return {
    id: `model-reasoning-effort-rule-${nextModelRuleRowID}`,
    model: rule.model ?? "",
    max_reasoning_effort: normalizeReasoningEffortForPlatform(
      platform,
      rule.max_reasoning_effort,
    ),
    reasoning_effort_mappings: reasoningEffortMappingsToRows(
      rule.reasoning_effort_mappings,
      platform,
    ),
  };
}

export function modelReasoningEffortRulesToRows(
  rules?: ModelReasoningEffortRule[] | null,
  platform: GroupPlatform = "openai",
): ModelReasoningEffortRuleRow[] {
  if (platform !== "openai") return [];
  return (rules ?? []).map((rule) =>
    createModelReasoningEffortRuleRow(rule, platform),
  );
}

export function modelReasoningEffortRulesToAPI(
  rows: ModelReasoningEffortRuleRow[],
): ModelReasoningEffortRule[] {
  return rows.map((row) => ({
    model: row.model.trim(),
    max_reasoning_effort: row.max_reasoning_effort.trim(),
    reasoning_effort_mappings: reasoningEffortMappingsToAPI(
      row.reasoning_effort_mappings,
    ),
  }));
}

export function validateModelReasoningEffortRules(
  rows: ModelReasoningEffortRuleRow[],
  platform: GroupPlatform = "openai",
): Record<string, ModelReasoningEffortRuleErrors> {
  const errors: Record<string, ModelReasoningEffortRuleErrors> = {};
  const modelRows = new Map<string, ModelReasoningEffortRuleRow[]>();

  rows.forEach((row) => {
    const model = row.model.trim();
    const mappings = validateReasoningEffortMappings(
      row.reasoning_effort_mappings,
      platform,
    );
    if (!model) {
      errors[row.id] = { model: "modelRequired", mappings };
    } else {
      modelRows.set(model, [...(modelRows.get(model) ?? []), row]);
      if (Object.keys(mappings).length > 0) {
        errors[row.id] = { mappings };
      }
    }
  });

  modelRows.forEach((duplicateRows) => {
    if (duplicateRows.length < 2) return;
    duplicateRows.forEach((row) => {
      errors[row.id] = {
        ...errors[row.id],
        model: "duplicateModel",
        mappings: errors[row.id]?.mappings ?? {},
      };
    });
  });

  return errors;
}
