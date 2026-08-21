import type { GroupPlatform, ReasoningEffortMapping } from "@/types";

const openAIReasoningEffortValues = [
  "minimal",
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
] as const;

const maxReasoningEffortMappingValueLength = 64;

const countUnicodeCodePoints = (value: string): number => Array.from(value).length;

const reasoningEffortValuesForPlatform = (
  platform: GroupPlatform,
): readonly string[] =>
  supportsReasoningEffortPolicyPlatform(platform)
    ? openAIReasoningEffortValues
    : [];

export function supportsReasoningEffortPolicyPlatform(
  platform: GroupPlatform,
): boolean {
  return platform === "openai" || platform === "composite";
}

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

export function normalizeReasoningEffortMappingSource(
  value: string | null | undefined,
): string {
  const normalized = value?.trim().toLowerCase() ?? "";
  const compact = normalized.replace(/[-_ ]/g, "");
  if (compact === "extrahigh" || compact === "xhigh") return "xhigh";
  if (
    compact === "minimal" ||
    compact === "low" ||
    compact === "medium" ||
    compact === "high" ||
    compact === "max"
  ) {
    return compact;
  }
  return normalized;
}

export function normalizeReasoningEffortMappingModel(
  value: string | null | undefined,
): string {
  return value?.trim().toLowerCase() ?? "";
}

export interface ReasoningEffortMappingRow extends ReasoningEffortMapping {
  id: string;
}

export type ReasoningEffortMappingErrorCode =
  | "fromRequired"
  | "toRequired"
  | "duplicateFrom"
  | "unsupportedFrom"
  | "unsupportedTo"
  | "modelTooLong"
  | "fromTooLong";

export type ReasoningEffortMappingErrors = Record<
  string,
  Partial<Record<"model" | "from" | "to", ReasoningEffortMappingErrorCode>>
>;

let nextMappingRowID = 0;

export function createReasoningEffortMappingRow(
  mapping: Partial<ReasoningEffortMapping> = {},
): ReasoningEffortMappingRow {
  nextMappingRowID += 1;
  return {
    id: `reasoning-effort-mapping-${nextMappingRowID}`,
    model: mapping.model ?? "",
    from: mapping.from ?? "",
    to: mapping.to ?? "",
  };
}

export function reasoningEffortMappingsToRows(
  mappings?: ReasoningEffortMapping[] | null,
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingRow[] {
  return (mappings ?? []).flatMap((mapping) => {
    const model = normalizeReasoningEffortMappingModel(mapping.model);
    const from = normalizeReasoningEffortMappingSource(mapping.from);
    const to = normalizeReasoningEffortForPlatform(platform, mapping.to);
    return from && to
      ? [createReasoningEffortMappingRow({ model, from, to })]
      : [];
  });
}

export function reasoningEffortMappingsToAPI(
  rows: ReasoningEffortMappingRow[],
): ReasoningEffortMapping[] {
  return rows.map((row) => {
    const model = normalizeReasoningEffortMappingModel(row.model);
    return {
      ...(model ? { model } : {}),
      from: normalizeReasoningEffortMappingSource(row.from),
      to: row.to.trim().toLowerCase(),
    };
  });
}

export function validateReasoningEffortMappings(
  rows: ReasoningEffortMappingRow[],
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingErrors {
  const errors: ReasoningEffortMappingErrors = {};
  const sourceRows = new Map<string, ReasoningEffortMappingRow[]>();

  rows.forEach((row) => {
    const model = normalizeReasoningEffortMappingModel(row.model);
    const from = normalizeReasoningEffortMappingSource(row.from);
    const to = row.to.trim();
    if (countUnicodeCodePoints(model) > maxReasoningEffortMappingValueLength) {
      errors[row.id] = { ...errors[row.id], model: "modelTooLong" };
    }
    if (!from) {
      errors[row.id] = { ...errors[row.id], from: "fromRequired" };
    } else if (from === "none") {
      errors[row.id] = { ...errors[row.id], from: "unsupportedFrom" };
    } else if (countUnicodeCodePoints(from) > maxReasoningEffortMappingValueLength) {
      errors[row.id] = { ...errors[row.id], from: "fromTooLong" };
    } else if (!supportsReasoningEffortPolicyPlatform(platform)) {
      errors[row.id] = { ...errors[row.id], from: "unsupportedFrom" };
    } else {
      const key = `${model}\u0000${from}`;
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
