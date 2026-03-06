import { describe, expect, it, vi, beforeEach REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'

const { getUsage REDACTED = vi.hoisted(() => ({
  getUsage: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getUsage
    REDACTED
  REDACTED
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

describe('AccountUsageCell', () => {
  beforeEach(() => {
    getUsage.mockReset()
  REDACTED)

  it('Antigravity 图片用量会聚合新旧 image 模型', async () => {
    getUsage.mockResolvedValue({
      antigravity_quota: {
        'gemini-3.1-flash-image': {
          utilization: 20,
          reset_time: '2026-03-01T10:00:00Z'
        REDACTED,
        'gemini-3-pro-image': {
          utilization: 70,
          reset_time: '2026-03-01T09:00:00Z'
        REDACTED
      REDACTED
    REDACTED)

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: {
          id: 1001,
          platform: 'antigravity',
          type: 'oauth',
          extra: {REDACTED
        REDACTED as any
      REDACTED,
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label REDACTEDREDACTED|{{ utilization REDACTEDREDACTED|{{ resetsAt REDACTEDREDACTED</div>'
          REDACTED,
          AccountQuotaInfo: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.gemini3Image|70|2026-03-01T09:00:00Z')
  REDACTED)

  it('OpenAI OAuth 在无 codex 快照时会回退显示 usage 接口窗口', async () => {
	getUsage.mockResolvedValue({
	  five_hour: {
	    utilization: 0,
	    resets_at: null,
	    remaining_seconds: 0,
	    window_stats: {
	      requests: 2,
	      tokens: 27700,
	      cost: 0.06,
	      standard_cost: 0.06,
	      user_cost: 0.06
	    REDACTED
	  REDACTED,
	  seven_day: {
	    utilization: 0,
	    resets_at: null,
	    remaining_seconds: 0,
	    window_stats: {
	      requests: 2,
	      tokens: 27700,
	      cost: 0.06,
	      standard_cost: 0.06,
	      user_cost: 0.06
	    REDACTED
	  REDACTED
REDACTED)

	const wrapper = mount(AccountUsageCell, {
	  props: {
	    account: {
	      id: 2002,
	      platform: 'openai',
	      type: 'oauth',
	      extra: {REDACTED
	    REDACTED as any
	  REDACTED,
	  global: {
	    stubs: {
	      UsageProgressBar: {
	        props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
	        template: '<div class="usage-bar">{{ label REDACTEDREDACTED|{{ utilization REDACTEDREDACTED|{{ windowStats?.tokens REDACTEDREDACTED</div>'
	      REDACTED,
	      AccountQuotaInfo: true
	    REDACTED
	  REDACTED
REDACTED)

	await flushPromises()

	expect(getUsage).toHaveBeenCalledWith(2002)
	expect(wrapper.text()).toContain('5h|0|27700')
	expect(wrapper.text()).toContain('7d|0|27700')
  REDACTED)
REDACTED)
