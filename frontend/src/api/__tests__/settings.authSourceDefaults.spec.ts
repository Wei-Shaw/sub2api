import { describe, expect, it } from "vitest";

import {
  appendAuthSourceDefaultsToUpdateRequest,
  buildAuthSourceDefaultsState,
  normalizePlatformQuotasMap,
  sanitizePlatformQuotasMap,
  type UpdateSettingsRequest,
  type DefaultPlatformQuotasMap,
} from "@/api/admin/settings";

/** 全 null 的 4 平台 map，用于断言归一化默认值 */
const nullQuota = { five_hour: null, daily: null, weekly: null, monthly: null, five_hour_align_minutes: 0 }
const allNullQuotas: DefaultPlatformQuotasMap = {
  anthropic: { ...nullQuota },
  openai:    { ...nullQuota },
  gemini:    { ...nullQuota },
  antigravity: { ...nullQuota },
}

describe("admin settings auth source defaults helpers", () => {
  it("builds auth source defaults state from flat settings fields", () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_balance: 9.5,
      auth_source_default_email_concurrency: 3,
      auth_source_default_email_subscriptions: [
        { group_id: 1, validity_days: 30 },
      ],
      auth_source_default_email_grant_on_signup: false,
      auth_source_default_email_grant_on_first_bind: true,
      auth_source_default_linuxdo_balance: 6,
      auth_source_default_linuxdo_concurrency: 8,
      auth_source_default_linuxdo_subscriptions: [
        { group_id: 2, validity_days: 60 },
      ],
      auth_source_default_linuxdo_grant_on_signup: true,
      auth_source_default_linuxdo_grant_on_first_bind: false,
    });

    expect(state.email).toEqual({
      balance: 9.5,
      concurrency: 3,
      subscriptions: [{ group_id: 1, validity_days: 30 }],
      grant_on_signup: false,
      grant_on_first_bind: true,
      platform_quotas: allNullQuotas,
    });
    expect(state.linuxdo).toEqual({
      balance: 6,
      concurrency: 8,
      subscriptions: [{ group_id: 2, validity_days: 60 }],
      grant_on_signup: true,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
    expect(state.oidc).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
    expect(state.wechat).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
  });

  it("defaults grant-on-signup to disabled when settings are missing", () => {
    const state = buildAuthSourceDefaultsState({});

    expect(state.email.grant_on_signup).toBe(false);
    expect(state.linuxdo.grant_on_signup).toBe(false);
    expect(state.oidc.grant_on_signup).toBe(false);
    expect(state.wechat.grant_on_signup).toBe(false);
  });

  it("reads nested platform_quotas from settings into auth source state", () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_platform_quotas: {
        anthropic: { five_hour: 5, daily: 10, weekly: 50, monthly: 200, five_hour_align_minutes: 60 },
        openai:    { ...nullQuota },
      } as DefaultPlatformQuotasMap,
    });

    // anthropic 填写的值应被保留
    expect(state.email.platform_quotas.anthropic).toEqual({ five_hour: 5, daily: 10, weekly: 50, monthly: 200, five_hour_align_minutes: 60 });
    // openai 全 null 应被保留
    expect(state.email.platform_quotas.openai).toEqual(nullQuota);
    // 未出现的平台（gemini/antigravity）归一化为 null
    expect(state.email.platform_quotas.gemini).toEqual(nullQuota);
    expect(state.email.platform_quotas.antigravity).toEqual(nullQuota);
  });

  it("appends auth source defaults back onto update payload", () => {
    const payload: UpdateSettingsRequest = {
      site_name: "Sub2API",
    };

    appendAuthSourceDefaultsToUpdateRequest(payload, {
      email: {
        balance: 1.25,
        concurrency: 2,
        subscriptions: [{ group_id: 3, validity_days: 7 }],
        grant_on_signup: true,
        grant_on_first_bind: false,
        platform_quotas: {},
      },
      linuxdo: {
        balance: 0,
        concurrency: 6,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: true,
        platform_quotas: {},
      },
      oidc: {
        balance: 4,
        concurrency: 9,
        subscriptions: [{ group_id: 9, validity_days: 90 }],
        grant_on_signup: true,
        grant_on_first_bind: true,
        platform_quotas: {},
      },
      wechat: {
        balance: 2,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {},
      },
      github: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {},
      },
      google: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {},
      },
      dingtalk: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {},
      },
    });

    expect(payload).toMatchObject({
      site_name: "Sub2API",
      auth_source_default_email_balance: 1.25,
      auth_source_default_email_concurrency: 2,
      auth_source_default_email_subscriptions: [
        { group_id: 3, validity_days: 7 },
      ],
      auth_source_default_email_grant_on_signup: true,
      auth_source_default_email_grant_on_first_bind: false,
      auth_source_default_linuxdo_balance: 0,
      auth_source_default_linuxdo_concurrency: 6,
      auth_source_default_linuxdo_subscriptions: [],
      auth_source_default_linuxdo_grant_on_signup: false,
      auth_source_default_linuxdo_grant_on_first_bind: true,
      auth_source_default_oidc_balance: 4,
      auth_source_default_oidc_concurrency: 9,
      auth_source_default_oidc_subscriptions: [
        { group_id: 9, validity_days: 90 },
      ],
      auth_source_default_oidc_grant_on_signup: true,
      auth_source_default_oidc_grant_on_first_bind: true,
      auth_source_default_wechat_balance: 2,
      auth_source_default_wechat_concurrency: 5,
      auth_source_default_wechat_subscriptions: [],
      auth_source_default_wechat_grant_on_signup: false,
      auth_source_default_wechat_grant_on_first_bind: false,
      // 嵌套 platform_quotas 字段
      auth_source_default_email_platform_quotas: allNullQuotas,
      auth_source_default_linuxdo_platform_quotas: allNullQuotas,
      auth_source_default_oidc_platform_quotas: allNullQuotas,
      auth_source_default_wechat_platform_quotas: allNullQuotas,
      auth_source_default_github_platform_quotas: allNullQuotas,
      auth_source_default_google_platform_quotas: allNullQuotas,
      auth_source_default_dingtalk_platform_quotas: allNullQuotas,
    });
  });

  it("appends sanitized nested platform_quotas with non-null values in update payload", () => {
    const payload: UpdateSettingsRequest = {};
    appendAuthSourceDefaultsToUpdateRequest(payload, {
      email: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {
          anthropic: { five_hour: 5, daily: 10, weekly: 50, monthly: 200, five_hour_align_minutes: 60 },
          openai:    { five_hour: null, daily: 0, weekly: null, monthly: null, five_hour_align_minutes: 0 },
        },
      },
      linuxdo: { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
      oidc:    { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
      wechat:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
      github:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
      google:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
      dingtalk: { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {} },
    });

    const emailQuotas = (payload as Record<string, unknown>)["auth_source_default_email_platform_quotas"] as DefaultPlatformQuotasMap;
    expect(emailQuotas.anthropic).toEqual({ five_hour: 5, daily: 10, weekly: 50, monthly: 200, five_hour_align_minutes: 60 });
    // 0 是合法值（不限额=0 与"不设"不同，保留）
    expect(emailQuotas.openai?.daily).toBe(0);
    // 缺失平台归一化为全 null
    expect(emailQuotas.gemini).toEqual(nullQuota);
    expect(emailQuotas.antigravity).toEqual(nullQuota);
  });
});

describe("normalizePlatformQuotasMap", () => {
  it("填充缺失的平台为全 null 三档", () => {
    const result = normalizePlatformQuotasMap({ anthropic: { five_hour: 2, daily: 5, weekly: null, monthly: null, five_hour_align_minutes: 60 } });
    expect(result.anthropic).toEqual({ five_hour: 2, daily: 5, weekly: null, monthly: null, five_hour_align_minutes: 60 });
    expect(result.openai).toEqual(nullQuota);
    expect(result.gemini).toEqual(nullQuota);
    expect(result.antigravity).toEqual(nullQuota);
  });

  it("无参数时返回全 4 平台全 null", () => {
    const result = normalizePlatformQuotasMap();
    expect(Object.keys(result)).toHaveLength(4);
    for (const v of Object.values(result)) {
      expect(v).toEqual(nullQuota);
    }
  });

  it("非 number 类型的值归一化为 null", () => {
    const result = normalizePlatformQuotasMap({
      anthropic: { five_hour: "5" as unknown as number, daily: "50" as unknown as number, weekly: undefined as unknown as number, monthly: null, five_hour_align_minutes: "60" as unknown as number },
    });
    expect(result.anthropic).toEqual(nullQuota);
  });
});

describe("sanitizePlatformQuotasMap", () => {
  it("保留合法的正数和零值", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { five_hour: 5, daily: 10.5, weekly: 0, monthly: null, five_hour_align_minutes: 60 },
    });
    expect(result.anthropic?.five_hour).toBe(5);
    expect(result.anthropic?.daily).toBe(10.5);
    expect(result.anthropic?.weekly).toBe(0);
    expect(result.anthropic?.monthly).toBe(null);
    expect(result.anthropic?.five_hour_align_minutes).toBe(60);
  });

  it("空字符串（v-model.number 空输入）清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { five_hour: "" as unknown as number, daily: "" as unknown as number, weekly: null, monthly: null, five_hour_align_minutes: "" as unknown as number },
    });
    expect(result.anthropic?.five_hour).toBe(null);
    expect(result.anthropic?.daily).toBe(null);
    expect(result.anthropic?.five_hour_align_minutes).toBe(0);
  });

  it("负数清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      openai: { five_hour: -1, daily: -1, weekly: null, monthly: null, five_hour_align_minutes: -1 },
    });
    expect(result.openai?.five_hour).toBe(null);
    expect(result.openai?.daily).toBe(null);
    expect(result.openai?.five_hour_align_minutes).toBe(0);
  });

  it("NaN/Infinity 清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      gemini: { five_hour: NaN, daily: NaN, weekly: Infinity, monthly: null, five_hour_align_minutes: Infinity },
    });
    expect(result.gemini?.five_hour).toBe(null);
    expect(result.gemini?.daily).toBe(null);
    expect(result.gemini?.weekly).toBe(null);
    expect(result.gemini?.five_hour_align_minutes).toBe(0);
  });

  it("缺失平台填充为全 null", () => {
    const result = sanitizePlatformQuotasMap({});
    expect(Object.keys(result)).toHaveLength(4);
    for (const v of Object.values(result)) {
      expect(v).toEqual(nullQuota);
    }
  });
});
