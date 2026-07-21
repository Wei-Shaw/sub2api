import type { GroupPlatform, ReasoningEffortMapping REDACTED from "@/types";

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
  REDACTED));
REDACTED

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
REDACTED

export interface ReasoningEffortMappingRow extends ReasoningEffortMapping {
  id: string;
REDACTED

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
  mapping: Partial<ReasoningEffortMapping> = {REDACTED,
): ReasoningEffortMappingRow {
  nextMappingRowID += 1;
  return {
    id: `reasoning-effort-mapping-${nextMappingRowIDREDACTED`,
    from: mapping.from ?? "",
    to: mapping.to ?? "",
  REDACTED;
REDACTED

export function reasoningEffortMappingsToRows(
  mappings?: ReasoningEffortMapping[] | null,
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingRow[] {
  return (mappings ?? []).flatMap((mapping) => {
    const from = normalizeReasoningEffortForPlatform(platform, mapping.from);
    const to = normalizeReasoningEffortForPlatform(platform, mapping.to);
    return from && to
      ? [createReasoningEffortMappingRow({ from, to REDACTED)]
      : [];
  REDACTED);
REDACTED

export function reasoningEffortMappingsToAPI(
  rows: ReasoningEffortMappingRow[],
): ReasoningEffortMapping[] {
  return rows.map((row) => ({
    from: row.from.trim(),
    to: row.to.trim(),
  REDACTED));
REDACTED

export function validateReasoningEffortMappings(
  rows: ReasoningEffortMappingRow[],
  platform: GroupPlatform = "openai",
): ReasoningEffortMappingErrors {
  const errors: ReasoningEffortMappingErrors = {REDACTED;
  const sourceRows = new Map<string, ReasoningEffortMappingRow[]>();

  rows.forEach((row) => {
    const from = row.from.trim();
    const to = row.to.trim();
    if (!from) {
      errors[row.id] = { ...errors[row.id], from: "fromRequired" REDACTED;
    REDACTED else if (!normalizeReasoningEffortForPlatform(platform, from)) {
      errors[row.id] = { ...errors[row.id], from: "unsupportedFrom" REDACTED;
    REDACTED else {
      const key = from.toLowerCase();
      sourceRows.set(key, [...(sourceRows.get(key) ?? []), row]);
    REDACTED
    if (!to) {
      errors[row.id] = { ...errors[row.id], to: "toRequired" REDACTED;
    REDACTED else if (!normalizeReasoningEffortForPlatform(platform, to)) {
      errors[row.id] = { ...errors[row.id], to: "unsupportedTo" REDACTED;
    REDACTED
  REDACTED);

  sourceRows.forEach((duplicateRows) => {
    if (duplicateRows.length < 2) return;
    duplicateRows.forEach((row) => {
      errors[row.id] = { ...errors[row.id], from: "duplicateFrom" REDACTED;
    REDACTED);
  REDACTED);

  return errors;
REDACTED
