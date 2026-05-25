import { describe, it, expect, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'

// t() 回显 key，便于断言文案键
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key REDACTED),
  REDACTED
REDACTED)

import UserPlatformQuotaCell from '../UserPlatformQuotaCell.vue'
import type { PlatformQuotaItem REDACTED from '@/api/admin/users'

function item(over: Partial<PlatformQuotaItem> & { platform: PlatformQuotaItem['platform'] REDACTED): PlatformQuotaItem {
  return {
    daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
    daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
    ...over,
  REDACTED as PlatformQuotaItem
REDACTED

describe('UserPlatformQuotaCell', () => {
  it('quotas 为 undefined 时渲染加载占位 …', () => {
    const w = mount(UserPlatformQuotaCell, { props: { quotas: undefined REDACTED REDACTED)
    expect(w.text()).toContain('…')
    expect(w.html()).not.toContain('admin.users.platformQuota.cellNotConfigured')
  REDACTED)

  it('空数组渲染「未配置」', () => {
    const w = mount(UserPlatformQuotaCell, { props: { quotas: [] REDACTED REDACTED)
    expect(w.html()).toContain('admin.users.platformQuota.cellNotConfigured')
  REDACTED)

  it('平台有记录但全部 limit 为 null 时视为未配置', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: { quotas: [item({ platform: 'openai', daily_usage_usd: 5 REDACTED)] REDACTED,
    REDACTED)
    expect(w.html()).toContain('admin.users.platformQuota.cellNotConfigured')
  REDACTED)

  it('已配置平台渲染 已用/限额，null 档显示 —，金额去尾零', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: {
        quotas: [
          item({ platform: 'anthropic', daily_limit_usd: 100, daily_usage_usd: 30,
                 weekly_limit_usd: null, weekly_usage_usd: 0,
                 monthly_limit_usd: 2000, monthly_usage_usd: 90.5 REDACTED),
        ],
      REDACTED,
    REDACTED)
    const html = w.html()
    expect(html).toContain('anthropic')
    expect(html).toContain('30/100')
    expect(html).toContain('0/—')
    expect(html).toContain('90.5/2000')
  REDACTED)

  it('多平台按 anthropic→openai→gemini→antigravity 顺序，仅展示有限额的', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: {
        quotas: [
          item({ platform: 'gemini', monthly_limit_usd: 50 REDACTED),
          item({ platform: 'anthropic', daily_limit_usd: 10 REDACTED),
          item({ platform: 'openai', daily_usage_usd: 9 REDACTED),
        ],
      REDACTED,
    REDACTED)
    const text = w.text()
    expect(text.indexOf('anthropic')).toBeLessThan(text.indexOf('gemini'))
    expect(text).not.toContain('openai')
  REDACTED)
REDACTED)
