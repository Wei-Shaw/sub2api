import { describe, expect, it, vi, beforeEach, afterEach REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const { list, getStats, getSnapshotV2, getById REDACTED = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  REDACTED)

  return {
    list: vi.fn(),
    getStats: vi.fn(),
    getSnapshotV2: vi.fn(),
    getById: vi.fn(),
  REDACTED
REDACTED)

const messages: Record<string, string> = {
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
REDACTED

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      getStats,
    REDACTED,
    dashboard: {
      getSnapshotV2,
    REDACTED,
    users: {
      getById,
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: vi.fn(),
  REDACTED,
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  REDACTED),
REDACTED))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    REDACTED),
  REDACTED
REDACTED)

const AppLayoutStub = { template: '<div><slot /></div>' REDACTED
const UsageFiltersStub = { template: '<div><slot name="after-reset" /></div>' REDACTED
const ModelDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="model-chart">
      <span class="metric">{{ metric REDACTEDREDACTED</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
REDACTED
const GroupDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="group-chart">
      <span class="metric">{{ metric REDACTEDREDACTED</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
REDACTED

describe('admin UsageView distribution metric toggles', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    REDACTED)
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    REDACTED)
    getSnapshotV2.mockResolvedValue({
      trend: [],
      models: [],
      groups: [],
    REDACTED)
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  it('keeps model and group metric toggles independent without refetching chart data', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        REDACTED,
      REDACTED,
    REDACTED)

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    const modelChart = wrapper.find('[data-test="model-chart"]')
    const groupChart = wrapper.find('[data-test="group-chart"]')

    expect(modelChart.find('.metric').text()).toBe('tokens')
    expect(groupChart.find('.metric').text()).toBe('tokens')

    await modelChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('tokens')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    await groupChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('actual_cost')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
  REDACTED)
REDACTED)
