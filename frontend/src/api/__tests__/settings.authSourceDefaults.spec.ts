import { describe, expect, it REDACTED from "vitest";

import {
  appendAuthSourceDefaultsToUpdateRequest,
  buildAuthSourceDefaultsState,
  normalizePlatformQuotasMap,
  sanitizePlatformQuotasMap,
  type UpdateSettingsRequest,
  type DefaultPlatformQuotasMap,
REDACTED from "@/api/admin/settings";

/** 全 null 的 5 平台 map，用于断言归一化默认值 */
const allNullQuotas: DefaultPlatformQuotasMap = {
  anthropic: { daily: null, weekly: null, monthly: null REDACTED,
  openai:    { daily: null, weekly: null, monthly: null REDACTED,
  gemini:    { daily: null, weekly: null, monthly: null REDACTED,
  antigravity: { daily: null, weekly: null, monthly: null REDACTED,
  grok: { daily: null, weekly: null, monthly: null REDACTED,
REDACTED

describe("admin settings auth source defaults helpers", () => {
  it("builds auth source defaults state from flat settings fields", () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_balance: 9.5,
      auth_source_default_email_concurrency: 3,
      auth_source_default_email_subscriptions: [
        { group_id: 1, validity_days: 30 REDACTED,
      ],
      auth_source_default_email_grant_on_signup: false,
      auth_source_default_email_grant_on_first_bind: true,
      auth_source_default_linuxdo_balance: 6,
      auth_source_default_linuxdo_concurrency: 8,
      auth_source_default_linuxdo_subscriptions: [
        { group_id: 2, validity_days: 60 REDACTED,
      ],
      auth_source_default_linuxdo_grant_on_signup: true,
      auth_source_default_linuxdo_grant_on_first_bind: false,
    REDACTED);

    expect(state.email).toEqual({
      balance: 9.5,
      concurrency: 3,
      subscriptions: [{ group_id: 1, validity_days: 30 REDACTED],
      grant_on_signup: false,
      grant_on_first_bind: true,
      platform_quotas: allNullQuotas,
    REDACTED);
    expect(state.linuxdo).toEqual({
      balance: 6,
      concurrency: 8,
      subscriptions: [{ group_id: 2, validity_days: 60 REDACTED],
      grant_on_signup: true,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    REDACTED);
    expect(state.oidc).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    REDACTED);
    expect(state.wechat).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    REDACTED);
  REDACTED);

  it("defaults grant-on-signup to disabled when settings are missing", () => {
    const state = buildAuthSourceDefaultsState({REDACTED);

    expect(state.email.grant_on_signup).toBe(false);
    expect(state.linuxdo.grant_on_signup).toBe(false);
    expect(state.oidc.grant_on_signup).toBe(false);
    expect(state.wechat.grant_on_signup).toBe(false);
  REDACTED);

  it("reads nested platform_quotas from settings into auth source state", () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_platform_quotas: {
        anthropic: { daily: 10, weekly: 50, monthly: 200 REDACTED,
        openai:    { daily: null, weekly: null, monthly: null REDACTED,
      REDACTED as DefaultPlatformQuotasMap,
    REDACTED);

    // anthropic 填写的值应被保留
    expect(state.email.platform_quotas.anthropic).toEqual({ daily: 10, weekly: 50, monthly: 200 REDACTED);
    // openai 全 null 应被保留
    expect(state.email.platform_quotas.openai).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    // 未出现的平台（gemini/antigravity）归一化为 null
    expect(state.email.platform_quotas.gemini).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    expect(state.email.platform_quotas.antigravity).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
  REDACTED);

  it("appends auth source defaults back onto update payload", () => {
    const payload: UpdateSettingsRequest = {
      site_name: "Sub2API",
    REDACTED;

    appendAuthSourceDefaultsToUpdateRequest(payload, {
      email: {
        balance: 1.25,
        concurrency: 2,
        subscriptions: [{ group_id: 3, validity_days: 7 REDACTED],
        grant_on_signup: true,
        grant_on_first_bind: false,
        platform_quotas: {REDACTED,
      REDACTED,
      linuxdo: {
        balance: 0,
        concurrency: 6,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: true,
        platform_quotas: {REDACTED,
      REDACTED,
      oidc: {
        balance: 4,
        concurrency: 9,
        subscriptions: [{ group_id: 9, validity_days: 90 REDACTED],
        grant_on_signup: true,
        grant_on_first_bind: true,
        platform_quotas: {REDACTED,
      REDACTED,
      wechat: {
        balance: 2,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {REDACTED,
      REDACTED,
      github: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {REDACTED,
      REDACTED,
      google: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {REDACTED,
      REDACTED,
      dingtalk: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {REDACTED,
      REDACTED,
    REDACTED);

    expect(payload).toMatchObject({
      site_name: "Sub2API",
      auth_source_default_email_balance: 1.25,
      auth_source_default_email_concurrency: 2,
      auth_source_default_email_subscriptions: [
        { group_id: 3, validity_days: 7 REDACTED,
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
        { group_id: 9, validity_days: 90 REDACTED,
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
    REDACTED);
  REDACTED);

  it("appends sanitized nested platform_quotas with non-null values in update payload", () => {
    const payload: UpdateSettingsRequest = {REDACTED;
    appendAuthSourceDefaultsToUpdateRequest(payload, {
      email: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
        platform_quotas: {
          anthropic: { daily: 10, weekly: 50, monthly: 200 REDACTED,
          openai:    { daily: 0, weekly: null, monthly: null REDACTED,
        REDACTED,
      REDACTED,
      linuxdo: { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
      oidc:    { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
      wechat:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
      github:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
      google:  { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
      dingtalk: { balance: 0, concurrency: 5, subscriptions: [], grant_on_signup: false, grant_on_first_bind: false, platform_quotas: {REDACTED REDACTED,
    REDACTED);

    const emailQuotas = (payload as Record<string, unknown>)["auth_source_default_email_platform_quotas"] as DefaultPlatformQuotasMap;
    expect(emailQuotas.anthropic).toEqual({ daily: 10, weekly: 50, monthly: 200 REDACTED);
    // 0 是合法值（不限额=0 与"不设"不同，保留）
    expect(emailQuotas.openai?.daily).toBe(0);
    // 缺失平台归一化为全 null
    expect(emailQuotas.gemini).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    expect(emailQuotas.antigravity).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
  REDACTED);
REDACTED);

describe("normalizePlatformQuotasMap", () => {
  it("填充缺失的平台为全 null 三档", () => {
    const result = normalizePlatformQuotasMap({ anthropic: { daily: 5, weekly: null, monthly: null REDACTED REDACTED);
    expect(result.anthropic).toEqual({ daily: 5, weekly: null, monthly: null REDACTED);
    expect(result.openai).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    expect(result.gemini).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    expect(result.antigravity).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    expect(result.grok).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
  REDACTED);

  it("无参数时返回全 5 平台全 null", () => {
    const result = normalizePlatformQuotasMap();
    expect(Object.keys(result)).toHaveLength(5);
    for (const v of Object.values(result)) {
      expect(v).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    REDACTED
  REDACTED);

  it("非 number 类型的值归一化为 null", () => {
    const result = normalizePlatformQuotasMap({
      anthropic: { daily: "50" as unknown as number, weekly: undefined as unknown as number, monthly: null REDACTED,
    REDACTED);
    expect(result.anthropic).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
  REDACTED);
REDACTED);

describe("sanitizePlatformQuotasMap", () => {
  it("保留合法的正数和零值", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { daily: 10.5, weekly: 0, monthly: null REDACTED,
    REDACTED);
    expect(result.anthropic?.daily).toBe(10.5);
    expect(result.anthropic?.weekly).toBe(0);
    expect(result.anthropic?.monthly).toBe(null);
  REDACTED);

  it("空字符串（v-model.number 空输入）清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { daily: "" as unknown as number, weekly: null, monthly: null REDACTED,
    REDACTED);
    expect(result.anthropic?.daily).toBe(null);
  REDACTED);

  it("负数清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      openai: { daily: -1, weekly: null, monthly: null REDACTED,
    REDACTED);
    expect(result.openai?.daily).toBe(null);
  REDACTED);

  it("NaN/Infinity 清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      gemini: { daily: NaN, weekly: Infinity, monthly: null REDACTED,
    REDACTED);
    expect(result.gemini?.daily).toBe(null);
    expect(result.gemini?.weekly).toBe(null);
  REDACTED);

  it("缺失平台填充为全 null", () => {
    const result = sanitizePlatformQuotasMap({REDACTED);
    expect(Object.keys(result)).toHaveLength(5);
    for (const v of Object.values(result)) {
      expect(v).toEqual({ daily: null, weekly: null, monthly: null REDACTED);
    REDACTED
  REDACTED);
REDACTED);
