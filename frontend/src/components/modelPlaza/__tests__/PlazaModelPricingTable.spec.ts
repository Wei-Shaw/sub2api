import { describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { PlazaModel REDACTED from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

function tokenModel(overrides: Partial<PlazaModel> = {REDACTED): PlazaModel {
  return {
    name: 'claude-sonnet',
    platform: 'anthropic',
    pricing: {
      billing_mode: 'token',
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_read_price: 3e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    REDACTED,
    official_pricing: {
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_write_1h_price: 6e-6,
      cache_read_price: 3e-7
    REDACTED,
    ...overrides
  REDACTED
REDACTED

function mountTable(
  models: PlazaModel[],
  rateMultiplier: number,
  userRateMultiplier?: number | null,
  extraProps?: { imageRateIndependent?: boolean; imageRateMultiplier?: number | null REDACTED
) {
  return mount(PlazaModelPricingTable, {
    props: { models, rateMultiplier, userRateMultiplier: userRateMultiplier ?? null, ...extraProps REDACTED
  REDACTED)
REDACTED

describe('PlazaModelPricingTable', () => {
  it('倍率为 1 时展示渠道单价原值($/1M),价格保底 2 位小数', () => {
    const wrapper = mountTable([tokenModel()], 1)
    const text = wrapper.text()
    expect(text).toContain('$3.00')
    expect(text).toContain('$15.00')
    // 缓存写 / 读(超过 2 位小数原样保留)
    expect(text).toContain('$3.75')
    expect(text).toContain('$0.30')
    // 倍率列
    expect(text).toContain('1x')
  REDACTED)

  it('倍率 ≠ 1 时价格列为折后实付价,官方价列保持原价', () => {
    const wrapper = mountTable([tokenModel()], 0.5)
    const text = wrapper.text()
    // 实付 = 3 × 0.5 / 15 × 0.5
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    // 官方价原值仍在(官方列不乘倍率)
    expect(text).toContain('$3.00')
    expect(text).toContain('$15.00')
    expect(text).toContain('0.5x')
  REDACTED)

  it('用户专属倍率覆盖分组倍率,并划线展示原倍率', () => {
    const wrapper = mountTable([tokenModel()], 1, 0.8)
    const text = wrapper.text()
    // 实付按 0.8:3 × 0.8 = 2.4
    expect(text).toContain('$2.40')
    expect(text).toContain('$12.00')
    // 倍率列:原倍率划线 + 专属倍率
    const struck = wrapper.find('td .line-through')
    expect(struck.exists()).toBe(true)
    expect(struck.text()).toBe('1x')
    expect(text).toContain('0.8x')
  REDACTED)

  it('模型按官方输出价从高到低排序,无官方价的排最后', () => {
    const expensive = tokenModel({
      name: 'model-expensive',
      official_pricing: {
        input_price: 1e-5,
        output_price: 7.5e-5,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      REDACTED
    REDACTED)
    const cheap = tokenModel({
      name: 'model-cheap',
      official_pricing: {
        input_price: 1e-6,
        output_price: 5e-6,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      REDACTED
    REDACTED)
    const noOfficial = tokenModel({ name: 'model-no-official', official_pricing: null REDACTED)

    const wrapper = mountTable([cheap, noOfficial, expensive], 1)
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names).toEqual(['model-expensive', 'model-cheap', 'model-no-official'])
  REDACTED)

  it('官方输出价相同时按模型名降序(新版本号在前)', () => {
    const older = tokenModel({ name: 'gpt-5.5' REDACTED)
    const newer = tokenModel({ name: 'gpt-5.6-sol' REDACTED)

    const wrapper = mountTable([older, newer], 1)
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names).toEqual(['gpt-5.6-sol', 'gpt-5.5'])
  REDACTED)

  it('按图片/按次计费的模型沉到末尾,不与 token 模型按官方价混排', () => {
    // 官方输出价 $10,介于下面两个 token 模型之间,但因计费模式不同应排最后
    const image = tokenModel({
      name: 'gpt-image-2',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.002,
        intervals: []
      REDACTED,
      official_pricing: {
        input_price: 5e-6,
        output_price: 1e-5,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: 1.25e-6
      REDACTED
    REDACTED)
    const pricier = tokenModel({
      name: 'gpt-5.6-terra',
      official_pricing: {
        input_price: 2.5e-6,
        output_price: 1.5e-5,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      REDACTED
    REDACTED)
    const cheaper = tokenModel({
      name: 'gpt-5.6-luna',
      official_pricing: {
        input_price: 1e-6,
        output_price: 6e-6,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null
      REDACTED
    REDACTED)

    const wrapper = mountTable([pricier, image, cheaper], 1)
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names[0]).toBe('gpt-5.6-terra')
    expect(names[1]).toBe('gpt-5.6-luna')
    // 首列含「按图片计费」徽章文本,只断言模型名
    expect(names[2]).toContain('gpt-image-2')
  REDACTED)

  it('两级表头:实付区与官方区各拆输入/输出/缓存列', () => {
    const wrapper = mountTable([tokenModel()], 1)
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.paidPrice')
    expect(text).toContain('modelPlaza.table.officialPrice')
    // token 行:模型 + 实付 3 列 + 官方 3 列 + 倍率
    expect(wrapper.findAll('tbody td')).toHaveLength(8)
  REDACTED)

  it('官方价包含 1h 缓存写入价;official_pricing 为 null 时官方三列显示 -', () => {
    const withOfficial = mountTable([tokenModel()], 1)
    expect(withOfficial.text()).toContain('$6.00')
    expect(withOfficial.text()).toContain('(1h')

    const withoutOfficial = mountTable([tokenModel({ official_pricing: null REDACTED)], 1)
    const cells = withoutOfficial.findAll('tbody td')
    // 官方 输入/输出/缓存 三列均为 -
    expect(cells[4].text().trim()).toBe('-')
    expect(cells[5].text().trim()).toBe('-')
    expect(cells[6].text().trim()).toBe('-')
  REDACTED)

  it('per_request 模型按单次价 × 倍率展示,官方价列显示 -', () => {
    const model = tokenModel({
      name: 'search-tool',
      pricing: {
        billing_mode: 'per_request',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.04,
        intervals: []
      REDACTED,
      official_pricing: null
    REDACTED)
    const wrapper = mountTable([model], 0.5)
    const text = wrapper.text()
    // 0.04 × 0.5 = 0.02,scale=1
    expect(text).toContain('$0.02')
    expect(text).toContain('modelPlaza.table.perRequest')
    // 单位后缀跟在价格后(按次 → / 次)
    expect(text).toContain('modelPlaza.table.perUnitRequest')
  REDACTED)

  it('token 模型阶梯定价内联进输入/输出列,按倍率折算', () => {
    const model = tokenModel({
      pricing: {
        billing_mode: 'token',
        input_price: 3e-6,
        output_price: 1.5e-5,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: 200000,
            tier_label: '',
            input_price: 3e-6,
            output_price: 1.5e-5,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: null
          REDACTED,
          {
            min_tokens: 200000,
            max_tokens: null,
            tier_label: '',
            input_price: 6e-6,
            output_price: 3e-5,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: null
          REDACTED
        ]
      REDACTED
    REDACTED)
    const wrapper = mountTable([model], 0.5)
    const text = wrapper.text()
    // 区间标签按 token 数生成
    expect(text).toContain('≤200K')
    expect(text).toContain('>200K')
    // 折后:输入 1.5 / 3,输出 7.5 / 15
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    expect(text).toContain('$15.00')
  REDACTED)

  it('生图独立倍率开启时,按图价格 × 独立倍率,不乘分组倍率;倍率列展示独立倍率', () => {
    const model = tokenModel({
      name: 'gpt-image-2',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: '1K',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.02
          REDACTED
        ]
      REDACTED,
      official_pricing: null
    REDACTED)
    const wrapper = mountTable([model], 0.1, null, {
      imageRateIndependent: true,
      imageRateMultiplier: 1
    REDACTED)
    const text = wrapper.text()
    // 0.02 × 1(独立倍率),而非 0.02 × 0.1
    expect(text).toContain('$0.02')
    expect(text).not.toContain('$0.002')
    // 倍率列展示独立倍率 1x,而非分组倍率 0.1x
    const rateCell = wrapper.findAll('tbody tr td').at(-1)!
    expect(rateCell.text()).toBe('1x')
  REDACTED)

  it('生图独立倍率关闭时,按图价格仍乘分组/专属生效倍率', () => {
    const model = tokenModel({
      name: 'gpt-image-2',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.2,
        intervals: []
      REDACTED,
      official_pricing: null
    REDACTED)
    const wrapper = mountTable([model], 0.1, null, { imageRateIndependent: false REDACTED)
    const text = wrapper.text()
    expect(text).toContain('$0.02')
    const rateCell = wrapper.findAll('tbody tr td').at(-1)!
    expect(rateCell.text()).toBe('0.1x')
  REDACTED)

  it('按图模型主行展示阶梯芯片,不把 image_output_price(每 token)当按次价', () => {
    const model = tokenModel({
      name: 'gpt-image-2',
      pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        // 每 token 图片输出价:不应被当作按次单价展示
        image_output_price: 3e-5,
        per_request_price: null,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: '1K',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.01
          REDACTED,
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: '2K',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.02
          REDACTED
        ]
      REDACTED,
      official_pricing: null
    REDACTED)
    const wrapper = mountTable([model], 0.1)
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.perImage')
    // 芯片:1K $0.001 / 2K $0.002,单位后缀内嵌(按图 → / 张)
    expect(text).toContain('1K')
    expect(text).toContain('$0.001')
    expect(text).toContain('2K')
    expect(text).toContain('$0.002')
    expect(text).toContain('modelPlaza.table.perUnitImage')
    // 旧 bug:image_output_price × 0.1 = 0.000003 被当按次价
    expect(text).not.toContain('$0.000003')
  REDACTED)

  it('Composite 分组中相同模型名按具体平台分别展示徽章', () => {
    const anthropic = tokenModel({ name: 'shared-model', platform: 'anthropic' REDACTED)
    const openai = tokenModel({ name: 'shared-model', platform: 'openai' REDACTED)
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [anthropic, openai],
        platform: 'composite',
        rateMultiplier: 1
      REDACTED
    REDACTED)

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows.map((row) => row.find('td').text())).toEqual([
      'shared-modelAnthropic',
      'shared-modelOpenAI'
    ])
    expect(wrapper.text()).toContain('Anthropic')
    expect(wrapper.text()).toContain('OpenAI')
  REDACTED)
REDACTED)

describe('PlazaModelPricingTable 长上下文阶梯', () => {
  function ladderIntervals() {
    return [
      {
        min_tokens: 0,
        max_tokens: 272000,
        tier_label: '≤272K',
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_read_price: 5e-7,
        per_request_price: null
      REDACTED,
      {
        min_tokens: 272000,
        max_tokens: null,
        tier_label: '>272K',
        input_price: 1e-5,
        output_price: 4.5e-5,
        cache_write_price: 1.25e-5,
        cache_read_price: 1e-6,
        per_request_price: null
      REDACTED
    ]
  REDACTED

  function ladderModel(overrides: Partial<PlazaModel> = {REDACTED): PlazaModel {
    return tokenModel({
      name: 'gpt-5.6-sol',
      platform: 'openai',
      pricing: {
        billing_mode: 'token',
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_read_price: 5e-7,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: ladderIntervals()
      REDACTED,
      official_pricing: {
        input_price: 5e-6,
        output_price: 3e-5,
        cache_write_price: 6.25e-6,
        cache_read_price: 5e-7,
        intervals: ladderIntervals()
      REDACTED,
      long_context_basis: 'whole_request',
      ...overrides
    REDACTED)
  REDACTED

  it('实付缓存列按档分行并乘倍率,每档一行与输入/输出列对齐;档位标签只在输入列', () => {
    const wrapper = mountTable([ladderModel()], 0.5)
    const cells = wrapper.findAll('tbody td')
    const cacheCell = cells[3]
    const rows = cacheCell.findAll('.leading-5')
    expect(rows).toHaveLength(2)
    // 写 6.25 × 0.5 / 读 0.5 × 0.5;高档 12.5 × 0.5 / 1 × 0.5
    expect(rows[0].text()).toContain('modelPlaza.table.cacheWriteShort')
    expect(rows[0].text()).toContain('$3.125')
    expect(rows[0].text()).toContain('$0.25')
    expect(rows[1].text()).toContain('$6.25')
    expect(rows[1].text()).toContain('$0.50')
    // 输入列带标签,输出/缓存列只按行对齐不重复标签
    expect(cells[1].text()).toContain('≤272K')
    expect(cells[1].text()).toContain('>272K')
    expect(cells[2].text()).not.toContain('272K')
    expect(cacheCell.text()).not.toContain('272K')
    expect(cells[1].findAll('.leading-5')).toHaveLength(2)
    expect(cells[2].findAll('.leading-5')).toHaveLength(2)
  REDACTED)

  it('官方三列按 official_pricing.intervals 分档且不乘倍率,不内联 1h', () => {
    const wrapper = mountTable([ladderModel()], 0.5)
    const cells = wrapper.findAll('tbody td')
    expect(cells[4].text()).toContain('≤272K')
    expect(cells[4].text()).toContain('$5.00')
    expect(cells[4].text()).toContain('>272K')
    expect(cells[4].text()).toContain('$10.00')
    expect(cells[5].text()).toContain('$30.00')
    expect(cells[5].text()).toContain('$45.00')
    expect(cells[6].text()).toContain('$6.25')
    expect(cells[6].text()).toContain('$12.50')
    expect(cells[6].text()).toContain('$1.00')
    expect(cells[6].text()).not.toContain('(1h')
  REDACTED)

  it('整单计价的档位标签带 tooltip;边际计价在模型名旁加徽章并换用边际说明', () => {
    const whole = mountTable([ladderModel()], 1)
    const wholeLabels = whole.findAll('tbody td span[title="modelPlaza.table.tierHint"]')
    expect(wholeLabels.length).toBeGreaterThan(0)
    expect(whole.text()).not.toContain('modelPlaza.table.marginalBadge')

    const marginal = mountTable([ladderModel({ long_context_basis: 'marginal' REDACTED)], 1)
    const marginalLabels = marginal.findAll('tbody td span[title="modelPlaza.table.tierHintMarginal"]')
    expect(marginalLabels.length).toBeGreaterThan(0)
    expect(marginal.findAll('tbody td')[0].text()).toContain('modelPlaza.table.marginalBadge')
  REDACTED)

  it('无标签的多档按区间生成统一形态(≤上限 / >下限),并按下限升序展示', () => {
    const model = ladderModel({
      pricing: {
        ...ladderModel().pricing!,
        // 故意乱序:展示必须按上下文从低到高
        intervals: [
          { ...ladderIntervals()[1], min_tokens: 1000000, tier_label: '' REDACTED,
          { ...ladderIntervals()[0], min_tokens: 100000, max_tokens: 200000, tier_label: '' REDACTED,
          { ...ladderIntervals()[0], max_tokens: 100000, tier_label: '' REDACTED,
          { ...ladderIntervals()[1], min_tokens: 200000, max_tokens: 1000000, tier_label: '' REDACTED
        ]
      REDACTED
    REDACTED)
    const rows = mountTable([model], 1).findAll('tbody td')[1].findAll('.leading-5')
    expect(rows.map((r) => r.text().split(/\s+/)[0])).toEqual(['≤100K', '≤200K', '≤1M', '>1M'])
  REDACTED)

  it('官方无 intervals 字段(旧响应)时官方列保持平价,实付无阶梯时缓存列保持两行', () => {
    const wrapper = mountTable([tokenModel()], 1)
    const cells = wrapper.findAll('tbody td')
    expect(cells[3].text()).toContain('modelPlaza.table.cacheWrite')
    expect(cells[3].text()).toContain('modelPlaza.table.cacheRead')
    expect(cells[3].findAll('.leading-5')).toHaveLength(0)
    expect(cells[6].text()).toContain('(1h')
  REDACTED)
REDACTED)
