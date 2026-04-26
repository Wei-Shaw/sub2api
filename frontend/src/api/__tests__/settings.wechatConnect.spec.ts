import { describe, expect, it } from "vitest";

import {
  defaultWeChatConnectScopesForMode,
  normalizeWeChatConnectMode,
} from "@/api/admin/settings";

describe("admin settings wechat connect helpers", () => {
  it("normalizes legacy or noisy mode values to the backend contract", () => {
    expect(normalizeWeChatConnectMode("OPEN")).toBe("open");
    expect(normalizeWeChatConnectMode(" open_platform ")).toBe("open");
    expect(normalizeWeChatConnectMode("mp")).toBe("mp");
    expect(normalizeWeChatConnectMode("official_account")).toBe("mp");
    expect(normalizeWeChatConnectMode("unknown")).toBe("open");
  });

  it("maps each mode to the backend default scopes", () => {
    expect(defaultWeChatConnectScopesForMode("open")).toBe("snsapi_login");
    expect(defaultWeChatConnectScopesForMode("mp")).toBe("snsapi_userinfo");
  });
});
