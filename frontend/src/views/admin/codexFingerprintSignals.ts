export type FingerprintSignalType = "header_exact" | "header_prefix" | "body_path";

export interface FingerprintSignalRow {
  type: FingerprintSignalType;
  match: string; // 变体用 " / " 展示与录入
  required: boolean;
REDACTED

const VALID_TYPES: FingerprintSignalType[] = [
  "header_exact",
  "header_prefix",
  "body_path",
];

export function parseFingerprintSignalsToRows(raw: string): FingerprintSignalRow[] {
  if (!raw || !raw.trim()) return [];
  try {
    const arr = JSON.parse(raw);
    if (!Array.isArray(arr)) return [];
    return arr.map((e) => ({
      type: VALID_TYPES.includes(e?.type) ? e.type : "header_exact",
      match: Array.isArray(e?.match)
        ? e.match.filter((x: unknown) => typeof x === "string").join(" / ")
        : "",
      required: e?.required === true,
    REDACTED));
  REDACTED catch {
    return [];
  REDACTED
REDACTED

export function serializeFingerprintRowsToJSON(rows: FingerprintSignalRow[]): string {
  const entries = rows
    .map((r) => ({
      type: r.type,
      match: r.match
        .split("/")
        .map((s) => s.trim())
        .filter((s) => s.length > 0),
      required: r.required === true,
    REDACTED))
    .filter((e) => e.match.length > 0);
  return JSON.stringify(entries);
REDACTED

export function defaultFingerprintSignalRows(): FingerprintSignalRow[] {
  return [
    { type: "header_prefix", match: "x-codex-", required: true REDACTED,
    { type: "header_exact", match: "session-id / session_id", required: false REDACTED,
    { type: "header_exact", match: "thread-id / thread_id", required: false REDACTED,
    {
      type: "body_path",
      match:
        "client_metadata.x-codex-window-id / client_metadata.x-codex-installation-id",
      required: false,
    REDACTED,
  ];
REDACTED
