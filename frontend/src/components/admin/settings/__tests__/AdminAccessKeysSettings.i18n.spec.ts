import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import AdminAccessKeysSettings from '../AdminAccessKeysSettings.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import enKeys from '@/i18n/locales/en/adminAccessKeys'
import zhKeys from '@/i18n/locales/zh/adminAccessKeys'

const catalogSource = readFileSync(resolve(process.cwd(), '../backend/internal/pkg/adminaccess/catalog.go'), 'utf8')
const catalog = [...catalogSource.matchAll(/Permission\{"([^"]+)", "([^"]+)", "([^"]+)", "([^"]+)"\}/g)].map(([, id, label, category, scope]) => ({ id, label, category, scope }))
const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), rotate: vi.fn(), revoke: vi.fn(), deleteRecord: vi.fn() }))
vi.mock('@/api/admin/adminAccessKeys', () => mocks)
vi.mock('@/api/admin/groups', () => ({ getAllIncludingInactive: vi.fn().mockResolvedValue([{ id: 7, name: 'Example group' }]) }))
vi.mock('@/components/auth/TotpStepUpDialog.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: (fn: () => unknown) => fn() }), isStepUpCancelled: () => false }))
const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))
beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue({ items: [], permissions: [...catalog, { id: 'future.feature', label: '后端新增权限', category: '后端新增分类', scope: 'global' }, { id: 'payment.mark.paid', label: '不可误认成人工确认到账', category: '未知分类', scope: 'global' }] })
  mocks.create.mockResolvedValue({ key: 'secret', item: { id: 'created', name: 'Example key' } })
})
function setup(locale: 'en' | 'zh' = 'en') {
  const i18n = createI18n({ legacy: false, locale, fallbackLocale: 'en', messages: { en, zh } })
  const wrapper = mount(AdminAccessKeysSettings, { global: { plugins: [i18n], stubs: { teleport: true } } })
  wrappers.push(wrapper)
  return { wrapper, i18n }
}

describe('管理员密钥英文体验', () => {
  it('各自后端目录的每个权限都有准确的中英文标签与分类', () => {
    expect(catalog.length).toBeGreaterThan(50)
    expect(new Set(catalog.map(permission => permission.id)).size).toBe(catalog.length)
    const expectedKeys = catalog.map(permission => permission.id.replaceAll('_', '_u').replaceAll('.', '_d')).sort()
    for (const messages of [enKeys, zhKeys]) {
      expect(Object.keys(messages.permissions).sort()).toEqual(expectedKeys)
      for (const { label, category } of Object.values(messages.permissions)) {
        expect(label.trim()).not.toBe('')
        expect(category.trim()).not.toBe('')
      }
    }
    expect(en.adminAccessKeys).toEqual(enKeys)
    expect(zh.adminAccessKeys).toEqual(zhKeys)
    for (const permission of catalog) {
      const key = permission.id.replaceAll('_', '_u').replaceAll('.', '_d') as keyof typeof zhKeys.permissions
      expect(zhKeys.permissions[key]).toEqual({ label: permission.label, category: permission.category })
      expect(enKeys.permissions[key].label).not.toMatch(/[\u3400-\u9fff]/u)
      expect(enKeys.permissions[key].category).not.toMatch(/[\u3400-\u9fff]/u)
    }
  })

  it('切换语言清除待确认文字和本次扩权，必须使用当前语言重新确认', async () => {
    const { wrapper, i18n } = setup('zh')
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('Example key')
    await wrapper.get('[data-test="full-access"]').setValue(true)
    await wrapper.get('[data-test="full-access-phrase"]').setValue('开启全权访问')
    i18n.global.locale.value = 'en'
    await flushPromises()
    expect(wrapper.find('[data-test="full-access-phrase"]').exists()).toBe(false)
    expect(wrapper.get<HTMLInputElement>('[data-test="full-access"]').element.checked).toBe(false)
    expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/u)
    await wrapper.get('[data-test="full-access"]').setValue(true)
    expect(wrapper.get<HTMLInputElement>('[data-test="full-access-phrase"]').element.value).toBe('')
    await wrapper.get('[data-test="full-access-phrase"]').setValue('ENABLE FULL ACCESS')
    await wrapper.get('[data-test="full-access-confirm"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-test="full-access"]').element.checked).toBe(true)
    i18n.global.locale.value = 'zh'
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('[data-test="full-access"]').element.checked).toBe(false)
    await wrapper.get('[data-test="full-access"]').setValue(true)
    await wrapper.get('[data-test="full-access-phrase"]').setValue('ENABLE FULL ACCESS')
    expect(wrapper.get('[data-test="full-access-confirm"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="full-access-phrase"]').setValue('开启全权访问')
    await wrapper.get('[data-test="full-access-confirm"]').trigger('click')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ policy: expect.objectContaining({ full_access: true }), confirm_full_access: true }))
  })

  it('语言切换保留已有全权权限并翻译列表摘要', async () => {
    mocks.list.mockResolvedValueOnce({ items: [{ id: 'existing', name: 'Existing key', enabled: true, allowed_ips: [], policy: { full_access: true, rules: [{ permissions: ['accounts.read'], group_ids: [7], account_types: ['oauth'], platforms: ['openai'] }], global_permissions: [] } }], permissions: catalog })
    const { wrapper, i18n } = setup('zh')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
    i18n.global.locale.value = 'en'
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('[data-test="full-access"]').element.checked).toBe(true)
    expect(wrapper.text()).toContain('Full access')
    expect(wrapper.text()).toContain('Example group')
    expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/u)
  })

  it('英文环境翻译页面及后端权限目录，未知权限不泄漏中文标签或分类', async () => {
    const { wrapper } = setup()
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    expect(wrapper.text()).toContain('Admin access keys')
    expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/u)
    expect(wrapper.text()).toContain('Read accounts (redacted)')
    expect(wrapper.text()).toContain('Finance')
    expect(wrapper.text()).toContain('future.feature')
    const unknown = wrapper.get('input[value="payment.mark.paid"]').element.closest('label')!
    expect(unknown.textContent).toContain('Unknown permission (payment.mark.paid)')
    for (const permission of catalog) {
      const key = permission.id.replaceAll('_', '_u').replaceAll('.', '_d') as keyof typeof enKeys.permissions
      expect(wrapper.get(`input[value="${permission.id}"]`).element.closest('label')!.textContent).toContain(enKeys.permissions[key].label)
    }
    const creation = wrapper.find('input[value="accounts.create"]')
    await creation.setValue(true)
    expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/u)
    await wrapper.get('[data-test="full-access"]').setValue(true)
    expect(wrapper.text()).toContain('ENABLE FULL ACCESS')
    expect(wrapper.text()).not.toMatch(/[\u3400-\u9fff]/u)
    await wrapper.get('[data-test="full-access-phrase"]').setValue('开启全权访问')
    expect(wrapper.get('[data-test="full-access-confirm"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="full-access-phrase"]').setValue('ENABLE FULL ACCESS')
    await wrapper.get('[data-test="full-access-confirm"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('Example key')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ confirm_full_access: true, policy: expect.objectContaining({ full_access: true }) }))
    expect(wrapper.get('[role="status"]').text()).toBe('Key saved')
  })
})
