import { describe, it, expect, beforeEach, vi REDACTED from 'vitest'
import { defineComponent REDACTED from 'vue'
import { flushPromises, mount REDACTED from '@vue/test-utils'
import OpsOpenAITokenStatsCard from '../OpsOpenAITokenStatsCard.vue'

const mockGetOpenAITokenStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getOpenAITokenStats: (...args: any[]) => mockGetOpenAITokenStats(...args),
  REDACTED,
REDACTED))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (key === 'admin.ops.openaiTokenStats.pageInfo' && params) {
          return `第 ${params.pageREDACTED/${params.totalREDACTED 页`
        REDACTED
        return key
      REDACTED,
    REDACTED),
  REDACTED
REDACTED)

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    REDACTED,
  REDACTED,
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
REDACTED)

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' REDACTED,
    description: { type: String, default: '' REDACTED,
  REDACTED,
  template: '<div class="empty-state">{{ title REDACTEDREDACTED|{{ description REDACTEDREDACTED</div>',
REDACTED)

const sampleResponse = {
  time_range: '30d' as const,
  start_time: '2026-01-01T00:00:00Z',
  end_time: '2026-01-31T00:00:00Z',
  platform: 'openai',
  group_id: 7,
  items: [
    {
      model: 'gpt-4o-mini',
      request_count: 12,
      avg_tokens_per_sec: 22.5,
      avg_first_token_ms: 123.45,
      total_output_tokens: 1234,
      avg_duration_ms: 321,
      requests_with_first_token: 10,
    REDACTED,
  ],
  total: 40,
  page: 1,
  page_size: 20,
  top_n: null,
REDACTED

describe('OpsOpenAITokenStatsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  REDACTED)

  it('默认加载并透传 platform/group 过滤，支持时间窗口切换', async () => {
    mockGetOpenAITokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsOpenAITokenStatsCard, {
      props: {
        platformFilter: 'openai',
        groupIdFilter: 7,
        refreshToken: 0,
      REDACTED,
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()
    expect(mockGetOpenAITokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '30d',
        platform: 'openai',
        group_id: 7,
        top_n: 20,
      REDACTED)
    )

    const selects = wrapper.findAllComponents(SelectStub)
    await selects[0].vm.$emit('update:modelValue', '1h')
    await flushPromises()

    expect(mockGetOpenAITokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '1h',
        platform: 'openai',
        group_id: 7,
      REDACTED)
    )
  REDACTED)

  it('支持分页与 TopN 模式切换并按参数请求', async () => {
    mockGetOpenAITokenStats.mockImplementation(async (params: Record<string, any>) => ({
      ...sampleResponse,
      time_range: params.time_range ?? '30d',
      page: params.page ?? 1,
      page_size: params.page_size ?? 20,
      top_n: params.top_n ?? null,
      total: 40,
    REDACTED))

    const wrapper = mount(OpsOpenAITokenStatsCard, {
      props: {
        refreshToken: 0,
      REDACTED,
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        REDACTED,
      REDACTED,
    REDACTED)
    await flushPromises()

    let selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', 'pagination')
    await flushPromises()

    expect(mockGetOpenAITokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 1,
        page_size: 20,
      REDACTED)
    )

    const buttons = wrapper.findAll('button')
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    await buttons[1].trigger('click')
    await flushPromises()

    expect(mockGetOpenAITokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 2,
        page_size: 20,
      REDACTED)
    )

    selects = wrapper.findAllComponents(SelectStub)
    await selects[1].vm.$emit('update:modelValue', 'topn')
    await flushPromises()
    selects = wrapper.findAllComponents(SelectStub)
    await selects[2].vm.$emit('update:modelValue', 50)
    await flushPromises()

    expect(mockGetOpenAITokenStats).toHaveBeenCalledWith(
      expect.objectContaining({
        top_n: 50,
      REDACTED)
    )
  REDACTED)

  it('接口返回空数据时显示空态', async () => {
    mockGetOpenAITokenStats.mockResolvedValue({
      ...sampleResponse,
      items: [],
      total: 0,
    REDACTED)

    const wrapper = mount(OpsOpenAITokenStatsCard, {
      props: { refreshToken: 0 REDACTED,
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        REDACTED,
      REDACTED,
    REDACTED)
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
  REDACTED)

  it('数据表使用固定高度滚动容器，避免纵向无限增长', async () => {
    mockGetOpenAITokenStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsOpenAITokenStatsCard, {
      props: { refreshToken: 0 REDACTED,
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        REDACTED,
      REDACTED,
    REDACTED)
    await flushPromises()

    expect(wrapper.find('.max-h-\\[420px\\]').exists()).toBe(true)
  REDACTED)

  it('接口异常时显示错误提示', async () => {
    mockGetOpenAITokenStats.mockRejectedValue(new Error('加载失败'))

    const wrapper = mount(OpsOpenAITokenStatsCard, {
      props: { refreshToken: 0 REDACTED,
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        REDACTED,
      REDACTED,
    REDACTED)
    await flushPromises()

    expect(wrapper.text()).toContain('加载失败')
  REDACTED)
REDACTED)
