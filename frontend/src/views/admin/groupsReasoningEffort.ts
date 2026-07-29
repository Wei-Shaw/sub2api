import type { GroupPlatform, ReasoningEffortMapping } from "@/types";

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
