import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import UpstreamBillingRateCell from '../UpstreamBillingRateCell.vue'
import type { Account REDACTED from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${keyREDACTED:${Object.values(params).join(',')REDACTED` : key
    REDACTED)
  REDACTED
REDACTED)

const makeAccount = (overrides: Partial<Account> = {REDACTED): Account => ({
  id: 1,
  name: 'upstream',
  platform: 'openai',
  type: 'apikey',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-13T00:00:00Z',
  updated_at: '2026-07-13T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
REDACTED)

const billingData = {
  object: 'sub2api.key_billing' as const,
  schema_version: 1 as const,
  billing_scope: 'token' as const,
  group_rate_multiplier: 0.8,
  resolved_rate_multiplier: 0.6,
  peak_rate_enabled: true,
  peak_start: '09:00',
  peak_end: '18:00',
  peak_rate_multiplier: 1.5,
  applied_peak_multiplier: 1.5,
  effective_rate_multiplier: 0.9,
  timezone: 'Asia/Shanghai',
  observed_at: '2026-07-13T00:00:00Z'
REDACTED

describe('UpstreamBillingRateCell', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-13T00:30:00Z'))
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  it('recomputes the current effective rate and keeps the icon-only probe action', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: makeAccount({
          extra: {
            upstream_billing_probe_enabled: true,
            upstream_billing_probe: {
              status: 'ok',
              data: billingData,
              received_at: '2026-07-13T00:00:00Z',
              fresh_until: '2026-07-14T00:00:00Z',
              last_attempt_at: '2026-07-13T00:00:00Z',
              next_probe_at: '2026-07-13T00:30:00Z'
            REDACTED
          REDACTED
        REDACTED),
        intervalMinutes: 30,
        now: Date.now()
      REDACTED
    REDACTED)

    expect(wrapper.text()).toContain('0.6x')
    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00Z') REDACTED)
    expect(wrapper.text()).toContain('0.9x')
    await wrapper.setProps({ now: Date.parse('2026-07-13T10:00:00Z') REDACTED)
    expect(wrapper.text()).toContain('0.6x')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.latest')
    expect(wrapper.get('[data-testid="upstream-billing-probe"]').text()).toBe('')
    expect(wrapper.get('[data-testid="upstream-billing-probe"]').attributes('aria-label')).toBe(
      'admin.accounts.upstreamBilling.manualProbe'
    )
  REDACTED)

  it('uses retained failed data only while it is still fresh', async () => {
    const account = makeAccount({
      extra: {
        upstream_billing_probe: {
          status: 'ok',
          data: billingData,
          received_at: '2026-07-12T22:00:00Z',
          fresh_until: '2026-07-12T23:00:00Z',
          last_attempt_at: '2026-07-12T22:00:00Z',
          next_probe_at: '2026-07-12T22:30:00Z'
        REDACTED
      REDACTED
    REDACTED)
    const wrapper = mount(UpstreamBillingRateCell, { props: { account, intervalMinutes: 30, now: Date.now() REDACTED REDACTED)
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            data: billingData,
            received_at: '2026-07-13T00:00:00Z',
            fresh_until: '2026-07-13T01:00:00Z',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'http_error'
          REDACTED
        REDACTED
      REDACTED)
    REDACTED)
    expect(wrapper.text()).toContain('0.6x')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.failed')

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00Z') REDACTED)
    expect(wrapper.text()).toContain('0.9x')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')

    await wrapper.setProps({ now: Date.parse('2026-07-13T01:00:00.001Z') REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')

    await wrapper.setProps({
      now: Date.now(),
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            data: billingData,
            received_at: '2026-07-12T22:00:00Z',
            fresh_until: '2026-07-12T23:00:00Z',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'http_error'
          REDACTED
        REDACTED
      REDACTED)
    REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.stale')
  REDACTED)

  it('emits manual probe commands only for eligible accounts', async () => {
    const wrapper = mount(UpstreamBillingRateCell, {
      props: { account: makeAccount(), intervalMinutes: 30, now: Date.now() REDACTED
    REDACTED)
    await wrapper.get('[data-testid="upstream-billing-probe"]').trigger('click')
    expect(wrapper.emitted('probe')).toHaveLength(1)

    await wrapper.setProps({ account: makeAccount({ type: 'oauth' REDACTED) REDACTED)
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).toBe('-')
  REDACTED)

  it('fails neutral for malformed data and timestamps', async () => {
    const malformedAccount = (
      dataOverrides: Partial<typeof billingData> = {REDACTED,
      snapshotOverrides: Record<string, unknown> = {REDACTED
    ) => makeAccount({
      extra: {
        upstream_billing_probe: {
          status: 'ok',
          data: { ...billingData, ...dataOverrides REDACTED,
          received_at: '2026-07-13T00:00:00Z',
          fresh_until: '2026-07-13T01:00:00Z',
          last_attempt_at: '2026-07-13T00:00:00Z',
          next_probe_at: '2026-07-13T01:00:00Z',
          ...snapshotOverrides
        REDACTED
      REDACTED
    REDACTED)
    const wrapper = mount(UpstreamBillingRateCell, {
      props: {
        account: malformedAccount({
          resolved_rate_multiplier: -1,
          peak_rate_enabled: false,
          effective_rate_multiplier: -1
        REDACTED),
        intervalMinutes: 30,
        now: Date.now()
      REDACTED
    REDACTED)

    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({ billing_scope: 'request' as 'token' REDACTED) REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({REDACTED, { received_at: 'not-a-time' REDACTED) REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({REDACTED, { received_at: '2026-07-13T00:31:00Z' REDACTED) REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    await wrapper.setProps({ account: malformedAccount({REDACTED, { fresh_until: '2026-07-12T23:59:00Z' REDACTED) REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          upstream_billing_probe: {
            status: 'failed',
            last_attempt_at: '2026-07-13T00:00:00Z',
            next_probe_at: '2026-07-13T01:00:00Z',
            last_error: 'network_error'
          REDACTED
        REDACTED
      REDACTED)
    REDACTED)
    expect(wrapper.get('[data-testid="upstream-billing-rate"]').text()).toBe('-')
    expect(wrapper.text()).toContain('admin.accounts.upstreamBilling.failed')
    expect(wrapper.text()).not.toContain('admin.accounts.upstreamBilling.stale')
  REDACTED)
REDACTED)
