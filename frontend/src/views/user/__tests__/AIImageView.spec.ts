import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'

// 只替换网络与存储相关的部分，常量与纯函数保持与真实实现一致。
const aiImageApi = vi.hoisted(() => ({
  AI_IMAGE_DEFAULT_MODEL: 'gpt-image-2',
  AI_IMAGE_DEFAULT_SIZE: '1200x1600',
  AI_IMAGE_MODEL_OPTIONS: ['gpt-image-2', 'gpt-image-2.5-sunburst', 'gpt-image-2.5-flare'],
  // 和真实实现同一条规则：不在白名单里就回到默认模型。
  normalizeAIImageModel: (value: string) =>
    ['gpt-image-2', 'gpt-image-2.5-sunburst', 'gpt-image-2.5-flare'].includes((value || '').trim())
      ? (value || '').trim()
      : 'gpt-image-2',
  AI_IMAGE_SIZE_OPTIONS: [
    { value: '1200x1600', label: '1200x1600' },
    { value: '1024x1536', label: '1024x1536' },
    { value: '1024x1024', label: '1024x1024' },
    { value: '1536x1024', label: '1536x1024' },
  ],
  AIImageError: class AIImageError extends Error {
    code: string
    status: number
    constructor(message: string, code = 'api_error', status = 500) {
      super(message)
      this.code = code
      this.status = status
    }
  },
  validateAIImageKey: vi.fn(),
  validateVisionKey: vi.fn(),
  generateAIImage: vi.fn(),
  isImagePermissionError: vi.fn(() => false),
  isPersistableImageUrl: (value: string) => Boolean(value) && !value.startsWith('data:'),
  toBlob: vi.fn(),
  extractSellingPoints: vi.fn(),
  analyzeHotStyle: vi.fn(),
  toDisplayUrl: (image: { url?: string; b64_json?: string }) =>
    image.b64_json ? `data:image/png;base64,${image.b64_json}` : image.url || '',
}))

vi.mock('@/api/aiImage', () => aiImageApi)

// 2026-11-04：页面顶部条多了「返回首页」，需要 router。测试里只要一个能 push 的壳。
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  // 回显 key，这样断言可以直接检查「显示了哪条文案」。
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}))

import AIImageView from '../AIImageView.vue'
import { PRODUCT_LOCK } from '@/api/aiImagePrompts'

enableAutoUnmount(afterEach)

const RECORDS_KEY = 'sub2api_ai_image_records'
const KEY_STORAGE = 'sub2api_ai_image_key'
const VISION_KEY_STORAGE = 'sub2api_ai_image_vision_key'

function mountView() {
  return mount(AIImageView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: { template: '<i />' },
      },
    },
  })
}

/** 状态筛选下拉是页面上唯一含 value="all" 的 select。 */
function statusSelect(wrapper: VueWrapper) {
  const select = wrapper.findAll('select').find((item) => item.find('option[value="all"]').exists())
  if (!select) throw new Error('status filter select not found')
  return select
}

function seedRecords(records: unknown[]) {
  window.localStorage.setItem(RECORDS_KEY, JSON.stringify(records))
}

beforeEach(() => {
  vi.clearAllMocks()
  window.localStorage.clear()
  aiImageApi.validateAIImageKey.mockResolvedValue({ ok: true, groupName: '生图分组' })
  aiImageApi.validateVisionKey.mockResolvedValue({ ok: true, groupName: 'Gemini 分组' })
  aiImageApi.generateAIImage.mockResolvedValue({ data: [{ url: 'https://cdn.example/out.png' }] })
  aiImageApi.extractSellingPoints.mockResolvedValue({
    product: '316 不锈钢保温杯',
    sp1: '24 часа тепло',
    sp2: 'сталь 316',
    sp3: 'крышка одной рукой',
    pain: '水到下午就凉了',
    diff: '别人用201钢',
  })
})

/**
 * 展开「单张生图」面板。
 *
 * 主人要求功能页面互斥之后，提示词输入框 / 模型 / 尺寸 / 张数 / 锁定
 * 都收进了这个面板里，默认是收起的 —— 所以要碰这些控件就得先点开它。
 */
async function openSinglePanel(wrapper: VueWrapper) {
  if (wrapper.find('[data-testid="single-panel"]').exists()) return wrapper
  await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
  await flushPromises()
  return wrapper
}

/** 存好密钥 + 展开单张生图面板，让页面进入「可以生成」的状态。 */
async function withSavedKey() {
  const wrapper = mountView()
  await flushPromises()
  await wrapper.get('input[type="password"]').setValue('sk-image-key')
  await wrapper.get('input[type="password"]').trigger('keyup.enter')
  await flushPromises()
  await openSinglePanel(wrapper)
  return wrapper
}

/**
 * 存好密钥，但**不**展开任何功能页面。
 *
 * 功能页面互斥之后，各功能的测试要自己决定展开哪一个，
 * 所以需要一个「只把密钥备好」的入口。
 */
async function withKeyOnly() {
  const wrapper = mountView()
  await flushPromises()
  await wrapper.get('input[type="password"]').setValue('sk-image-key')
  await wrapper.get('input[type="password"]').trigger('keyup.enter')
  await flushPromises()
  return wrapper
}

function generateButton(wrapper: VueWrapper) {
  const button = wrapper
    .findAll('button')
    .find((item) => item.text().includes('aiImage.prompt.generate'))
  if (!button) throw new Error('generate button not found')
  return button
}

/** 造一个假的参考图文件。 */
function fakeImageFile(name = 'product.png') {
  return new File([new Uint8Array([1, 2, 3])], name, { type: 'image/png' })
}

/** 通过 input[type=file] 走一遍真实的上传流程。 */
async function attachReference(wrapper: VueWrapper, file = fakeImageFile()) {
  // 参考图选择框在「单张生图」和「套图」两个页面里各有一个，都绑定同一份状态。
  // 功能页面互斥之后，当前展开的是哪个页面不一定，所以按存在的那一个来选。
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

/* ------------------------------------------------------------------ *
 * 爆款复刻（2026-10-19 改成「逐张拆解 → 一图一脚本」）
 * ------------------------------------------------------------------ */

/**
 * 一张参考图的拆解结果（模型回复的 JSON）。
 *
 * 必须带 `blocks`，否则 `parseHotLayout` 会判定「没看懂图」而整张丢弃。
 */
function hotLayoutJson(textRu: string) {
  return JSON.stringify({
    canvas: '3:4',
    background: '纯白',
    palette: ['#C2185B'],
    product_slot: '画面中部，占 55%',
    graphics: '三列圆形图标',
    notes: '',
    blocks: [
      {
        zone: 'top',
        kind: 'headline',
        style: '粗体居中，三行',
        size_pct: 22,
        source_ru: 'ДОРОГИЕ ПОКУПАТЕЛИ',
        text_ru: textRu,
      },
    ],
  })
}

/** 在爆款复刻面板里放一张「我的商品图」（拆解时它会和参考图一起送出去）。 */
async function attachHotProductImage(wrapper: VueWrapper) {
  const input = wrapper.get('[data-testid="hot-product-image"] input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [fakeImageFile('mine.png')], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

/** 把参考图塞进爆款复刻面板（绕开真实文件选择框）。 */
async function pickHotRefs(wrapper: VueWrapper, names = ['hot.png']) {
  const vm = wrapper.vm as unknown as { onHotRefPicked: (e: unknown) => void }
  vm.onHotRefPicked({ target: { files: names.map((name) => fakeImageFile(name)), value: '' } })
  await flushPromises()
}

/** 轮询等条件成立（拆解是串行的，每张之间还有 300ms 间隔）。 */
async function waitUntil(check: () => boolean, timeout = 6000) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    if (check()) return
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
  throw new Error('等待超时')
}

describe('AI生图页面结构', () => {
  it('状态筛选在汇总结果区里，且四个选项顺序为 全部/已完成/生成中/失败', async () => {
    const wrapper = mountView()
    await flushPromises()

    const select = statusSelect(wrapper)
    expect(select.findAll('option').map((option) => option.attributes('value'))).toEqual([
      'all',
      'completed',
      'generating',
      'failed',
    ])

    // 这个筛选筛的是最下面那张**汇总所有功能**的结果表，
    // 所以它必须长在那张表里 —— 以前它长在「单张生图」的卡片里，
    // 而单张生图现在是可以收起的，筛选跟着一起藏起来就不合理了。
    const results = wrapper.get('[data-testid="all-results"]')
    expect(results.element.contains(select.element)).toBe(true)
  })

  it('中间区域在没有记录时给出空状态提示', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.result.empty')
  })

  it('要求先填密钥：未填时点击生成不会发起请求', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openSinglePanel(wrapper)

    await wrapper.get('textarea').setValue('把背景换成厨房')
    const generate = wrapper
      .findAll('button')
      .find((button) => button.text().includes('aiImage.prompt.generate'))!
    await generate.trigger('click')
    await flushPromises()

    expect(aiImageApi.generateAIImage).not.toHaveBeenCalled()
    // 密钥面板应当保持可见，提示用户先填。
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
  })
})

describe('AI生图密钥校验提示', () => {
  it('密钥不是生图模型时给出警告并带上分组名', async () => {
    aiImageApi.validateAIImageKey.mockResolvedValue({
      ok: false,
      reason: 'not-image-model',
      groupName: '纯文本分组',
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('sk-plain-text')
    await wrapper.get('input[type="password"]').trigger('keyup.enter')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.key.notImageModel')
    expect(wrapper.text()).toContain('纯文本分组')
    expect(window.localStorage.getItem(KEY_STORAGE)).toBeNull()
  })

  it('密钥不属于当前账号时给出另一条提示', async () => {
    aiImageApi.validateAIImageKey.mockResolvedValue({ ok: false, reason: 'not-owned' })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('sk-someone-else')
    await wrapper.get('input[type="password"]').trigger('keyup.enter')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.key.invalid')
    expect(wrapper.text()).toContain('aiImage.key.notOwned')
  })

  it('校验通过时记住密钥并显示分组', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('sk-image-key')
    await wrapper.get('input[type="password"]').trigger('keyup.enter')
    await flushPromises()

    expect(window.localStorage.getItem(KEY_STORAGE)).toBe('sk-image-key')
    expect(wrapper.text()).toContain('aiImage.key.valid')
  })

  // 这两条是给「点了没反应」这个真实反馈兜底的：
  // 页头上那个按钮只是展开面板，如果它也叫「保存密钥」，
  // 用户会点它、然后以为保存失败。
  it('页头那个按钮不叫「保存密钥」，避免和面板里的保存按钮混淆', async () => {
    const wrapper = mountView()
    await flushPromises()

    // 面板里的保存按钮是主按钮，文案才是「保存密钥」。
    const saveButtons = wrapper
      .findAll('button')
      .filter((button) => button.text().includes('aiImage.key.save'))
    expect(saveButtons).toHaveLength(1)

    // 页头那个应当叫「填入密钥」。
    expect(wrapper.text()).toContain('aiImage.key.configure')
  })

  it('密钥可用之后仍然能重新打开面板更换密钥', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('sk-image-key')
    await wrapper.get('input[type="password"]').trigger('keyup.enter')
    await flushPromises()

    // 保存成功后面板收起，页头变成「更换密钥」。
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('aiImage.key.change')

    const change = wrapper
      .findAll('button')
      .find((button) => button.text().includes('aiImage.key.change'))!
    await change.trigger('click')
    await flushPromises()

    // 关键：必须能重新打开，否则用户永远换不了密钥。
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('aiImage.key.clear')
  })
})

/**
 * 视觉模型密钥（主人选的 B 路线）。
 *
 * 背景：平台按分组隔离模型。生图分组只绑生图渠道，分组里没有视觉模型，
 * 拿生图密钥去调视觉模型时网关在**路由阶段**就返回 503，请求到不了上游。
 * 所以「AI 识别卖点」「拆解爆款版式」要单独一把能调视觉模型的密钥。
 */
describe('AI生图 视觉模型密钥', () => {
  /** 视觉密钥输入框是页面上第二个 password 输入框（第一个是生图密钥）。 */
  function visionKeyInput(wrapper: VueWrapper) {
    const inputs = wrapper.findAll('input[type="password"]')
    if (inputs.length < 2) throw new Error('vision key input not found')
    return inputs[1]
  }

  /**
   * 密钥面板在生图密钥保存成功后会收起，视觉密钥框跟着一起藏起来。
   * 真实用户要走「更换密钥」把它重新摊开——测试也走同一条路。
   */
  async function openKeyPanel(wrapper: VueWrapper) {
    if (wrapper.findAll('input[type="password"]').length >= 2) return
    const toggle = wrapper
      .findAll('button')
      .find(
        (item) =>
          item.text().includes('aiImage.key.change') || item.text().includes('aiImage.key.configure'),
      )
    if (!toggle) throw new Error('key panel toggle not found')
    await toggle.trigger('click')
    await flushPromises()
  }

  function saveVisionKeyButton(wrapper: VueWrapper) {
    const button = wrapper
      .findAll('button')
      .find((item) => item.text().includes('aiImage.visionKey.save'))
    if (!button) throw new Error('vision key save button not found')
    return button
  }

  /** 打开脚本面板、挂上参考图，点「AI 识别卖点」。 */
  async function runAiExtract(wrapper: VueWrapper) {
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    const vm = wrapper.vm as unknown as { onReferencePicked: (e: unknown) => void }
    vm.onReferencePicked({ target: { files: [fakeImageFile('product.png')], value: '' } })
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.aiExtract'))!.trigger('click')
    await flushPromises()
  }

  it('密钥面板里有视觉模型密钥输入框，并提示去选 Gemini 渠道', async () => {
    const wrapper = mountView()
    await flushPromises()

    // 面板默认就是摊开的（还没有可用密钥），两把密钥都在。
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('aiImage.visionKey.sectionTitle')
    // 主人点名要的提示：明确写「去选 Gemini 渠道的密钥」。
    expect(wrapper.text()).toContain('aiImage.visionKey.hint')
    // 保存按钮的文案必须和生图密钥的「保存密钥」区分开。
    expect(saveVisionKeyButton(wrapper).text()).toContain('aiImage.visionKey.save')
  })

  it('视觉模型的框里也提示选 Gemini 渠道', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.visionKey.modelLabel')
    expect(wrapper.text()).toContain('aiImage.visionKey.modelHint')
  })

  it('填好视觉密钥后存在单独的存储键里，不覆盖生图密钥', async () => {
    const wrapper = await withSavedKey()
    await openKeyPanel(wrapper)

    await visionKeyInput(wrapper).setValue('sk-vision-key')
    await saveVisionKeyButton(wrapper).trigger('click')
    await flushPromises()

    expect(aiImageApi.validateVisionKey).toHaveBeenCalledWith('sk-vision-key')
    expect(window.localStorage.getItem(VISION_KEY_STORAGE)).toBe('sk-vision-key')
    // 生图密钥必须原样保留——两把密钥互不覆盖。
    expect(window.localStorage.getItem(KEY_STORAGE)).toBe('sk-image-key')
    expect(wrapper.text()).toContain('aiImage.visionKey.valid')
  })

  it('视觉密钥校验失败时给出提示，不会当成可用', async () => {
    aiImageApi.validateVisionKey.mockResolvedValue({ ok: false, reason: 'not-owned' })
    const wrapper = await withSavedKey()
    await openKeyPanel(wrapper)

    await visionKeyInput(wrapper).setValue('sk-someone-elses')
    await saveVisionKeyButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.visionKey.invalidTitle')
    expect(window.localStorage.getItem(VISION_KEY_STORAGE)).toBeNull()
  })

  it('AI 识别卖点用的是视觉密钥，不是生图密钥', async () => {
    const wrapper = await withSavedKey()
    await openKeyPanel(wrapper)
    await visionKeyInput(wrapper).setValue('sk-vision-key')
    await saveVisionKeyButton(wrapper).trigger('click')
    await flushPromises()

    await runAiExtract(wrapper)

    expect(aiImageApi.extractSellingPoints).toHaveBeenCalledTimes(1)
    expect(aiImageApi.extractSellingPoints.mock.calls[0][0].apiKey).toBe('sk-vision-key')
  })

  it('没填视觉密钥时退回用生图密钥（不破坏旧行为）', async () => {
    const wrapper = await withSavedKey()

    await runAiExtract(wrapper)

    expect(aiImageApi.extractSellingPoints).toHaveBeenCalledTimes(1)
    expect(aiImageApi.extractSellingPoints.mock.calls[0][0].apiKey).toBe('sk-image-key')
  })

  it('一把密钥都没有时给出「请先填视觉模型密钥」而不是静默无反应', async () => {
    const wrapper = mountView()
    await flushPromises()

    await runAiExtract(wrapper)

    expect(aiImageApi.extractSellingPoints).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('aiImage.visionKey.needed')
  })

  it('分组调不通视觉模型时，给出专属话术并摊开密钥面板', async () => {
    // 网关在路由阶段就拒了：响应体只有这一句，请求根本没到上游。
    aiImageApi.extractSellingPoints.mockRejectedValue(
      new aiImageApi.AIImageError('Service temporarily unavailable', 'api_error', 503),
    )
    const wrapper = await withSavedKey()
    await runAiExtract(wrapper)

    // 关键：不能只显示 "Service temporarily unavailable"，否则用户会一直重试。
    expect(wrapper.text()).toContain('aiImage.visionKey.failedRouting')
    // 话术和修复入口要在同一个视野里：密钥面板自动摊开。
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(2)
  })

  it('刷新页面后自动恢复上次保存的视觉密钥', async () => {
    window.localStorage.setItem(VISION_KEY_STORAGE, 'sk-vision-key')

    const wrapper = mountView()
    await flushPromises()

    expect(aiImageApi.validateVisionKey).toHaveBeenCalledWith('sk-vision-key')
    expect((visionKeyInput(wrapper).element as HTMLInputElement).value).toBe('sk-vision-key')
  })
})

describe('AI生图 功能页面互斥（一次只开一个）', () => {
  /** 当前展开着哪些功能页面。 */
  function openPanels(wrapper: VueWrapper): string[] {
    // 水印面板的 testid 历史上叫 enhance-panel，别按 id 拼。
    const ids: Array<[string, string]> = [
      ['single', 'single-panel'],
      ['hot', 'hot-panel'],
      ['script', 'script-panel'],
      ['template', 'template-panel'],
      ['composite', 'composite-panel'],
      ['watermark', 'enhance-panel'],
    ]
    return ids.filter(([, testid]) => wrapper.find(`[data-testid="${testid}"]`).exists()).map(([id]) => id)
  }

  it('默认一个功能页面都不展开', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(openPanels(wrapper)).toEqual([])
  })

  it('点开一个功能页面后，再点另一个，前一个会收起来', async () => {
    const wrapper = await withKeyOnly()

    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['hot'])

    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['script'])

    await wrapper.get('[data-testid="toggle-template-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['template'])

    await wrapper.get('[data-testid="toggle-composite-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['composite'])

    await wrapper.get('[data-testid="toggle-enhance-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['watermark'])

    // 单张生图也不例外：主人明确说了「单张图片生成也不行」。
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['single'])
  })

  it('再点一次同一个按钮就把它收起来', async () => {
    const wrapper = await withKeyOnly()

    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual(['hot'])

    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    expect(openPanels(wrapper)).toEqual([])
  })

  it('单张生图是一个按钮，不再是常驻的输入框', async () => {
    const wrapper = await withKeyOnly()

    // 没展开时，提示词输入框不该出现在页面上。
    expect(wrapper.find('[data-testid="prompt-input"]').exists()).toBe(false)

    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="prompt-input"]').exists()).toBe(true)
  })
})

describe('AI生图 脚本卡片放在本功能结果的上方，且一次只展开一张', () => {
  /** 等一个条件成立（拆解是异步的）。 */
  async function waitUntil(check: () => boolean, timeout = 4000) {
    const start = Date.now()
    while (Date.now() - start < timeout) {
      if (check()) return
      await new Promise((resolve) => setTimeout(resolve, 25))
    }
    throw new Error('等待超时')
  }

  /** 生成一套套图脚本（scriptSource = ozon）。 */
  async function buildOzonCards() {
    const wrapper = await withKeyOnly()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()
    return wrapper
  }

  it('套图面板里，脚本列表在本功能结果区的上方', async () => {
    const wrapper = await buildOzonCards()

    const panel = wrapper.get('[data-testid="script-panel"]')
    const cards = panel.get('[data-testid="script-script-cards"]')
    // 「本功能的结果」那个标题
    const resultsHeading = panel
      .findAll('h3')
      .find((item) => item.text().includes('aiImage.result.thisPanel'))!

    // 脚本必须在结果区之前（DOM 顺序就是视觉顺序）。
    const relation = cards.element.compareDocumentPosition(resultsHeading.element)
    expect(relation & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('脚本卡片不再长在底部那条批量栏里（已移到各自面板）', async () => {
    const wrapper = await buildOzonCards()
    const batchBar = wrapper.get('[data-testid="batch-bar"]')
    expect(batchBar.find('[data-testid^="script-card-toggle-"]').exists()).toBe(false)
    // 但「一键生成全部」还在那条栏里。
    expect(batchBar.find('[data-testid="generate-all-script-cards"]').exists()).toBe(true)
  })

  /**
   * 主人 2026-10-17 报的现象：点第一张卡片的提示词后，
   * 同一行的其它卡片也冒出一个空框、里面没内容。
   *
   * 根因是 grid 的默认行为：同一行的子项会被拉成一样高，
   * 一张展开后其它卡片被撑高，看着就像「也展开了但没内容」。
   * 修法是 `items-start`（见 AIScriptCards.vue）。
   * 这里断言「同一时间只有一个编辑框存在」，防止以后又变成同时开好几个。
   */
  it('展开一张卡片时，其它卡片不会跟着出现空的编辑框', async () => {
    const wrapper = await buildOzonCards()

    // 一开始没有任何编辑框。
    expect(wrapper.findAll('[data-testid^="script-card-text-"]')).toHaveLength(0)

    await wrapper.get('[data-testid="script-card-toggle-01_main_image"]').trigger('click')
    await flushPromises()

    const boxes = wrapper.findAll('[data-testid^="script-card-text-"]')
    expect(boxes).toHaveLength(1)
    // 而且这个框里必须有内容，不能是空的。
    expect((boxes[0].element as HTMLTextAreaElement).value.length).toBeGreaterThan(100)

    // 再点另一张：只应换一张，不会变成两个。
    await wrapper.get('[data-testid="script-card-toggle-04_detail_texture"]').trigger('click')
    await flushPromises()
    const boxes2 = wrapper.findAll('[data-testid^="script-card-text-"]')
    expect(boxes2).toHaveLength(1)
    expect((boxes2[0].element as HTMLTextAreaElement).value).toContain('材质纹理')
  })

  it('爆款复刻面板里同样把脚本放在本功能结果的上方', async () => {
    const wrapper = await withKeyOnly()
    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    aiImageApi.analyzeHotStyle.mockResolvedValue(hotLayoutJson('УВАЖАЕМЫЕ ПОКУПАТЕЛИ'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper)
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    const panel = wrapper.get('[data-testid="hot-panel"]')
    const cards = panel.get('[data-testid="hot-script-cards"]')
    const resultsHeading = panel
      .findAll('h3')
      .find((item) => item.text().includes('aiImage.result.thisPanel'))!
    const relation = cards.element.compareDocumentPosition(resultsHeading.element)
    expect(relation & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    // 2026-10-19 主人要求：一键生成全部也在脚本下方、本功能结果上方。
    const batchBar = panel.get('[data-testid="batch-bar"]')
    const bar = batchBar.element.compareDocumentPosition(resultsHeading.element)
    expect(bar & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    const afterCards = cards.element.compareDocumentPosition(batchBar.element)
    expect(afterCards & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

describe('AI生图 功能页各自都能写提示词（不能有死胡同）', () => {
  it('提示词模板页自带一个输入框，而且和单张生图页是同一份内容', async () => {
    const wrapper = await withKeyOnly()

    await wrapper.get('[data-testid="toggle-template-panel"]').trigger('click')
    await flushPromises()

    // 模板页必须有地方写提示词 —— 不能只有「下拉 + 保存」。
    const templateBox = wrapper.get('[data-testid="template-prompt"]')

    await templateBox.setValue('俄语主图，白底')
    await flushPromises()

    // 切到单张生图，内容应当已经带过去了（两个框绑的是同一个 prompt）。
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="prompt-input"]').element as HTMLTextAreaElement).value).toBe('俄语主图，白底')
  })

  it('在模板页写提示词就能直接存成模板', async () => {
    const wrapper = await withKeyOnly()

    await wrapper.get('[data-testid="toggle-template-panel"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="template-prompt"]').setValue('厨房场景主图')
    await wrapper.get('[data-testid="template-save"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="template-name"]').setValue('厨房模板')
    await wrapper.get('[data-testid="template-confirm"]').trigger('click')
    await flushPromises()

    // 存完自动选中，下拉里能看到它。
    const select = wrapper.get('[data-testid="template-select"]').element as HTMLSelectElement
    expect(select.value).not.toBe('')
    expect(select.selectedOptions[0]?.textContent).toContain('厨房模板')
  })

  it('套用模板时留在模板页，不把用户甩到别的页面', async () => {
    const wrapper = await withKeyOnly()

    // 先存一个模板。
    await wrapper.get('[data-testid="toggle-template-panel"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="template-prompt"]').setValue('白底主图')
    await wrapper.get('[data-testid="template-save"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="template-name"]').setValue('白底模板')
    await wrapper.get('[data-testid="template-confirm"]').trigger('click')
    await flushPromises()

    // 清空提示词，再套用模板 —— 内容应该回来了，而且人还在模板页。
    await wrapper.get('[data-testid="template-prompt"]').setValue('')
    await flushPromises()
    await wrapper.get('[data-testid="template-select"]').setValue(
      (wrapper.get('[data-testid="template-select"]').element as HTMLSelectElement).options[1].value,
    )
    await wrapper.get('[data-testid="template-select"]').trigger('change')
    await flushPromises()

    expect(wrapper.find('[data-testid="template-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="single-panel"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="template-prompt"]').element as HTMLTextAreaElement).value).toBe('白底主图')
  })
})

describe('AI生图 每个功能只看自己的结果', () => {
  const seeded = [
    {
      id: 's1',
      prompt: '单张：换背景',
      model: 'gpt-image-2',
      size: '1200x1600',
      status: 'completed',
      images: ['https://cdn.example/single.png'],
      error: '',
      createdAt: 1,
      source: 'single',
    },
    {
      id: 'h1',
      prompt: '爆款：复刻版式',
      model: 'gpt-image-2',
      size: '1200x1600',
      status: 'completed',
      images: ['https://cdn.example/hot.png'],
      error: '',
      createdAt: 2,
      source: 'hot',
    },
    {
      id: 'c1',
      prompt: '套图：第 1 张',
      model: 'gpt-image-2',
      size: '1200x1600',
      status: 'completed',
      images: ['https://cdn.example/script.png'],
      error: '',
      createdAt: 3,
      source: 'script',
    },
    {
      // 升级前存下来的老记录：没有 source，推断不出出处。
      id: 'old1',
      prompt: '老记录',
      model: 'gpt-image-2',
      size: '1200x1600',
      status: 'completed',
      images: ['https://cdn.example/old.png'],
      error: '',
      createdAt: 4,
    },
  ]

  it('功能页面里只显示这个功能自己的图，默认是空的', async () => {
    const wrapper = await withKeyOnly()
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()

    const panel = wrapper.get('[data-testid="single-panel"]')
    // 还没生成过任何东西，所以这里应该是空状态。
    expect(panel.text()).toContain('aiImage.result.panelEmpty')
    expect(panel.find('[data-testid="result-image-0"]').exists()).toBe(false)
  })

  it('各功能页只装自己的记录，底部汇总区装全部', async () => {
    seedRecords(seeded)
    const wrapper = await withKeyOnly()

    // 单张生图页：只有 single 那一条。
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()
    let panel = wrapper.get('[data-testid="single-panel"]')
    expect(panel.text()).toContain('单张：换背景')
    expect(panel.text()).not.toContain('爆款：复刻版式')
    expect(panel.text()).not.toContain('套图：第 1 张')
    // 没有 source 的老记录也不该混进功能页 —— 宁可少显示，也不要显示错的。
    expect(panel.text()).not.toContain('老记录')

    // 爆款复刻页：只有 hot 那一条。
    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    panel = wrapper.get('[data-testid="hot-panel"]')
    expect(panel.text()).toContain('爆款：复刻版式')
    expect(panel.text()).not.toContain('单张：换背景')
    expect(panel.text()).not.toContain('套图：第 1 张')

    // 套图页：只有 script 那一条。
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    panel = wrapper.get('[data-testid="script-panel"]')
    expect(panel.text()).toContain('套图：第 1 张')
    expect(panel.text()).not.toContain('单张：换背景')
    expect(panel.text()).not.toContain('爆款：复刻版式')

    // 底部汇总区：四条全在，包括没有 source 的老记录。
    const all = wrapper.get('[data-testid="all-results"]')
    for (const text of ['单张：换背景', '爆款：复刻版式', '套图：第 1 张', '老记录']) {
      expect(all.text()).toContain(text)
    }
  })

  it('单张生图做出来的记录带 source=single', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="prompt-input"]').setValue('换成白色背景')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    // 生成后应当出现在「单张生图」页自己的结果区里。
    const panel = wrapper.get('[data-testid="single-panel"]')
    expect(panel.text()).toContain('换成白色背景')
  })
})

describe('AI生图结果区', () => {
  it('失败记录显示失败原因', async () => {
    seedRecords([
      {
        id: 'r1',
        prompt: '把背景换成厨房',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'failed',
        images: [],
        error: '上游返回 429：额度不足',
        createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.result.failedReason')
    expect(wrapper.text()).toContain('上游返回 429：额度不足')
  })

  it('已完成记录渲染图片', async () => {
    seedRecords([
      {
        id: 'r2',
        prompt: '白色背景主图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['https://cdn.example/done.png'],
        error: '',
        createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('img[src="https://cdn.example/done.png"]').exists()).toBe(true)
  })

  it('状态筛选只留下对应状态的记录', async () => {
    seedRecords([
      {
        id: 'done',
        prompt: '已完成的图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['https://cdn.example/done.png'],
        error: '',
        createdAt: Date.now(),
      },
      {
        id: 'bad',
        prompt: '失败的图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'failed',
        images: [],
        error: '模型不支持该尺寸',
        createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('已完成的图')
    expect(wrapper.text()).toContain('失败的图')

    await statusSelect(wrapper).setValue('failed')
    await flushPromises()

    expect(wrapper.text()).not.toContain('已完成的图')
    expect(wrapper.text()).toContain('失败的图')

    await statusSelect(wrapper).setValue('all')
    await flushPromises()
    expect(wrapper.text()).toContain('已完成的图')
  })
})

describe('AI生图商品一致性锁定', () => {
  it('默认就是开启的', async () => {
    const wrapper = await withSavedKey()

    const toggle = wrapper.get('input[type="checkbox"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.text()).toContain('aiImage.lock.on')
  })

  it('上传参考图后，发给模型的提示词带上了商品一致性锁定', async () => {
    const wrapper = await withSavedKey()
    await attachReference(wrapper)

    await wrapper.get('textarea').setValue('换成白色背景')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    expect(aiImageApi.generateAIImage).toHaveBeenCalledTimes(1)
    const request = aiImageApi.generateAIImage.mock.calls[0][0]
    expect(request.prompt.startsWith(PRODUCT_LOCK)).toBe(true)
    expect(request.prompt).toContain('换成白色背景')
    expect(request.referenceImage).toBeInstanceOf(File)
  })

  it('关掉锁定后，不再拼商品锁定，但「不许画中文」这条尾巴还在', async () => {
    const wrapper = await withSavedKey()
    await attachReference(wrapper)

    await wrapper.get('input[type="checkbox"]').setValue(false)
    await wrapper.get('textarea').setValue('换成白色背景')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    const request = aiImageApi.generateAIImage.mock.calls[0][0]
    // 用户原话要在，而且排在最前面（前面只允许有「中文只是指令」的说明）。
    expect(request.prompt).toContain('换成白色背景')
    expect(request.prompt.indexOf('换成白色背景')).toBeLessThan(320)
    expect(request.prompt).not.toContain('PRODUCT CONSISTENCY LOCK')
    // 2026-10-19：结尾一律再压一条「不许画中文」，这一条跟锁定无关，不能跟着关掉。
    expect(request.prompt).toContain('[Hard rule — read last]')
    expect(request.prompt).toContain('Not one Chinese character may appear on the image')
  })

  it('没上传参考图时，即使是纯文生图也不会加锁', async () => {
    const wrapper = await withSavedKey()

    await wrapper.get('textarea').setValue('一个白色背景的保温杯')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    const request = aiImageApi.generateAIImage.mock.calls[0][0]
    // 没有参考图就没有「参考图里的商品」可锁，加了反而误导模型。
    expect(request.prompt).toContain('一个白色背景的保温杯')
    expect(request.prompt).not.toContain('PRODUCT CONSISTENCY LOCK')
    expect(request.referenceImage).toBeNull()
  })

  it('提示用户：没上传参考图时锁定不生效', async () => {
    const wrapper = await withSavedKey()
    expect(wrapper.text()).toContain('aiImage.lock.needsReference')
  })
})

describe('AI生图 排版：功能模块在顶部，提示词顺位向下', () => {
  // 主人要求：把「爆款复刻」这类功能模块按钮放到顶部显眼处，提示词输入框顺位向下排。
  it('功能模块栏在提示词输入框之前', async () => {
    const wrapper = await withSavedKey()
    const html = wrapper.html()

    const moduleIndex = html.indexOf('data-testid="module-bar"')
    const promptIndex = html.indexOf('aiImage.prompt.label')

    expect(moduleIndex).toBeGreaterThan(-1)
    expect(promptIndex).toBeGreaterThan(-1)

    // 模块栏 → 提示词
    expect(moduleIndex).toBeLessThan(promptIndex)

    // 2026-10-19：批量条不再是页面底部那条常驻横条，而是跟着脚本进功能页，
    // 没有脚本时页面上根本没有 batch-bar。
    expect(wrapper.find('[data-testid="batch-bar"]').exists()).toBe(false)
  })

  it('五个模块按钮都在模块栏里，且默认都是收起的', async () => {
    const wrapper = await withSavedKey()
    const bar = wrapper.get('[data-testid="module-bar"]')

    for (const id of [
      'toggle-hot-panel',
      'toggle-template-panel',
      'toggle-composite-panel',
      'toggle-enhance-panel',
      'toggle-script-panel',
    ]) {
      expect(bar.find(`[data-testid="${id}"]`).exists()).toBe(true)
    }

    // 默认收起：面板都不在。
    expect(wrapper.find('[data-testid="hot-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="template-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="enhance-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="script-panel"]').exists()).toBe(false)
  })

  it('模块分两档：主要功能是爆款复刻和套图，其余归辅助功能', async () => {
    const wrapper = await withSavedKey()
    const primary = wrapper.get('[data-testid="primary-modules"]')
    const secondary = wrapper.get('[data-testid="secondary-modules"]')

    // 「套图」原来在辅助功能里，主人 2026-10-15 要求提到主要功能。
    expect(primary.find('[data-testid="toggle-hot-panel"]').exists()).toBe(true)
    expect(primary.find('[data-testid="toggle-script-panel"]').exists()).toBe(true)

    for (const id of ['toggle-template-panel', 'toggle-composite-panel', 'toggle-enhance-panel']) {
      expect(secondary.find(`[data-testid="${id}"]`).exists()).toBe(true)
    }
    // 反过来：主要功能里不该混进辅助的那三个。
    expect(primary.find('[data-testid="toggle-template-panel"]').exists()).toBe(false)
    expect(primary.find('[data-testid="toggle-composite-panel"]').exists()).toBe(false)
    expect(primary.find('[data-testid="toggle-enhance-panel"]').exists()).toBe(false)
  })

  it('每个模块按钮都能各自展开对应的面板', async () => {
    const wrapper = await withSavedKey()

    await wrapper.get('[data-testid="toggle-template-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="template-panel"]').exists()).toBe(true)

    await wrapper.get('[data-testid="toggle-composite-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="composite-panel"]').exists()).toBe(true)

    await wrapper.get('[data-testid="toggle-enhance-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="enhance-panel"]').exists()).toBe(true)
  })

  /**
   * 【2026-10-19 主人要求】「一键生成全部」的位置要和脚本一样 ——
   * 在「本功能的结果」上方、脚本的下方。所以批量条不再是页面底部那条常驻横条，
   * 而是跟着脚本进到「爆款复刻 / 套图」各自的功能页里。
   */
  it('批量条跟着脚本进功能页：没有脚本时不出现，有脚本时暂停/停止置灰', async () => {
    const wrapper = await withSavedKey()
    // 还没生成任何脚本 → 页面上没有批量条。
    expect(wrapper.find('[data-testid="batch-bar"]').exists()).toBe(false)

    // 生成一套套图脚本之后，批量条出现在套图面板里。
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    const panel = wrapper.get('[data-testid="script-panel"]')
    const pause = panel.get('[data-testid="batch-pause"]')
    const stop = panel.get('[data-testid="batch-stop"]')
    // 没在跑 → 两个都置灰。
    expect(pause.attributes('disabled')).toBeDefined()
    expect(stop.attributes('disabled')).toBeDefined()
    // 但按钮本身是看得见的（主人两次说「找不到暂停/停止」）。
    expect(wrapper.get('[data-testid="batch-pause"]').isVisible()).toBe(true)
  })
})

describe('AI生图 Ozon 主图脚本（8 张）', () => {
  async function openScriptPanel() {
    const wrapper = await withKeyOnly()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  it('展开后能看到脚本面板', async () => {
    const wrapper = await withKeyOnly()
    expect(wrapper.text()).not.toContain('aiImage.script.build')

    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('aiImage.script.build')
  })

  it('填了产品名就能生成八张脚本（主图打头，末尾是联系卖家图）', async () => {
    const wrapper = await openScriptPanel()

    const productInput = wrapper.findAll('input').find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))
    expect(productInput).toBeDefined()
    await productInput!.setValue('316 不锈钢保温杯')

    const build = wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!
    await build.trigger('click')
    await flushPromises()

    // 2026-10-24 主人重排了顺序：第 1 张改成主图（商品原样 + 俄语卖点大字）。
    expect(wrapper.text()).toContain('第 1 张 · 主图（商品原样 + 大字卖点详情）')
    // 痛点对比独立成第 2 张，因为「对比必须是同一件商品」混在合成图里容易串货。
    expect(wrapper.text()).toContain('第 2 张 · 痛点对比（同类不同款）')
    expect(wrapper.text()).toContain('第 3 张 · 生活场景图（模特正在使用）')
    expect(wrapper.text()).not.toContain('合规主图')
    // 三张细节图：角度不同、部位不同、不许有 logo。
    expect(wrapper.text()).toContain('第 4 张 · 细节①材质纹理')
    expect(wrapper.text()).toContain('第 5 张 · 细节②结构部位')
    expect(wrapper.text()).toContain('第 6 张 · 细节③功能效果（防水/防油污）')
    // 2026-10-24：售后保障图整张删掉，只留一张联系卖家图。
    expect(wrapper.text()).not.toContain('售后保障图')
    expect(wrapper.text()).toContain('第 8 张 · 售后与联系卖家图')
    expect(wrapper.text()).not.toContain('角度·正俯视')
    expect(wrapper.text()).not.toContain('角度·正侧视')
  })

  // 2026-10-24 主人要求「顺序需要提前让我知道」：
  // 点「生成脚本」之前，面板里就要按顺序列出这一套 8 张。
  /**
   * 【2026-10-24 主人要求】「套图不显示出图顺序这些内容，防止别的照抄」。
   * 生成脚本之前那一块顺序列表已经整块删掉，这条守住「不要又加回来」。
   */
  it('生成脚本之前不再显示整张清单（防照抄）', async () => {
    const wrapper = await openScriptPanel()
    expect(wrapper.find('[data-testid="script-plan"]').exists()).toBe(false)
  })

  it('产品名会自动匹配场景并显示出来', async () => {
    const wrapper = await openScriptPanel()

    const productInput = wrapper.findAll('input').find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('保温杯')
    await flushPromises()

    expect(wrapper.text()).toContain('办公室桌面')
  })

  it('点「填入提示词」把这一张的提示词填进输入框', async () => {
    const wrapper = await openScriptPanel()

    const productInput = wrapper.findAll('input').find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    // 卡片现在默认收起，先展开第 1 张。
    await wrapper.get('[data-testid="script-card-toggle-01_main_image"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="script-card-fill-01_main_image"]').trigger('click')
    await flushPromises()

    const textarea = wrapper.get('[data-testid="prompt-input"]')
    const value = (textarea.element as HTMLTextAreaElement).value
    expect(value).toContain('Ozon 商品主图，3:4 竖版')
    expect(value).toContain('主图')
  })

  it('没填产品名就生成会给出提示而不是空白脚本', async () => {
    const wrapper = await openScriptPanel()

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('第 1 张 · 合规主图')
  })

  it('AI 识别卖点会把结果填进表单', async () => {
    const wrapper = await openScriptPanel()
    await attachReference(wrapper)

    const aiButton = wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.aiExtract'))!
    await aiButton.trigger('click')
    await flushPromises()

    expect(aiImageApi.extractSellingPoints).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('aiImage.script.aiExtracted')

    const productInput = wrapper.findAll('input').find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    expect((productInput.element as HTMLInputElement).value).toBe('316 不锈钢保温杯')
  })

  it('没上传产品图时 AI 识别会提示先传图', async () => {
    const wrapper = await openScriptPanel()

    const aiButton = wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.aiExtract'))!
    await aiButton.trigger('click')
    await flushPromises()

    expect(aiImageApi.extractSellingPoints).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('aiImage.script.aiNeedsImage')
  })
})

describe('AI生图 商品编号与图片地址（给客户端用）', () => {
  function productIdInput(wrapper: VueWrapper) {
    // t() 回显 key，所以占位符就是这条 key 本身。
    const input = wrapper.findAll('input').find((item) => item.attributes('placeholder') === 'aiImage.product.placeholder')
    if (!input) throw new Error('product id input not found')
    return input
  }

  it('填了商品编号后，生成的记录会带上它', async () => {
    const wrapper = await withSavedKey()

    await productIdInput(wrapper).setValue('SKU-8899')
    await wrapper.get('textarea').setValue('换成白色背景')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('SKU-8899')
  })

  it('商品编号会记住，下次进页面还在', async () => {
    const wrapper = await withSavedKey()
    await productIdInput(wrapper).setValue('SKU-8899')
    await flushPromises()

    // 新开的页面默认收起所有功能页，所以要先展开「单张生图」才看得到这个输入框。
    const again = mountView()
    await flushPromises()
    await openSinglePanel(again)
    expect((productIdInput(again).element as HTMLInputElement).value).toBe('SKU-8899')
  })

  it('生成的图有网址时，可以一键复制给客户端', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    seedRecords([
      {
        id: 'r-url',
        prompt: '白色背景主图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['https://cdn.example/out.png'],
        error: '',
        createdAt: Date.now(),
        productId: 'SKU-8899',
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    const copyButton = wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyUrl'))
    expect(copyButton).toBeDefined()
    await copyButton!.trigger('click')
    await flushPromises()

    // 复制出去的必须正好是客户端要下载的那个地址。
    expect(writeText).toHaveBeenCalledWith('https://cdn.example/out.png')
    expect(wrapper.text()).toContain('aiImage.product.copied')
  })

  it('返回的是 base64 时不给复制按钮，并说明没有可用地址', async () => {
    seedRecords([
      {
        id: 'r-b64',
        prompt: '白色背景主图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['data:image/png;base64,AAAA'],
        error: '',
        createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    // base64 客户端拿不到，所以不该提供「复制图片地址」。
    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyUrl'))).toBeUndefined()
    expect(wrapper.text()).toContain('aiImage.product.noUrl')
  })

  it('一次复制全部地址时，按生成先后排序（对上客户端的第 1、2、3 张）', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const base = Date.now()
    // 故意乱序传入，并且列表是新的在前；复制出去必须是早→晚。
    seedRecords([
      {
        id: 'r3', prompt: '第 3 张', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/3.png'], error: '', createdAt: base + 3000,
      },
      {
        id: 'r1', prompt: '第 1 张', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/1.png'], error: '', createdAt: base + 1000,
      },
      {
        id: 'r2', prompt: '第 2 张', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/2.png'], error: '', createdAt: base + 2000,
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    const copyAll = wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))
    expect(copyAll).toBeDefined()
    await copyAll!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith(
      ['https://cdn.example/1.png', 'https://cdn.example/2.png', 'https://cdn.example/3.png'].join('\n'),
    )
    expect(wrapper.text()).toContain('aiImage.product.copiedAll')
  })

  it('复制全部时跳过 base64，只给出客户端能下载的地址', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const base = Date.now()
    seedRecords([
      {
        id: 'ok', prompt: '有地址', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/ok.png'], error: '', createdAt: base,
      },
      {
        id: 'b64', prompt: '只有 base64', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['data:image/png;base64,AAAA'], error: '', createdAt: base + 1000,
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('https://cdn.example/ok.png')
  })

  it('一张可用地址都没有时不显示「复制全部」按钮', async () => {
    seedRecords([
      {
        id: 'gen', prompt: '还在生成', model: 'gpt-image-2', size: '1200x1600', status: 'generating',
        images: [], error: '', createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))).toBeUndefined()
  })

  it('复制全部会跟随状态下拉，只导出当前筛选里的地址', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const base = Date.now()
    seedRecords([
      {
        id: 'done', prompt: '成功的', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/done.png'], error: '', createdAt: base,
      },
      {
        id: 'bad', prompt: '失败的', model: 'gpt-image-2', size: '1200x1600', status: 'failed',
        images: [], error: '生成失败', createdAt: base + 1000,
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    await statusSelect(wrapper).setValue('completed')
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('https://cdn.example/done.png')
  })
})

describe('AI生图 批量生成（并发）', () => {
  /** 展开脚本面板、填产品名、生成八张脚本。 */
  async function withScriptCards() {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()

    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()
    return wrapper
  }

  function generateAllButton(wrapper: VueWrapper) {
    const button = wrapper.get('[data-testid="generate-all-script-cards"]')
    return button
  }

  it('八张脚本就绪后才有「一键生成全部」', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    // 还没生成脚本，按钮不存在。
    expect(wrapper.find('[data-testid="generate-all-script-cards"]').exists()).toBe(false)

    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="generate-all-script-cards"]').exists()).toBe(true)
  })

  // 主人真实反馈过「暂停/停止按钮我没找到」。
  // 原因是它们原来只在 batchRunning 时渲染，而八张图几秒就跑完，
  // 那个窗口太短，等于不存在。所以按钮必须一开始就在。
  it('暂停和停止按钮在开始之前就能看见', async () => {
    const wrapper = await withScriptCards()

    const pause = wrapper.get('[data-testid="batch-pause"]')
    const stop = wrapper.get('[data-testid="batch-stop"]')
    expect(pause.text()).toContain('aiImage.batch.pause')
    expect(stop.text()).toContain('aiImage.batch.stop')

    // 没在跑的时候置灰，用户看得出「现在不能点」。
    expect(pause.attributes('disabled')).toBeDefined()
    expect(stop.attributes('disabled')).toBeDefined()
  })

  it('批量跑起来之后暂停和停止变成可点', async () => {
    const wrapper = await withScriptCards()

    const pending: Array<{ resolve: (value: unknown) => void }> = []
    aiImageApi.generateAIImage.mockImplementation(
      () => new Promise((resolve) => { pending.push({ resolve }) }),
    )

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="batch-pause"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="batch-stop"]').attributes('disabled')).toBeUndefined()

    // 放行，避免测试结束时还挂着请求。
    pending.forEach((item) => item.resolve({ data: [{ url: 'https://cdn.example/x.png' }] }))
    await flushPromises()
  })

  it('一键生成会把八张脚本全部提交出去', async () => {
    const wrapper = await withScriptCards()

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    expect(aiImageApi.generateAIImage).toHaveBeenCalledTimes(8)
  })

  /**
   * 主人反馈：「在生成每个图片的提示词的时候不满意要修改的时候，修改完再点回去还是原来的」。
   *
   * 病根是脚本卡片当时只是个**只读按钮**：点一下把文字复制到下面的输入框，
   * 改了也回不到卡片里。现在卡片可以展开编辑，生成时读的是卡片里的内容。
   */
  it('在卡片里改了提示词，一键生成时用的就是改后的内容', async () => {
    const wrapper = await withScriptCards()

    // 展开第 1 张，改掉它。
    await wrapper.get('[data-testid="script-card-toggle-01_main_image"]').trigger('click')
    await flushPromises()
    const box = wrapper.get('[data-testid="script-card-text-01_main_image"]')
    await box.setValue('我自己改的提示词：纯黑底，产品居中')
    await flushPromises()
    await wrapper.get('[data-testid="script-card-save-01_main_image"]').trigger('click')
    await flushPromises()

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    const prompts = aiImageApi.generateAIImage.mock.calls.map(
      (call) => (call[0] as { prompt: string }).prompt,
    )
    // 第 1 张发的必须是改后的内容。
    expect(prompts[0]).toContain('我自己改的提示词：纯黑底，产品居中')
    // 而且改过的卡片会额外压一段英文硬规则（不许画中文等）。
    expect(prompts[0].toLowerCase()).toContain('never render chinese')
    // 没改过的第 2 张仍然发模板的英文版。
    expect(prompts[1]).toContain('Card 2:')
  })

  it('没改过的卡片发的是模板英文版，不是中文', async () => {
    const wrapper = await withScriptCards()

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    const prompts = aiImageApi.generateAIImage.mock.calls.map(
      (call) => (call[0] as { prompt: string }).prompt,
    )
    for (const prompt of prompts) {
      // 用户看到的是中文，但发给模型的是英文（省掉运行时翻译的钱）。
      expect(prompt).toContain('Card ')
      expect(prompt).not.toContain('第1张：')
    }
  })

  it('「全部还原」把改过的卡片恢复成模板原文', async () => {
    const wrapper = await withScriptCards()

    await wrapper.get('[data-testid="script-card-toggle-01_main_image"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="script-card-text-01_main_image"]').setValue('乱改的')
    await flushPromises()
    await wrapper.get('[data-testid="script-card-save-01_main_image"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="script-reset-edits"]').trigger('click')
    await flushPromises()

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()
    const prompts = aiImageApi.generateAIImage.mock.calls.map(
      (call) => (call[0] as { prompt: string }).prompt,
    )
    expect(prompts[0]).not.toContain('乱改的')
    expect(prompts[0]).toContain('Card 1:')
  })

  it('批量里每张只要 1 张图，不会按张数设置成倍花钱', async () => {
    const wrapper = await withScriptCards()
    // 把张数设成 4，批量仍应每张只要 1 张。
    const countInput = wrapper.findAll('input[type="number"]')[0]
    if (countInput) await countInput.setValue('4')

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    const counts = aiImageApi.generateAIImage.mock.calls.map(
      (call) => (call[0] as { n?: number }).n,
    )
    expect(counts).toHaveLength(8)
    expect(counts.every((n) => n === 1)).toBe(true)
  })

  it('提交顺序就是脚本顺序，且并发不超过所选并发数', async () => {
    const wrapper = await withScriptCards()

    // 用可控的 promise 代替定时器：这样才能精确观察「同时在跑几个」。
    const pending: Array<{ index: number; prompt: string; done: boolean; resolve: (v: unknown) => void }> = []
    let active = 0
    let peak = 0
    aiImageApi.generateAIImage.mockImplementation((request: { prompt: string }) => {
      const index = pending.length + 1
      active += 1
      peak = Math.max(peak, active)
      return new Promise((resolve) => {
        pending.push({
          index,
          prompt: request.prompt,
          done: false,
          resolve: (value: unknown) => {
            active -= 1
            resolve(value)
          },
        })
      })
    })

    /** 把当前挂起的请求全部放行，重复到不再有新请求为止。 */
    async function settleAll(maxRounds = 30) {
      for (let round = 0; round < maxRounds; round += 1) {
        await flushPromises()
        const waiting = pending.filter((item) => !item.done)
        if (!waiting.length) return
        waiting.forEach((item) => {
          item.done = true
          item.resolve({ data: [{ url: `https://cdn.example/img-${item.index}.png` }] })
        })
      }
    }

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    // 并发数默认 2：一开跑就该正好挂起 2 个。
    expect(pending).toHaveLength(2)
    expect(peak).toBe(2)

    await settleAll()

    expect(aiImageApi.generateAIImage).toHaveBeenCalledTimes(8)
    expect(peak).toBeLessThanOrEqual(2)

    // 提交顺序必须和脚本卡片的顺序完全一致，否则客户端上架时主图顺序会错位。
    //
    // 注意：**发给模型的是英文版**（`promptEn`），不是卡片上显示的中文。
    // 所以这里要拿卡片里的英文版来比对，不能拿界面上的中文标题。
    const vm = wrapper.vm as unknown as {
      scriptCards: Array<{ key: string; promptEn?: string }>
    }
    const expected = vm.scriptCards.map((card) => card.promptEn)
    expect(expected).toHaveLength(8)
    // 发出去的提示词 = 模板英文版 + 结尾那条「不许画中文」的硬规则，
    // 所以这里比的是前缀：前缀一一对上，就说明提交顺序和脚本顺序一致。
    expect(pending).toHaveLength(8)
    pending.forEach((item, index) => {
      // 模板英文版必须原样出现在发出去的内容里。
      // 前面可能多一段「正文里的中文只是指令」的说明（产品名是中文时就会出现），
      // 结尾一定多一条「不许画中文」的硬规则，所以用包含判断而不是前缀判断。
      expect(item.prompt).toContain(expected[index] as string)
      expect(item.prompt).toContain('[Hard rule — read last]')
    })
  })

  it('批量完成后，记录按提交顺序排列，复制的地址也是第 1 张在前', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const wrapper = await withScriptCards()

    const pending: Array<{ index: number; done: boolean; resolve: (v: unknown) => void }> = []
    aiImageApi.generateAIImage.mockImplementation(() => {
      const index = pending.length + 1
      return new Promise((resolve) => {
        pending.push({ index, done: false, resolve })
      })
    })

    /** 故意倒序放行：让「先提交的」最后返回，验证排序靠的是提交顺序。 */
    async function settleAllReverse(maxRounds = 30) {
      for (let round = 0; round < maxRounds; round += 1) {
        await flushPromises()
        const waiting = pending.filter((item) => !item.done)
        if (!waiting.length) return
        for (const item of waiting.slice().reverse()) {
          item.done = true
          item.resolve({ data: [{ url: `https://cdn.example/img-${item.index}.png` }] })
        }
      }
    }

    await generateAllButton(wrapper).trigger('click')
    await settleAllReverse()

    const copyAll = wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))
    expect(copyAll).toBeDefined()
    await copyAll!.trigger('click')
    await flushPromises()

    const copied = (writeText.mock.calls[0][0] as string).split('\n')
    expect(copied).toHaveLength(8)
    // 第 1 张脚本的图必须排第一，哪怕它是最后返回的。
    expect(copied[0]).toBe('https://cdn.example/img-1.png')
    expect(copied[7]).toBe('https://cdn.example/img-8.png')
  })

  it('批量里有失败时显示失败数量，成功的那几张照常保留', async () => {
    const wrapper = await withScriptCards()

    let call = 0
    aiImageApi.generateAIImage.mockImplementation(async () => {
      call += 1
      if (call === 2) throw new Error('上游炸了')
      return { data: [{ url: `https://cdn.example/ok-${call}.png` }] }
    })

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.batch.failedCount')
    // 7 张成功仍然可复制。
    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.product.copyAll'))).toBeDefined()
  })

  it('没密钥时点一键生成会把密钥面板打开，而不是静默失败', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()

    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    await generateAllButton(wrapper).trigger('click')
    await flushPromises()

    expect(aiImageApi.generateAIImage).not.toHaveBeenCalled()
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
  })
})

describe('AI生图 暂停/停止按钮（主人反馈「找不到」）', () => {
  /**
   * 2026-10-19 主人又改了一次位置要求：
   * 「页面的一键生成全部位置和生成的脚本位置一样，在本功能的结果上方，脚本的下方」。
   * 所以批量条不再常驻页面底部，而是**紧贴在脚本卡片下面** ——
   * 距离脚本只有几像素，不会再出现「找不到」。
   */
  it('批量条紧跟在脚本卡片下面，和脚本在同一个功能页里', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()
    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')
    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    const panel = wrapper.get('[data-testid="script-panel"]')
    const cards = panel.get('[data-testid="script-script-cards"]')
    const bar = panel.get('[data-testid="batch-bar"]')

    // 批量条排在脚本卡片后面（DOM 顺序就是视觉顺序）。
    const relation = cards.element.compareDocumentPosition(bar.element)
    expect(relation & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 全页只有一个批量条，不会出现「两个一键生成全部」。
    expect(wrapper.findAll('[data-testid="batch-bar"]')).toHaveLength(1)

    // 没在跑 → 置灰，但看得见。
    const pause = panel.get('[data-testid="batch-pause"]')
    const stop = panel.get('[data-testid="batch-stop"]')
    expect(pause.exists()).toBe(true)
    expect(stop.exists()).toBe(true)
    expect(pause.attributes('disabled')).toBeDefined()
    expect(stop.attributes('disabled')).toBeDefined()
  })
})

describe('AI生图 水印', () => {
  async function openWatermark() {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-enhance-panel"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  /**
   * 2026-11-05 布局变化：开着「水印」时右侧给水印自己的设置用，**不显示生图结果**，
   * 所以「加水印下载」按钮（它长在结果区里）这时看不见。
   * 设完水印后切到「单张生图」，结果区回来，才谈得上按钮出不出现。
   */
  async function openWatermarkThenSwitchToSingle() {
    const wrapper = await openWatermark()
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  it('水印面板默认收起，点了才展开', async () => {
    const wrapper = await withSavedKey()
    expect(wrapper.find('[data-testid="enhance-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="toggle-enhance-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="enhance-panel"]').exists()).toBe(true)
  })

  /**
   * 2026-10-22 主人要求「水印不能默认为熊管家AI，可以没有」。
   * 所以默认文字是空的 —— 谁把它改回品牌名，这条会红。
   */
  it('默认是关的，而且水印文字是空的（不替用户署名）', async () => {
    const wrapper = await openWatermark()
    const checkbox = wrapper.get('[data-testid="watermark-enabled"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-testid="watermark-text"]').element as HTMLInputElement).value).toBe('')
  })

  it('浓度滑块范围是桌面程序的 5~100', async () => {
    const wrapper = await openWatermark()
    const slider = wrapper.get('[data-testid="watermark-alpha"]')
    expect(slider.attributes('min')).toBe('5')
    expect(slider.attributes('max')).toBe('100')
  })

  it('没开水印时不显示「加水印下载」按钮', async () => {
    seedRecords([
      {
        id: 'r1', prompt: 'p', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/a.png'], error: '', createdAt: Date.now(),
      },
    ])
    const wrapper = await openWatermarkThenSwitchToSingle()

    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.watermark.download'))).toBeUndefined()
  })

  /**
   * 按钮的显示条件从「开关开着」变成了「开关开着 **且** 有文字」。
   * 因为默认文字是空的：只勾开关不填字，等于没水印可打，
   * 这时候露出按钮会让人点下去以为坏了。
   */
  it('只勾开关不填字，仍然不出现「加水印下载」按钮', async () => {
    seedRecords([
      {
        id: 'r1', prompt: 'p', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/a.png'], error: '', createdAt: Date.now(),
      },
    ])
    const wrapper = await openWatermark()

    await wrapper.get('[data-testid="watermark-enabled"]').setValue(true)
    await flushPromises()
    // 切到会出图的功能，结果区才看得见
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.watermark.download'))).toBeUndefined()
  })

  it('开关开着且填了字，才出现「加水印下载」按钮', async () => {
    seedRecords([
      {
        id: 'r1', prompt: 'p', model: 'gpt-image-2', size: '1200x1600', status: 'completed',
        images: ['https://cdn.example/a.png'], error: '', createdAt: Date.now(),
      },
    ])
    const wrapper = await openWatermark()

    await wrapper.get('[data-testid="watermark-enabled"]').setValue(true)
    await wrapper.get('[data-testid="watermark-text"]').setValue('我的店铺')
    await flushPromises()
    // 切到会出图的功能，结果区回来，按钮才该出现
    await wrapper.get('[data-testid="toggle-single-panel"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('button').find((item) => item.text().includes('aiImage.watermark.download'))).toBeDefined()
  })
})

describe('AI生图 爆款复刻', () => {
  async function openHotPanel() {
    const wrapper = await withKeyOnly()
    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    return wrapper
  }

  it('面板默认收起，点了才展开', async () => {
    const wrapper = await withKeyOnly()
    expect(wrapper.find('[data-testid="hot-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="toggle-hot-panel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="hot-panel"]').exists()).toBe(true)
  })

  it('没选爆款图时「拆解」按钮是灰的', async () => {
    const wrapper = await openHotPanel()
    expect(wrapper.get('[data-testid="hot-analyze"]').attributes('disabled')).toBeDefined()
  })

  it('没放自己的商品图时点拆解会提示先选商品图，不会白跑视觉接口', async () => {
    const wrapper = await openHotPanel()
    await pickHotRefs(wrapper)

    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await flushPromises()

    expect(aiImageApi.analyzeHotStyle).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('aiImage.hot.needProductImage')
  })

  /**
   * 主人 2026-10-19 的原话：「生图的数量必须以放入的参考图的数量一样」。
   * 所以这里放三张、断言正好三张脚本；一张图失败也不会把别人挤掉。
   */
  it('一张参考图 = 一张脚本 = 一张出图（三张图就出三张）', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle
      .mockResolvedValueOnce(hotLayoutJson('ПЕРВЫЙ'))
      .mockResolvedValueOnce(hotLayoutJson('ВТОРОЙ'))
      .mockResolvedValueOnce(hotLayoutJson('ТРЕТИЙ'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper, ['a.png', 'b.png', 'c.png'])
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    expect(aiImageApi.analyzeHotStyle).toHaveBeenCalledTimes(3)
    const panel = wrapper.get('[data-testid="hot-panel"]')
    expect(panel.findAll('[data-testid^="script-card-toggle-"]')).toHaveLength(3)
    expect(panel.text()).toContain('第 1 张 · 复刻参考图 1')
    expect(panel.text()).toContain('第 3 张 · 复刻参考图 3')
  })

  it('拆解时把参考图和「我的商品图」两张一起送出去，顺序是参考图在前', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle.mockResolvedValue(hotLayoutJson('НОВЫЙ'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper, ['ref.png'])
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    const call = aiImageApi.analyzeHotStyle.mock.calls[0][0] as {
      images: File[]
      temperature: number
      timeoutMs: number
    }
    // 两张一起送，顺序固定：第 1 张是版式参考，第 2 张是我们的商品。
    expect(call.images).toHaveLength(2)
    expect(call.images[0].name).toBe('ref.png')
    expect(call.images[1].name).toBe('mine.png')
    expect(call.temperature).toBe(0.2)
    expect(call.timeoutMs).toBe(90000)
  })

  it('可以粘贴图片链接当参考图，链接原样交给视觉模型（浏览器不去下载它）', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle.mockResolvedValue(hotLayoutJson('ПО ССЫЛКЕ'))

    await attachHotProductImage(wrapper)
    await wrapper.get('[data-testid="hot-url-input"]').setValue('https://cdn.example.com/hot/ref-1.jpg')
    await wrapper.get('[data-testid="hot-add-url"]').trigger('click')
    await flushPromises()

    // 列表里能看到这一张（名字取链接尾巴，认得出是哪张）。
    expect(wrapper.get('[data-testid="hot-ref-list"]').text()).toContain('ref-1.jpg')

    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    const call = aiImageApi.analyzeHotStyle.mock.calls[0][0] as { images: Array<File | string> }
    // 关键：链接**原样**传下去，不去 fetch（同行图站基本不给跨域头）。
    expect(call.images[0]).toBe('https://cdn.example.com/hot/ref-1.jpg')
    expect(call.images[1]).toBeInstanceOf(File)
  })

  it('链接格式不对时给出提示，不会加进列表', async () => {
    const wrapper = await openHotPanel()

    await wrapper.get('[data-testid="hot-url-input"]').setValue('ht!tp://bad')
    await wrapper.get('[data-testid="hot-add-url"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('aiImage.hot.badUrl')
    expect(wrapper.find('[data-testid="hot-ref-list"]').exists()).toBe(false)
  })

  it('同一批里文件和链接混着放也可以，数量一起算', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle
      .mockResolvedValueOnce(hotLayoutJson('ФАЙЛ'))
      .mockResolvedValueOnce(hotLayoutJson('ССЫЛКА'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper, ['local.png'])
    await wrapper.get('[data-testid="hot-url-input"]').setValue('https://cdn.example.com/a.png')
    await wrapper.get('[data-testid="hot-add-url"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    // 一张文件 + 一条链接 = 两张脚本。
    expect(wrapper.get('[data-testid="hot-panel"]').findAll('[data-testid^="script-card-toggle-"]')).toHaveLength(2)
  })

  it('卡片里写的是拆解出来的版式和俄语文案，不是套图那套默认脚本', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle.mockResolvedValue(hotLayoutJson('УВАЖАЕМЫЕ ПОКУПАТЕЛИ'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper)
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    await wrapper.get('[data-testid="script-card-toggle-hot_01_ref1"]').trigger('click')
    await flushPromises()
    const prompt = (wrapper.get('[data-testid="script-card-text-hot_01_ref1"]').element as HTMLTextAreaElement).value
    // 拆出来的俄语文案和分区都进脚本了。
    expect(prompt).toContain('УВАЖАЕМЫЕ ПОКУПАТЕЛИ')
    expect(prompt).toContain('顶部')
    expect(prompt).toContain('大标题')
    // 绝不能混进套图的默认脚本（那是主人明确要删掉的）。
    expect(prompt).not.toContain('Ozon 商品主图，3:4 竖版')
    expect(prompt).not.toContain('核心卖点信息图')
    expect(prompt).not.toContain('主视觉钩子')
  })

  it('某一张拆解失败时其余照常出脚本，并给出提示', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle
      .mockRejectedValueOnce(new Error('上游炸了'))
      .mockResolvedValueOnce(hotLayoutJson('ВТОРОЙ'))

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper, ['a.png', 'b.png'])
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    const panel = wrapper.get('[data-testid="hot-panel"]')
    expect(panel.findAll('[data-testid^="script-card-toggle-"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('aiImage.hot.oneFailed')
  })

  it('都没拆出来时提示换几张', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle.mockResolvedValue('抱歉我分析不了')

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper, ['a.png'])
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.text().includes('aiImage.hot.noResult'))

    expect(wrapper.text()).toContain('aiImage.hot.noResult')
    expect(wrapper.find('[data-testid="hot-script-cards"]').exists()).toBe(false)
  })

  /**
   * 主人 2026-10-19 的原话：「不能有套图的脚本，不能有任何的默认脚本」，
   * 外加「产品的名称、卖点、用户痛点、差异化这些不需要，额外生成的几张也不需要」。
   * 所以这些入口一个都不该再存在 —— 这条测试就是防止它们偷偷回来。
   */
  it('爆款复刻里不再有任何默认脚本入口', async () => {
    const wrapper = await openHotPanel()

    for (const id of [
      'hot-product',
      'hot-sp1',
      'hot-sp2',
      'hot-sp3',
      'hot-pain',
      'hot-diff',
      'hot-extras',
      'hot-extract',
      'hot-build-script',
      'hot-template-state',
      'hot-style-desc',
    ]) {
      expect(wrapper.find(`[data-testid="${id}"]`).exists()).toBe(false)
    }
  })

  it('不碰「套图」面板也能在爆款复刻里出脚本（主人反馈的逻辑问题）', async () => {
    const wrapper = await openHotPanel()
    aiImageApi.analyzeHotStyle.mockResolvedValue(hotLayoutJson('ПО МОЕМУ ТОВАРУ'))

    expect(wrapper.find('[data-testid="script-panel"]').exists()).toBe(false)

    await attachHotProductImage(wrapper)
    await pickHotRefs(wrapper)
    await wrapper.get('[data-testid="hot-analyze"]').trigger('click')
    await waitUntil(() => wrapper.find('[data-testid="hot-script-cards"]').exists())

    expect(wrapper.text()).toContain('第 1 张 · 复刻参考图 1')
    // 全程没有打开过套图面板。
    expect(wrapper.find('[data-testid="script-panel"]').exists()).toBe(false)
    // 一键生成全部就在本页脚本下方。
    expect(
      wrapper.get('[data-testid="hot-panel"]').find('[data-testid="generate-all-script-cards"]').exists(),
    ).toBe(true)
  })

  it('爆款复刻页自带「我的商品图」入口，不用切到别的页面去设', async () => {
    const wrapper = await openHotPanel()
    const zone = wrapper.get('[data-testid="hot-product-image"]')
    expect(zone.exists()).toBe(true)
    await attachHotProductImage(wrapper)
    expect(wrapper.text()).toContain('mine.png')
    expect(wrapper.find('[data-testid="single-panel"]').exists()).toBe(false)
  })
})


describe('AI生图 卖点：界面中文、出图俄语', () => {
  beforeEach(() => {
    aiImageApi.extractSellingPoints.mockResolvedValue({
      product: '316 不锈钢保温杯',
      sp1: '24 小时保温',
      sp2: '316 不锈钢',
      sp3: '单手开盖',
      sp1_ru: '24 часа тепло',
      sp2_ru: 'сталь 316',
      sp3_ru: 'крышка одной рукой',
      pain: '水到下午就凉了',
      diff: '别人用 201 钢',
    })
  })

  async function extractWithImage() {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()

    const vm = wrapper.vm as unknown as { onReferencePicked: (e: unknown) => void }
    vm.onReferencePicked({ target: { files: [fakeImageFile('product.png')], value: '' } })
    await flushPromises()

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.aiExtract'))!.trigger('click')
    await flushPromises()
    return wrapper
  }

  it('AI 识别出来的卖点框里显示的是中文', async () => {
    const wrapper = await extractWithImage()

    const inputs = wrapper.findAll('input')
    expect(inputs.some((item) => (item.element as HTMLInputElement).value === '24 小时保温')).toBe(true)
    expect(inputs.some((item) => (item.element as HTMLInputElement).value === '316 不锈钢')).toBe(true)
    // 俄语不该出现在输入框里（那是给图用的）。
    expect(inputs.some((item) => (item.element as HTMLInputElement).value === '24 часа тепло')).toBe(false)
  })

  it('界面上明确列出「图上会印的俄语文案」', async () => {
    const wrapper = await extractWithImage()

    const hint = wrapper.get('[data-testid="printed-ru-hint"]').text()
    expect(hint).toContain('aiImage.script.printedRu')
    expect(hint).toContain('24 часа тепло')
    expect(hint).toContain('сталь 316')
  })

  it('生成脚本时图上用的是俄语，不是界面上的中文', async () => {
    const wrapper = await extractWithImage()

    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('316 不锈钢保温杯')

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    const cards = wrapper.findAll('button[title]')
    const joined = cards.map((item) => item.attributes('title') || '').join('\n')
    expect(joined).toContain('24 часа тепло')
    expect(joined).toContain('сталь 316')
    // 中文卖点不该被印到图上。
    expect(joined).not.toContain('24 小时保温')
  })

  it('手填中文（没有俄语版）时退回用中文，不报错也不丢字', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="toggle-script-panel"]').trigger('click')
    await flushPromises()

    const productInput = wrapper
      .findAll('input')
      .find((item) => item.attributes('placeholder')?.includes('productPlaceholder'))!
    await productInput.setValue('保温杯')
    const spInputs = wrapper.findAll('input').filter((item) => {
      const value = (item.element as HTMLInputElement).value
      return value === ''
    })
    expect(spInputs.length).toBeGreaterThan(0)

    const vm = wrapper.vm as unknown as { scriptForm: Record<string, string> }
    vm.scriptForm.sp1 = '手填的中文卖点'
    await flushPromises()

    await wrapper.findAll('button').find((item) => item.text().includes('aiImage.script.build'))!.trigger('click')
    await flushPromises()

    const joined = wrapper.findAll('button[title]').map((item) => item.attributes('title') || '').join('\n')
    expect(joined).toContain('手填的中文卖点')
  })
})

describe('AI生图 张数自由选择', () => {
  it('张数输入框上限是 10', async () => {
    const wrapper = await withSavedKey()
    const input = wrapper.get('[data-testid="count-input"]')
    expect(input.attributes('max')).toBe('10')
    expect(input.attributes('min')).toBe('1')
  })

  it('一次生成多张时，结果卡片把每一张都显示出来', async () => {
    const wrapper = await withSavedKey()
    aiImageApi.generateAIImage.mockResolvedValue({
      data: [
        { url: 'https://cdn.example/a.png' },
        { url: 'https://cdn.example/b.png' },
        { url: 'https://cdn.example/c.png' },
      ],
    })

    await wrapper.get('[data-testid="prompt-input"]').setValue('出一组图')
    await wrapper.get('[data-testid="count-input"]').setValue(3)
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    // 关键：三张都要看得见，不能只显示第一张。
    expect(wrapper.find('[data-testid="result-image-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="result-image-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="result-image-2"]').exists()).toBe(true)

    const images = wrapper.findAll('img')
    const sources = images.map((item) => item.attributes('src'))
    expect(sources).toContain('https://cdn.example/a.png')
    expect(sources).toContain('https://cdn.example/b.png')
    expect(sources).toContain('https://cdn.example/c.png')
  })

  it('张数超过 10 时按 10 发请求（不发无限张，免得烧额度）', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="prompt-input"]').setValue('出一组图')
    await wrapper.get('[data-testid="count-input"]').setValue(50)
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    expect(aiImageApi.generateAIImage).toHaveBeenCalledTimes(1)
    const request = aiImageApi.generateAIImage.mock.calls[0][0] as { n?: number }
    expect(request.n).toBe(10)
  })
})

describe('AI生图 提示词完整展示', () => {
  it('结果卡片里的提示词不再截断，整段都能看到', async () => {
    const longPrompt = '把这张产品图换成纯白背景，3:4 竖版，加上俄语卖点文案，' + '并且保留商品本身的材质纹理和接口位置，'.repeat(6)
    seedRecords([
      {
        id: 'r-long',
        prompt: longPrompt,
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['https://cdn.example/out.png'],
        error: '',
        createdAt: Date.now(),
      },
    ])

    const wrapper = mountView()
    await flushPromises()

    const shown = wrapper.get('[data-testid="record-prompt"]')
    expect(shown.text()).toBe(longPrompt)
    // 不能再是「截成 2 行」的样式。
    expect(shown.classes().some((item) => item.includes('line-clamp'))).toBe(false)
  })

  it('提示词输入框加高了，写长提示词时能看全', async () => {
    const wrapper = await withSavedKey()
    const textarea = wrapper.get('[data-testid="prompt-input"]')
    expect(Number(textarea.attributes('rows'))).toBeGreaterThanOrEqual(6)
  })
})

describe('AI生图 本轮生成排在前面', () => {
  it('新生成的一轮单独标出来，旧图保留但沉到历史记录里', async () => {
    // 先塞一条「上一轮」的旧记录。
    seedRecords([
      {
        id: 'old-1',
        prompt: '上一轮的图',
        model: 'gpt-image-2',
        size: '1200x1600',
        status: 'completed',
        images: ['https://cdn.example/old.png'],
        error: '',
        createdAt: Date.now() - 100000,
        seq: 1,
        roundId: 1,
      },
    ])

    const wrapper = await withSavedKey()
    expect(wrapper.find('[data-testid="result-group-current"]').exists()).toBe(false)

    // 再生成一条新的（新的一轮）。
    await wrapper.get('[data-testid="prompt-input"]').setValue('这一轮的图')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    // 两段都在：本轮 + 历史。
    expect(wrapper.find('[data-testid="result-group-current"]').text()).toContain('aiImage.result.currentRound')
    expect(wrapper.find('[data-testid="result-group-history"]').text()).toContain('aiImage.result.history')

    // 关键：本轮的图排在旧图前面，而且旧图没被删掉。
    const html = wrapper.html()
    expect(html.indexOf('这一轮的图')).toBeLessThan(html.indexOf('上一轮的图'))
    expect(wrapper.text()).toContain('上一轮的图')
  })

  it('只有一轮时不用显示分组标题', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="prompt-input"]').setValue('唯一一轮')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="result-group-current"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-group-history"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('唯一一轮')
  })
})

describe('AI生图 生图模型下拉', () => {
  // 主人 2026-10-15 给了三个生图模型：gpt-image-2 / gpt-image-2.5-sunburst /
  // gpt-image-2.5-flare。要求做成下拉，不让手打——手打错一个字符，
  // 上游只回一句「模型不存在」，用户看不出是自己打错了。
  it('模型是一个下拉框，选项就是那三个，不是输入框', async () => {
    const wrapper = await withSavedKey()
    const select = wrapper.get('[data-testid="model-select"]')

    expect(select.element.tagName).toBe('SELECT')
    const options = select.findAll('option').map((option) => option.element.getAttribute('value'))
    expect(options).toEqual(['gpt-image-2', 'gpt-image-2.5-sunburst', 'gpt-image-2.5-flare'])
  })

  it('默认选中第一个（gpt-image-2）', async () => {
    const wrapper = await withSavedKey()
    expect((wrapper.get('[data-testid="model-select"]').element as HTMLSelectElement).value).toBe('gpt-image-2')
  })

  it('生成时用的是下拉里选中的那个模型', async () => {
    const wrapper = await withSavedKey()
    await wrapper.get('[data-testid="model-select"]').setValue('gpt-image-2.5-flare')
    await wrapper.get('[data-testid="prompt-input"]').setValue('换背景')
    await generateButton(wrapper).trigger('click')
    await flushPromises()

    const request = aiImageApi.generateAIImage.mock.calls.at(-1)?.[0] as { model?: string }
    expect(request.model).toBe('gpt-image-2.5-flare')
  })

  it('存过的模型名不在白名单里时回退到默认，不让下拉显示空白', async () => {
    // 以前模型是输入框，用户可能存过别的名字（比如 gpt-image-1）。
    window.localStorage.setItem('sub2api_ai_image_model', 'gpt-image-1')
    const wrapper = await withKeyOnly()
    await openSinglePanel(wrapper)

    const select = wrapper.get('[data-testid="model-select"]').element as HTMLSelectElement
    expect(select.value).toBe('gpt-image-2')
  })

  it('存过的模型名在白名单里时照常选中它', async () => {
    window.localStorage.setItem('sub2api_ai_image_model', 'gpt-image-2.5-sunburst')
    const wrapper = await withKeyOnly()
    await openSinglePanel(wrapper)

    const select = wrapper.get('[data-testid="model-select"]').element as HTMLSelectElement
    expect(select.value).toBe('gpt-image-2.5-sunburst')
  })
})
