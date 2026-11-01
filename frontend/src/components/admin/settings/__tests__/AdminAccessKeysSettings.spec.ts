import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AdminAccessKeysSettings from '../AdminAccessKeysSettings.vue'
import zh from '@/i18n/locales/zh/adminAccessKeys'
import en from '@/i18n/locales/en/adminAccessKeys'
const mountSettings = () => mount(AdminAccessKeysSettings)
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, values: Record<string, unknown> = {}) => {
    const message = zh[key.replace('adminAccessKeys.', '') as keyof typeof zh]
    if (!message) throw new Error(`缺少翻译：${key}`)
    return message.replace(/\{(\w+)\}/g, (_, name: string) => String(values[name]))
  }
}) }))
import { getAllIncludingInactive } from '@/api/admin/groups'
const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), rotate: vi.fn(), revoke: vi.fn(), deleteRecord: vi.fn(), stepUp: vi.fn((fn: () => unknown) => fn()) }))
vi.mock('@/api/admin/adminAccessKeys', () => mocks)
vi.mock('@/api/admin/groups', () => ({ getAllIncludingInactive: vi.fn().mockResolvedValue([{ id: 7, name: '测试分组' }]) }))
vi.mock('@/components/auth/TotpStepUpDialog.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: mocks.stepUp }), isStepUpCancelled: () => false }))
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getAllIncludingInactive).mockResolvedValue([{ id: 7, name: '测试分组' }] as Awaited<ReturnType<typeof getAllIncludingInactive>>)
  mocks.list.mockResolvedValue({ items: [], permissions: [{ id: 'accounts.read', label: '读取账户', scope: 'account', category: '账户' }, { id: 'settings.write', label: '修改设置', scope: 'global', category: '设置' }] })
})
describe('管理员受限密钥配置', () => {
  it('中英文文案键保持一致，模型限制提示采用上游白名单语义', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
    expect(en.modelAllowlistHint).toContain('restricts')
    expect(zh.modelAllowlistHint).toContain('白名单')
  })
  it('加载失败时不允许创建密钥', async () => {
    mocks.list.mockRejectedValueOnce(new Error('加载失败'))
    const wrapper = mountSettings()
    await flushPromises()
    expect(wrapper.get('[data-test="create"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="alert"]').text()).toBe('加载失败')
  })

  it('分组模型权限独立勾选并保留所选分组，不隐式授予账户或全站权限', async () => {
    mocks.list.mockResolvedValueOnce({ items: [], permissions: [
      { id: 'accounts.models.write', label: '账户模型', scope: 'account', category: '账户' },
      { id: 'groups.models.read', label: '读取分组模型', scope: 'group', category: '分组模型' },
      { id: 'groups.models.write', label: '编辑分组模型', scope: 'group', category: '分组模型' }
    ] })
    mocks.create.mockResolvedValue({ key: 'once-secret', item: { id: '1' } })
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('分组模型工具')
    await wrapper.get('[data-test="groups-0"]').setValue(['7'])
    expect(wrapper.text()).toContain('分组模型白名单会限制分组可调用的模型')
    await wrapper.get('input[value="groups.models.read"]').setValue(true)
    await wrapper.get('input[value="groups.models.write"]').setValue(true)
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    const input = mocks.create.mock.calls[0]![0]
    expect(input.policy.rules[0].permissions).toEqual(['groups.models.read', 'groups.models.write'])
    expect(input.policy.rules[0].group_ids).toEqual([7])
    expect(input.policy.rules[0].account_types).toEqual([])
    expect(input.policy.global_permissions).toEqual([])
  })
  it('分组选择不会隐式授予操作或全局权限，并支持多个规则', async () => {
    mocks.create.mockResolvedValue({ key: 'once-secret', item: { id: '1' } })
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('自动化')
    await wrapper.get('[data-test="groups-0"]').setValue(['7'])
    await wrapper.get('[data-test="add-rule"]').trigger('click')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    const input = mocks.create.mock.calls[0]![0]
    expect(input.policy.global_permissions).toEqual([])
    expect(input.policy.rules).toHaveLength(2)
    expect(input.policy.rules[0].permissions).toEqual([])
    expect(input.policy.rules[0].account_types).toEqual([])
    expect(input.policy.rules[0].group_ids).toEqual([7])
    expect(mocks.stepUp).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('once-secret')
    await wrapper.get('[data-test="dismiss-secret"]').trigger('click')
    expect(wrapper.text()).not.toContain('once-secret')
  })
  it('保存失败时保留表单并展示错误，不显示成功密钥', async () => {
    mocks.create.mockRejectedValue(new Error('服务不可用'))
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('自动化')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('服务不可用')
    expect(wrapper.find('[data-test="save"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="dismiss-secret"]').exists()).toBe(false)
  })
  it('停用失败保持原状态，成功后更新状态；轮换及撤销均等待服务器确认', async () => {
    const item = { id: 'key-1', name: '自动化', description: '', enabled: true, key_prefix: 'adm_sc_1', expires_at: null, allowed_ips: [], policy: { rules: [], global_permissions: [] }, revoked_at: null }
    mocks.list.mockResolvedValueOnce({ items: [item], permissions: [] })
    mocks.update.mockRejectedValueOnce(new Error('更新失败'))
    const wrapper = mountSettings()
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === '删除记录')).toBe(false)
    const click = async (text: string) => {
      await wrapper.findAll('button').find(button => button.text() === text)!.trigger('click')
      await flushPromises()
    }
    await click('停用')
    expect(wrapper.text()).toContain('已启用')
    expect(wrapper.text()).toContain('更新失败')
    mocks.update.mockResolvedValueOnce({ ...item, enabled: false })
    await click('停用')
    expect(wrapper.text()).toContain('已停用')
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mocks.rotate.mockResolvedValueOnce({ item: { ...item, enabled: false }, key: 'rotated-once' })
    await click('轮换')
    expect(mocks.rotate).toHaveBeenCalledWith('key-1')
    expect(wrapper.text()).toContain('rotated-once')
    mocks.revoke.mockRejectedValueOnce(new Error('撤销失败'))
    await click('永久撤销')
    expect(wrapper.text()).toContain('自动化')
    expect(wrapper.text()).toContain('撤销失败')
    mocks.revoke.mockResolvedValueOnce({ message: 'ok' })
    await click('永久撤销')
    expect(wrapper.text()).toContain('adm_sc_1')
    expect(wrapper.text()).toContain('已撤销')
    expect(wrapper.findAll('button').some(button => button.text() === '删除记录')).toBe(true)
    expect(mocks.stepUp).toHaveBeenCalledTimes(5)
    vi.restoreAllMocks()
  })

  it('删除已撤销记录时提示历史保留，取消和失败不移除，服务器确认后才移除', async () => {
    const item = { id: 'revoked-key', name: '已撤销自动化', enabled: false, key_prefix: 'revoked-prefix', policy: { rules: [], global_permissions: [] }, revoked_at: '2026-10-22T00:00:00Z' }
    mocks.list.mockResolvedValueOnce({ items: [item], permissions: [] })
    const wrapper = mountSettings()
    await flushPromises()
    const remove = () => wrapper.findAll('button').find(button => button.text() === '删除记录')!
    expect(remove()).toBeDefined()
    expect(wrapper.findAll('button').some(button => button.text() === '永久撤销')).toBe(false)
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    try {
      await remove().trigger('click')
      expect(confirm).toHaveBeenCalledWith(expect.stringContaining('删除只是从列表移除，历史操作记录仍会继续保留'))
      expect(mocks.deleteRecord).not.toHaveBeenCalled()
      expect(wrapper.text()).toContain('revoked-prefix')

      confirm.mockReturnValue(true)
      mocks.deleteRecord.mockRejectedValueOnce(new Error('删除失败'))
      await remove().trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('删除失败')
      expect(wrapper.text()).toContain('revoked-prefix')

      let resolve!: (value: { message: string }) => void
      mocks.deleteRecord.mockReturnValueOnce(new Promise(r => { resolve = r }))
      await remove().trigger('click')
      expect(remove().attributes('disabled')).toBeDefined()
      expect(wrapper.text()).toContain('revoked-prefix')
      resolve({ message: 'ok' })
      await flushPromises()
      expect(mocks.deleteRecord).toHaveBeenLastCalledWith(item.id)
      expect(mocks.stepUp).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).not.toContain('revoked-prefix')
      expect(wrapper.text()).toContain('历史操作记录仍会继续保留')
    } finally {
      confirm.mockRestore()
      wrapper.unmount()
    }
  })

  it('编辑仅修改名称时保留所有规则且不会自动授予操作或全站权限', async () => {
    const rules = [
      { permissions: ['accounts.read'], group_ids: [7], all_groups: false, include_ungrouped: false, account_types: ['oauth'], platforms: ['openai'] },
      { permissions: [], group_ids: [8], all_groups: false, include_ungrouped: true, account_types: [], platforms: [] }
    ]
    const item = { id: 'key-2', name: '原名称', description: '', enabled: true, key_prefix: 'prefix', expires_at: null, allowed_ips: null, policy: { rules, global_permissions: null }, revoked_at: null }
    mocks.list.mockResolvedValueOnce({ items: [item], permissions: [] })
    mocks.update.mockResolvedValueOnce(item)
    const wrapper = mountSettings()
    await flushPromises()
    expect(wrapper.text()).toContain('测试分组')
    expect(wrapper.text()).toContain('全站权限：0 项')
    expect(wrapper.text()).toContain('按操作和资源范围授权；未勾选操作不生效')
    await wrapper.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
    await wrapper.get('[data-test="name"]').setValue('新名称')
    expect(wrapper.text()).not.toContain('kiro')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith('key-2', expect.objectContaining({ name: '新名称', allowed_ips: [], policy: { rules, global_permissions: [] } }))
  })
  it('空数组返回 null 时仍可创建默认无权限密钥', async () => {
    mocks.list.mockResolvedValueOnce({ items: null, permissions: null })
    mocks.create.mockResolvedValueOnce({ key: 'new-secret', item: { id: 'key-3', policy: { rules: null, global_permissions: null } } })
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('无权限')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(mocks.create.mock.calls[0]![0].policy.global_permissions).toEqual([])
    expect(wrapper.text()).toContain('new-secret')
  })

  it.each(['2025-01-02T03:04:56.123456Z', '2099-01-02T03:04:56.123456Z'])('只改名称时精确保留到期时间 %s', async (expiresAt) => {
    const item = { id: 'expiry-key', name: '原名称', description: '', enabled: true, key_prefix: 'prefix', expires_at: expiresAt, allowed_ips: [], policy: { rules: [], global_permissions: [] }, revoked_at: null }
    mocks.list.mockResolvedValueOnce({ items: [item], permissions: [] })
    mocks.update.mockResolvedValueOnce(item)
    const wrapper = mountSettings()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
    await wrapper.get('[data-test="name"]').setValue('新名称')
    await wrapper.get('[data-test="save"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith('expiry-key', expect.objectContaining({ name: '新名称', expires_at: expiresAt }))
    wrapper.unmount()
  })

  it('允许主动修改和清空有效期，重试保持修改值，新建不会继承原有效期', async () => {
    const item = { id: 'expiry-edit', name: '原名称', description: '', enabled: true, key_prefix: 'prefix', expires_at: '2025-01-02T03:04:56.123456Z', allowed_ips: [], policy: { rules: [], global_permissions: [] }, revoked_at: null }
    mocks.list.mockResolvedValueOnce({ items: [item], permissions: [] })
    const wrapper = mountSettings()
    await flushPromises()
    const edit = async () => { await wrapper.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click') }
    const save = async () => { await wrapper.get('[data-test="save"]').trigger('click'); await flushPromises() }
    await edit()
    const local = '2099-02-03T04:05'
    await wrapper.get('input[type="datetime-local"]').setValue(local)
    mocks.update.mockRejectedValueOnce(new Error('重试'))
    await save()
    const expected = new Date(local).toISOString()
    expect(mocks.update).toHaveBeenLastCalledWith(item.id, expect.objectContaining({ expires_at: expected }))
    mocks.update.mockResolvedValueOnce({ ...item, expires_at: expected })
    await save()
    expect(mocks.update).toHaveBeenLastCalledWith(item.id, expect.objectContaining({ expires_at: expected }))
    await edit()
    await wrapper.get('input[type="datetime-local"]').setValue('')
    mocks.update.mockResolvedValueOnce({ ...item, expires_at: null })
    await save()
    expect(mocks.update).toHaveBeenLastCalledWith(item.id, expect.objectContaining({ expires_at: null }))
    await wrapper.get('[data-test="create"]').trigger('click')
    await wrapper.get('[data-test="name"]').setValue('新建')
    await wrapper.get('input[type="datetime-local"]').setValue(local)
    mocks.create.mockResolvedValueOnce({ key: 'new-secret', item: { ...item, id: 'new', expires_at: expected } })
    await save()
    expect(mocks.create).toHaveBeenLastCalledWith(expect.objectContaining({ expires_at: expected }))
    wrapper.unmount()
  })

})
