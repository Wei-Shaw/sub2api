import { describe, expect, it, vi, beforeEach REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account REDACTED from '@/types'

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

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  REDACTED
REDACTED

describe('AccountUsageCell', () => {
  beforeEach(() => {
    getUsage.mockReset()
  REDACTED)

  it('Antigravity 图片用量会聚合新旧 image 模型', async () => {
    getUsage.mockResolvedValue({
      antigravity_quota: {
        'gemini-2.5-flash-image': {
          utilization: 45,
          reset_time: '2026-03-01T11:00:00Z'
        REDACTED,
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
        account: makeAccount({
          id: 1001,
          platform: 'antigravity',
          type: 'oauth',
          extra: {REDACTED
        REDACTED)
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

  it('Antigravity 会显示 AI Credits 余额信息', async () => {
    getUsage.mockResolvedValue({
      ai_credits: [
        {
          credit_type: 'GOOGLE_ONE_AI',
          amount: 25,
          minimum_balance: 5
        REDACTED
      ]
    REDACTED)

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 1002,
          platform: 'antigravity',
          type: 'oauth',
          extra: {REDACTED
        REDACTED)
      REDACTED,
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.aiCreditsBalance')
    expect(wrapper.text()).toContain('25')
  REDACTED)


  it('OpenAI OAuth 快照已过期时首屏会重新请求 usage', async () => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 15,
        resets_at: '2026-03-08T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 3,
          tokens: 300,
          cost: 0.03,
          standard_cost: 0.03,
          user_cost: 0.03
        REDACTED
      REDACTED,
      seven_day: {
        utilization: 77,
        resets_at: '2026-03-13T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 3,
          tokens: 300,
          cost: 0.03,
          standard_cost: 0.03,
          user_cost: 0.03
        REDACTED
      REDACTED
    REDACTED)

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2000,
          platform: 'openai',
          type: 'oauth',
          extra: {
            codex_usage_updated_at: '2026-03-07T00:00:00Z',
            codex_5h_used_percent: 12,
            codex_5h_reset_at: '2026-03-08T12:00:00Z',
            codex_7d_used_percent: 34,
            codex_7d_reset_at: '2026-03-13T12:00:00Z'
          REDACTED
        REDACTED)
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

    expect(getUsage).toHaveBeenCalledWith(2000)
    expect(wrapper.text()).toContain('5h|15|300')
    expect(wrapper.text()).toContain('7d|77|300')
  REDACTED)

  it('OpenAI OAuth 有现成快照且未限额时不会首屏请求 usage', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2001,
          platform: 'openai',
          type: 'oauth',
          extra: {
            codex_usage_updated_at: '2099-03-07T10:00:00Z',
            codex_5h_used_percent: 12,
            codex_5h_reset_at: '2099-03-07T12:00:00Z',
            codex_7d_used_percent: 34,
            codex_7d_reset_at: '2099-03-13T12:00:00Z'
          REDACTED
        REDACTED)
      REDACTED,
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
            template: '<div class="usage-bar">{{ label REDACTEDREDACTED|{{ utilization REDACTEDREDACTED</div>'
          REDACTED,
          AccountQuotaInfo: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(getUsage).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('5h|12')
    expect(wrapper.text()).toContain('7d|34')
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
		    account: makeAccount({
		      id: 2002,
		      platform: 'openai',
		      type: 'oauth',
		      extra: {REDACTED
		    REDACTED)
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

  it('OpenAI OAuth 在行数据刷新但仍无 codex 快照时会重新拉取 usage', async () => {
	getUsage
	  .mockResolvedValueOnce({
	    five_hour: {
	      utilization: 0,
	      resets_at: null,
	      remaining_seconds: 0,
	      window_stats: {
	        requests: 1,
	        tokens: 100,
	        cost: 0.01,
	        standard_cost: 0.01,
	        user_cost: 0.01
	      REDACTED
	    REDACTED,
	    seven_day: null
	  REDACTED)
	  .mockResolvedValueOnce({
	    five_hour: {
	      utilization: 0,
	      resets_at: null,
	      remaining_seconds: 0,
	      window_stats: {
	        requests: 2,
	        tokens: 200,
	        cost: 0.02,
	        standard_cost: 0.02,
	        user_cost: 0.02
	      REDACTED
	    REDACTED,
	    seven_day: null
	  REDACTED)

		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 2003,
		      platform: 'openai',
		      type: 'oauth',
		      updated_at: '2026-03-07T10:00:00Z',
		      extra: {REDACTED
		    REDACTED)
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
	expect(wrapper.text()).toContain('5h|0|100')
	expect(getUsage).toHaveBeenCalledTimes(1)

	await wrapper.setProps({
	  account: {
	    id: 2003,
	    platform: 'openai',
	    type: 'oauth',
	    updated_at: '2026-03-07T10:01:00Z',
	    extra: {REDACTED
	  REDACTED
REDACTED)

	await flushPromises()
	expect(getUsage).toHaveBeenCalledTimes(2)
	expect(wrapper.text()).toContain('5h|0|200')
  REDACTED)

  it('OpenAI OAuth 已限额时首屏优先展示重新查询后的 usage，而不是旧 codex 快照', async () => {
	getUsage.mockResolvedValue({
	  five_hour: {
	    utilization: 100,
	    resets_at: '2026-03-07T12:00:00Z',
	    remaining_seconds: 3600,
	    window_stats: {
	      requests: 211,
	      tokens: 106540000,
	      cost: 38.13,
	      standard_cost: 38.13,
	      user_cost: 38.13
	    REDACTED
	  REDACTED,
	  seven_day: {
	    utilization: 100,
	    resets_at: '2026-03-13T12:00:00Z',
	    remaining_seconds: 3600,
	    window_stats: {
	      requests: 211,
	      tokens: 106540000,
	      cost: 38.13,
	      standard_cost: 38.13,
	      user_cost: 38.13
	    REDACTED
	  REDACTED
REDACTED)

		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 2004,
		      platform: 'openai',
		      type: 'oauth',
		      rate_limit_reset_at: '2099-03-07T12:00:00Z',
		      extra: {
		        codex_5h_used_percent: 0,
		        codex_7d_used_percent: 0
		      REDACTED
		    REDACTED)
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

	expect(getUsage).toHaveBeenCalledWith(2004)
	expect(wrapper.text()).toContain('5h|100|106540000')
	expect(wrapper.text()).toContain('7d|100|106540000')
	expect(wrapper.text()).not.toContain('5h|0|')
  REDACTED)
REDACTED)
