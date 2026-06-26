import { describe, it, expect REDACTED from "vitest";
import {
  parseFingerprintSignalsToRows,
  serializeFingerprintRowsToJSON,
REDACTED from "../codexFingerprintSignals";

describe("codex fingerprint signals 行编解码", () => {
  it("解析: 变体数组 → / 合并字符串", () => {
    const rows = parseFingerprintSignalsToRows(
      '[{"type":"header_exact","match":["session-id","session_id"],"required":trueREDACTED]',
    );
    expect(rows).toEqual([
      { type: "header_exact", match: "session-id / session_id", required: true REDACTED,
    ]);
  REDACTED);
  it("序列化: / 合并 → 变体数组, required 透传", () => {
    const json = serializeFingerprintRowsToJSON([
      { type: "header_prefix", match: "x-codex-", required: true REDACTED,
      { type: "body_path", match: " a / b ", required: false REDACTED,
    ]);
    expect(JSON.parse(json)).toEqual([
      { type: "header_prefix", match: ["x-codex-"], required: true REDACTED,
      { type: "body_path", match: ["a", "b"], required: false REDACTED,
    ]);
  REDACTED);
  it("空/非法 → 空数组 / [] 串", () => {
    expect(parseFingerprintSignalsToRows("")).toEqual([]);
    expect(parseFingerprintSignalsToRows("nope")).toEqual([]);
    expect(serializeFingerprintRowsToJSON([])).toBe("[]");
  REDACTED);
REDACTED);
