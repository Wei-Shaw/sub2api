import { defineComponent, h } from 'vue'

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

import IntelligenceCheckView from '@/views/admin/IntelligenceCheckView.vue'
import zhIntelligenceCheck from '@/i18n/locales/zh/admin/intelligenceCheck'

// 用户端（非管理员）智力检测页的渲染契约：卡片第二行必须显示「模型 · 智力等级」——
// 这是用户唯一能看出「这张卡跑的是什么模型、用了什么智力等级」的线索；
// 同时账号 id / 账号名 / 上游标识一个都不许出现。
// 这里用真实中文文案渲染：键写错会直接回显成 key 原文，断言立刻失败，不会静默通过。

const { listPublicRuns, getPublicArtifact, listAccounts, listRuns } = vi.hoisted(() => ({
  listPublicRuns: vi.fn(),
  getPublicArtifact: vi.fn(),
  listAccounts: vi.fn(),
  listRuns: vi.fn()
}))

// 同一页面服务两种身份，身份开关要能在用例间切换（vi.mock 工厂提升，所以放 hoisted 里）。
const authState = vi.hoisted(() => ({ isAdmin: false }))

// vi.mock 的工厂会被提升到文件顶部，里面既不能引用顶层 import，也不能动态 import
// （两者都会变成「Cannot access '__vi_import_N__' before initialization」）。
// 所以用一个 vi.hoisted 出来的桥接对象持有真实文案，等 import 初始化后再注入。
const i18nBridge = vi.hoisted(() => ({ messages: undefined as unknown }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { list: listAccounts },
    intelligenceCheck: {
      listRuns,
      createRun: vi.fn(),
      reviewRun: vi.fn(),
      getArtifact: vi.fn()
    }
  }
}))

vi.mock('@/api/intelligenceCheckPublic', () => ({
  intelligenceCheckPublicAPI: { listPublicRuns, getPublicArtifact }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

// 身份可在用例间切换：默认普通用户（页面只允许调用脱敏公开接口），管理端用例再打开。
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAdmin() {
      return authState.isAdmin
    }
  })
}))

// 布局/弹窗/图标一律用渲染函数桩件（不走运行时模板编译，也不受 vi.mock 提升影响）。
const AppLayoutStub = defineComponent({
  name: 'AppLayout',
  setup: (_props, { slots }) => () => h('main', slots.default?.())
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  setup: (_props, { slots }) => () => h('div', slots.default?.())
})

const IconStub = defineComponent({
  name: 'Icon',
  props: { name: { type: String, required: false } },
  setup: () => () => h('i')
})

vi.mock('vue-i18n', async (importOriginal) => {
  // 只替换 useI18n；createI18n 等真实导出必须保留 —— src/i18n/index.ts 在模块加载时
  // 就要用 createI18n 建实例，缺了它会直接 "No createI18n export is defined on the mock"。
  const actual = await importOriginal<typeof import('vue-i18n')>()

  type Messages = { [key: string]: string | Messages }

  // 与真实 locale 同路径取值 + 极简 {占位符} 替换，够这个页面用。
  const translate = (key: string, params?: Record<string, unknown>) => {
    let node: string | Messages | undefined = i18nBridge.messages as Messages | undefined
    for (const part of key.replace(/^admin\./, '').split('.')) {
      node = typeof node === 'string' ? undefined : node?.[part]
    }
    if (typeof node !== 'string') return key
    if (!params) return node
    return Object.entries(params).reduce(
      (text, [name, value]) => text.replaceAll(`{${name}}`, String(value)),
      node
    )
  }

  return { ...actual, useI18n: () => ({ t: translate }) }
})

// 注入真实中文文案：此时顶层 import 已初始化，键写错会回显成 key 原文，断言立刻失败。
// 注意这里注入的是整个模块（含 intelligenceCheck 这一层），与 translate 里「剥掉 admin. 前缀」
// 的取值方式对齐；少这一层就会一路取不到值、静默退化成回显 key。
i18nBridge.messages = zhIntelligenceCheck

// 一条公开卡片：模型与等级必须露出来；account_id / 账号名 / 上游标识后端根本不会给。
const publicCard = {
  index: 1,
  status: 'completed',
  verdict: 'unknown',
  latency_ms: 242100,
  has_artifact: true,
  artifact_url: '/api/v1/intelligence-check/runs/11/artifact',
  created_at: '2026-10-01T09:00:00Z',
  model_id: 'glm-5.3-flashx',
  reasoning_effort: 'xhigh'
}

async function mountView() {
  const wrapper = mount(IntelligenceCheckView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: BaseDialogStub,
        Icon: IconStub
      }
    }
  })
  await flushPromises()
  return wrapper
}

async function mountWall(items: unknown[]) {
  listPublicRuns.mockResolvedValue({ items, total: items.length })
  return mountView()
}

describe('用户端智力检测作品墙', () => {
  beforeEach(() => {
    authState.isAdmin = false
    listPublicRuns.mockReset()
    getPublicArtifact.mockReset()
    getPublicArtifact.mockResolvedValue('')
  })

  it('卡片第二行显示模型名与智力等级', async () => {
    const wrapper = await mountWall([publicCard])
    const text = wrapper.text()

    expect(text).toContain('账号 1')
    expect(text).toContain('glm-5.3-flashx')
    expect(text).toContain('智力等级 xhigh')
    // 文案真实存在（不是回显的 key 原文），且不带管理员那行的 #账号id 前缀。
    expect(text).not.toContain('admin.intelligenceCheck')
    expect(text).not.toContain('#')

    wrapper.unmount()
  })

  it('记录里没有模型信息时整行不渲染，也不编造「未记录模型」', async () => {
    const wrapper = await mountWall([
      { ...publicCard, model_id: undefined, reasoning_effort: undefined }
    ])
    const text = wrapper.text()

    expect(text).toContain('账号 1')
    expect(text).not.toContain('智力等级')
    expect(text).not.toContain('未记录模型')

    wrapper.unmount()
  })

  it('只有模型、没有智力等级时只显示模型名', async () => {
    const wrapper = await mountWall([{ ...publicCard, reasoning_effort: undefined }])
    const text = wrapper.text()

    expect(text).toContain('glm-5.3-flashx')
    expect(text).not.toContain('智力等级')

    wrapper.unmount()
  })
})

// 同一个 .vue 文件服务两种身份：改用户端那一行，必须证明管理端那行没被带坏。
describe('管理端智力检测作品墙（回归护栏）', () => {
  const adminAccount = {
    id: 37,
    name: 'openai-账号1',
    extra: { intelligence_check_enabled: true }
  }
  const adminRun = {
    id: 11,
    account_id: 37,
    status: 'completed',
    verdict: 'unknown',
    model_id: 'glm-5.3-flashx',
    reasoning_effort: 'xhigh',
    has_html: true,
    latency_ms: 242100,
    created_at: '2026-10-01T09:00:00Z',
    reviewed_at: null
  }

  beforeEach(() => {
    authState.isAdmin = true
    listAccounts.mockReset()
    listRuns.mockReset()
    listAccounts.mockResolvedValue({ items: [adminAccount], total: 1, page: 1, page_size: 200, pages: 1 })
    listRuns.mockResolvedValue({ items: [adminRun], total: 1, page: 1, page_size: 20, pages: 1 })
  })

  afterEach(() => {
    authState.isAdmin = false
  })

  it('仍是「#账号id · 模型 · 智力等级」，且显示账号名而不是「账号 N」', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('#37 · glm-5.3-flashx · xhigh')
    expect(text).toContain('openai-账号1')
    // 管理端不走公开的「账号 N」匿名标签，也不该出现用户端那套文案键。
    expect(text).not.toContain('账号 1')
    expect(text).not.toContain('admin.intelligenceCheck')

    wrapper.unmount()
  })
})
