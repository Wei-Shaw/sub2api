import { mount REDACTED from '@vue/test-utils'
import { describe, expect, it, vi REDACTED from 'vitest'
import { defineComponent REDACTED from 'vue'
import OpsErrorDistributionChart from '../OpsErrorDistributionChart.vue'
import OpsErrorTrendChart from '../OpsErrorTrendChart.vue'

vi.mock('chart.js', () => ({
  Chart: { register: vi.fn() REDACTED,
  ArcElement: {REDACTED,
  CategoryScale: {REDACTED,
  Filler: {REDACTED,
  Legend: {REDACTED,
  LineElement: {REDACTED,
  LinearScale: {REDACTED,
  PointElement: {REDACTED,
  Title: {REDACTED,
  Tooltip: {REDACTED,
REDACTED))

vi.mock('vue-chartjs', async () => {
  const { defineComponent REDACTED = await import('vue')

  return {
    Doughnut: defineComponent({
      name: 'Doughnut',
      props: {
        data: { type: Object, required: true REDACTED,
        options: { type: Object, default: () => ({REDACTED) REDACTED,
      REDACTED,
      template: '<div class="doughnut-stub" />',
    REDACTED),
    Line: defineComponent({
      name: 'LineChartStub',
      props: {
        data: { type: Object, required: true REDACTED,
        options: { type: Object, default: () => ({REDACTED) REDACTED,
      REDACTED,
      template: '<div class="line-stub" />',
    REDACTED),
  REDACTED
REDACTED)

vi.mock('../../utils/opsFormatters', () => ({
  formatHistoryLabel: (date: string | undefined) => date ?? '',
  sumNumbers: (values: Array<number | null | undefined>) =>
    values.reduce<number>((total, value) => total + (typeof value === 'number' && Number.isFinite(value) ? value : 0), 0),
REDACTED))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    REDACTED),
  REDACTED
REDACTED)

const HelpTooltipStub = defineComponent({
  name: 'HelpTooltip',
  props: {
    content: { type: String, default: '' REDACTED,
  REDACTED,
  template: '<span class="help-tooltip-stub" />',
REDACTED)

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' REDACTED,
    description: { type: String, default: '' REDACTED,
  REDACTED,
  template: '<div class="empty-state-stub" />',
REDACTED)

const globalStubs = {
  stubs: {
    HelpTooltip: HelpTooltipStub,
    EmptyState: EmptyStateStub,
  REDACTED,
REDACTED

describe('Ops SLA-scoped error charts', () => {
  it('错误分布图按 SLA 错误数统计，不把业务限制错误算进请求错误分布', () => {
    const wrapper = mount(OpsErrorDistributionChart, {
      props: {
        loading: false,
        data: {
          total: 10,
          items: [
            { status_code: 400, total: 7, sla: 2, business_limited: 5 REDACTED,
            { status_code: 503, total: 3, sla: 0, business_limited: 3 REDACTED,
          ],
        REDACTED,
      REDACTED,
      global: globalStubs,
    REDACTED)

    const doughnut = wrapper.findComponent({ name: 'Doughnut' REDACTED)
    expect(doughnut.exists()).toBe(true)
    expect(doughnut.props('data')).toMatchObject({
      labels: ['admin.ops.client'],
      datasets: [{ data: [2] REDACTED],
    REDACTED)
  REDACTED)

  it('错误分布图在只有业务限制错误时显示为空态', () => {
    const wrapper = mount(OpsErrorDistributionChart, {
      props: {
        loading: false,
        data: {
          total: 4,
          items: [{ status_code: 500, total: 4, sla: 0, business_limited: 4 REDACTED],
        REDACTED,
      REDACTED,
      global: globalStubs,
    REDACTED)

    expect(wrapper.findComponent({ name: 'Doughnut' REDACTED).exists()).toBe(false)
    expect(wrapper.find('.empty-state-stub').exists()).toBe(true)
  REDACTED)

  it('错误趋势图的请求错误详情按钮只按 SLA 错误启用', () => {
    const wrapper = mount(OpsErrorTrendChart, {
      props: {
        loading: false,
        timeRange: '1h',
        points: [
          {
            bucket_start: '2026-05-18T00:00:00Z',
            error_count_total: 5,
            business_limited_count: 5,
            error_count_sla: 0,
            upstream_error_count_excl_429_529: 0,
            upstream_429_count: 0,
            upstream_529_count: 0,
          REDACTED,
        ],
      REDACTED,
      global: globalStubs,
    REDACTED)

    const requestErrorsButton = wrapper.findAll('button')[0]
    expect(requestErrorsButton.attributes('disabled')).toBeDefined()
  REDACTED)
REDACTED)
