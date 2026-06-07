import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// `useClipboard` is the real composable in src/composables; we mock it
// here to (a) avoid touching navigator.clipboard / document.execCommand
// in jsdom and (b) be able to assert the exact value passed in.
const copyToClipboard = vi.fn()
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: { value: false },
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import HomeContactSection from '../HomeContactSection.vue'

function mountSection() {
  setActivePinia(createPinia())
  return mount(HomeContactSection)
}

describe('HomeContactSection', () => {
  beforeEach(() => {
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)
  })

  it('渲染三个联系卡片：QQ 客服 / QQ 群 / Telegram', () => {
    // 行为契约：首页底部"联系我们"分区必须呈现这三条独立通道。
    // 任何一张卡片缺失都说明整段被误隐藏或意外重构 —— 守门测试。
    const wrapper = mountSection()
    expect(wrapper.find('[data-test="home-contact-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="contact-card-qq"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="contact-card-qq-group"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="contact-card-telegram"]').exists()).toBe(true)
  })

  it('展示硬编码的 QQ 号、群链接、Telegram 链接 与 二维码图片地址', () => {
    // 这些值来自运营方提供的真实地址，目前是硬编码（理由见组件
    // 头部注释：抽到 admin settings 工作量过大、变更频率极低）。
    // 测试钉死这些字面量，使得未来一旦有人改动配置，会强制他们
    // 同步更新本断言（防止误改 / 误删）。
    const wrapper = mountSection()

    expect(wrapper.find('[data-test="contact-qq-number"]').text()).toBe('1161431181')

    const groupLink = wrapper.find('[data-test="contact-qq-group-link"]')
    expect(groupLink.attributes('href')).toBe('https://qm.qq.com/q/wZ14xvlU66')
    expect(groupLink.attributes('target')).toBe('_blank')
    // rel=noopener noreferrer 是出站外链的安全基线（防止 window.opener
    // 反向劫持 + 防止 referrer 泄露），别在重构里被顺手删掉。
    expect(groupLink.attributes('rel')).toBe('noopener noreferrer')

    const tgLink = wrapper.find('[data-test="contact-telegram-link"]')
    expect(tgLink.attributes('href')).toBe('https://t.me/opentks')
    expect(tgLink.attributes('target')).toBe('_blank')
    expect(tgLink.attributes('rel')).toBe('noopener noreferrer')

    const qqGroupQr = wrapper.find('[data-test="contact-qq-group-qr"]')
    expect(qqGroupQr.attributes('src')).toBe(
      'https://17wanai-1251015133.cos.ap-guangzhou.myqcloud.com/opentoken_qqun.jpg',
    )
    // 二维码位于折叠下方分区，强制 lazy + async 解码以保护首屏 LCP
    // —— 如果谁去掉了，等于让首屏被两张外链图片拖慢，测试拦截。
    expect(qqGroupQr.attributes('loading')).toBe('lazy')
    expect(qqGroupQr.attributes('decoding')).toBe('async')

    const tgQr = wrapper.find('[data-test="contact-telegram-qr"]')
    expect(tgQr.attributes('src')).toBe(
      'https://17wanai-1251015133.cos.ap-guangzhou.myqcloud.com/opentoken_telegram.png',
    )
    expect(tgQr.attributes('loading')).toBe('lazy')
    expect(tgQr.attributes('decoding')).toBe('async')
  })

  it('点击复制按钮调用 useClipboard 并把 QQ 号传给剪贴板', async () => {
    // 复制流程的契约：确保按下按钮真的触发了剪贴板写入，且写入的是
    // QQ 号本体（不是带前缀 / 带空格的派生串）。这是用户能否一键加好友
    // 的核心路径，必须有断言守住。
    const wrapper = mountSection()
    await wrapper.find('[data-test="contact-qq-copy"]').trigger('click')
    await flushPromises()
    expect(copyToClipboard).toHaveBeenCalledTimes(1)
    expect(copyToClipboard).toHaveBeenCalledWith('1161431181', expect.any(String))
  })

  it('复制成功后按钮内出现 "已复制" 文案（in-place 反馈），失败则不出现', async () => {
    // 行为契约：成功时除全局 toast 之外还需要 in-place 文案反馈，避免
    // 用户视线移开按钮去看 toast；失败时不能展示 "已复制"，否则会
    // 误导用户以为剪贴板已写入而实际并未发生。
    //
    // 注意：footer 形态下按钮默认不显示 "复制 QQ 号" 字样（号码本身
    // 加 copy 图标已经表达了语义），所以断言不再做"两种字样翻转"，
    // 而是单向检查 "已复制" 文案的出现/缺席。
    copyToClipboard.mockResolvedValueOnce(true)
    const wrapper = mountSection()
    const btn = wrapper.find('[data-test="contact-card-qq"]')
    expect(btn.text()).not.toContain('home.contact.qq.copied')
    await btn.trigger('click')
    await flushPromises()
    expect(btn.text()).toContain('home.contact.qq.copied')

    // 重新挂载验证失败路径独立 —— 用现有 wrapper 会受 2s 计时器影响。
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValueOnce(false)
    const wrapper2 = mountSection()
    const btn2 = wrapper2.find('[data-test="contact-card-qq"]')
    await btn2.trigger('click')
    await flushPromises()
    expect(btn2.text()).not.toContain('home.contact.qq.copied')
  })
})
