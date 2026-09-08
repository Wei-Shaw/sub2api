import { describe, expect, it } from "vitest";

import {
  diffSettingsPayload,
  fingerprintSettingsPayload,
  stableStringify,
} from "@/api/admin/settings";
import type { UpdateSettingsRequest } from "@/api/admin/settings";

// 设置页「只发变更字段」依赖这三个纯函数；这里钉住它们的关键语义。

describe("stableStringify", () => {
  it("对 key 顺序不敏感", () => {
    expect(stableStringify({ b: 1, a: [1, { y: null, x: "s" }] })).toBe(
      stableStringify({ a: [1, { x: "s", y: null }], b: 1 }),
    );
  });

  it("把 undefined 属性与键不存在归一为同一种缺失", () => {
    // 密钥留空（undefined）= 未变更 = 不发送，必须与键不存在等价。
    expect(stableStringify({ a: undefined, b: 1 })).toBe(
      stableStringify({ b: 1 }),
    );
    expect(stableStringify(undefined)).toBe("null");
  });

  it("区分真正会改变行为的值", () => {
    expect(stableStringify(false)).not.toBe(stableStringify(undefined));
    expect(stableStringify("")).not.toBe(stableStringify(null));
    expect(stableStringify(0)).not.toBe(stableStringify(null));
  });
});

describe("diffSettingsPayload", () => {
  it("只保留相对基线变化过的键", () => {
    const baseline = fingerprintSettingsPayload({
      site_name: "A",
      registration_enabled: true,
      default_platform_quotas: { anthropic: { daily: 1 } },
    } as unknown as UpdateSettingsRequest);

    const diff = diffSettingsPayload(
      {
        site_name: "B",
        registration_enabled: true,
        default_platform_quotas: { anthropic: { daily: 2 } },
      } as unknown as UpdateSettingsRequest,
      baseline,
    );

    expect(diff).toEqual({
      site_name: "B",
      default_platform_quotas: { anthropic: { daily: 2 } },
    });
  });

  it("嵌套对象内部相同（仅 key 顺序不同）不产生 diff", () => {
    const baseline = fingerprintSettingsPayload({
      default_platform_quotas: { anthropic: { daily: 1 }, openai: { weekly: 2 } },
    } as unknown as UpdateSettingsRequest);

    const diff = diffSettingsPayload(
      {
        default_platform_quotas: { openai: { weekly: 2 }, anthropic: { daily: 1 } },
      } as unknown as UpdateSettingsRequest,
      baseline,
    );

    expect(diff).toEqual({});
  });

  it("基线为 null（首次加载前）时原样返回全量 payload", () => {
    const payload = { site_name: "A" } as unknown as UpdateSettingsRequest;
    expect(diffSettingsPayload(payload, null)).toBe(payload);
  });

  it("密钥留空（undefined）与基线一致，不进入 diff", () => {
    const baseline = fingerprintSettingsPayload({
      smtp_password: undefined,
      site_name: "A",
    } as unknown as UpdateSettingsRequest);

    const diff = diffSettingsPayload(
      { smtp_password: undefined, site_name: "A" } as unknown as UpdateSettingsRequest,
      baseline,
    );

    expect(diff).toEqual({});
  });
});
