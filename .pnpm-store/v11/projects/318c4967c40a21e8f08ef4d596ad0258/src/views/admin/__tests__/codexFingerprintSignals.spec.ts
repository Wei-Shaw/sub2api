import { describe, it, expect } from "vitest";
import {
  parseFingerprintSignalsToRows,
  serializeFingerprintRowsToJSON,
} from "../codexFingerprintSignals";

describe("codex fingerprint signals 行编解码", () => {
  it("解析: 变体数组 → / 合并字符串", () => {
    const rows = parseFingerprintSignalsToRows(
      '[{"type":"header_exact","match":["session-id","session_id"],"required":true}]',
    );
    expect(rows).toEqual([
      { type: "header_exact", match: "session-id / session_id", required: true },
    ]);
  });
  it("序列化: / 合并 → 变体数组, required 透传", () => {
    const json = serializeFingerprintRowsToJSON([
      { type: "header_prefix", match: "x-codex-", required: true },
      { type: "body_path", match: " a / b ", required: false },
    ]);
    expect(JSON.parse(json)).toEqual([
      { type: "header_prefix", match: ["x-codex-"], required: true },
      { type: "body_path", match: ["a", "b"], required: false },
    ]);
  });
  it("空/非法 → 空数组 / [] 串", () => {
    expect(parseFingerprintSignalsToRows("")).toEqual([]);
    expect(parseFingerprintSignalsToRows("nope")).toEqual([]);
    expect(serializeFingerprintRowsToJSON([])).toBe("[]");
  });
});
