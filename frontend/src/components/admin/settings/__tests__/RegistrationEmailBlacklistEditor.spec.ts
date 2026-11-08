import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RegistrationEmailBlacklistEditor from '../RegistrationEmailBlacklistEditor.vue'

const mocks = vi.hoisted(() => ({
  getBlacklist: vi.fn(),
  updateBlacklist: vi.fn(),
  findUsers: vi.fn(),
  batchDisable: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getRegistrationEmailBlacklist: mocks.getBlacklist,
      updateRegistrationEmailBlacklist: mocks.updateBlacklist
    },
    users: {
      findBlacklistedEmailUsers: mocks.findUsers,
      batchDisable: mocks.batchDisable
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
    locale: { value: 'en' }
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showSuccess: mocks.showSuccess,
    showError: mocks.showError,
    showInfo: mocks.showInfo
  })
}))

const wrappers: ReturnType<typeof mount>[] = []

function mountEditor() {
  const wrapper = mount(RegistrationEmailBlacklistEditor, {
    attachTo: document.body,
    global: {
      stubs: { teleport: true }
    }
  })
  wrappers.push(wrapper)
  return wrapper
}

afterEach(() => wrappers.splice(0).forEach((wrapper) => wrapper.unmount()))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getBlacklist.mockResolvedValue({ emails: ['old@gmail.com'], max: 20000 })
  mocks.findUsers.mockResolvedValue({ items: [], total: 0 })
  mocks.batchDisable.mockResolvedValue({ affected: 0 })
})

describe('RegistrationEmailBlacklistEditor', () => {
  it('批量添加后保存整份名单，并对新增项命中的用户弹窗确认禁用', async () => {
    mocks.updateBlacklist.mockResolvedValue({
      emails: ['new.one@gmail.com', 'new2@googlemail.com', 'old@gmail.com'],
      added: ['new.one@gmail.com', 'new2@googlemail.com'],
      max: 20000
    })
    mocks.findUsers.mockResolvedValue({
      items: [{ id: 11, email: 'newone@gmail.com', username: 'n1' }],
      total: 1
    })
    mocks.batchDisable.mockResolvedValue({ affected: 1 })

    const wrapper = mountEditor()
    await flushPromises()
    expect(wrapper.get('[data-testid="email-blacklist-summary"]').text()).toContain('"count":"1"')

    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-bulk-input"]').setValue(
      'New.One@gmail.com, new2@googlemail.com\nold@gmail.com\nnew.one@gmail.com\nnot-an-email'
    )
    await wrapper.get('[data-testid="email-blacklist-bulk-add"]').trigger('click')

    expect(wrapper.get('[data-testid="email-blacklist-add-result"]').text()).toContain('"added":"2","duplicates":"2"')
    expect(wrapper.get('[data-testid="email-blacklist-invalid"]').text()).toContain('not-an-email')
    expect(wrapper.findAll('[data-testid="email-blacklist-row"]')).toHaveLength(3)
    expect(
      (wrapper.get('[data-testid="email-blacklist-bulk-input"]').element as HTMLTextAreaElement).value
    ).toBe('not-an-email')

    await wrapper.get('[data-testid="email-blacklist-save"]').trigger('click')
    await flushPromises()

    expect(mocks.updateBlacklist).toHaveBeenCalledWith(['new.one@gmail.com', 'new2@googlemail.com', 'old@gmail.com'])
    expect(mocks.findUsers).toHaveBeenCalledWith(['new.one@gmail.com', 'new2@googlemail.com'])
    expect(wrapper.get('[data-testid="email-blacklist-matched-users"]').text()).toContain('newone@gmail.com')

    await wrapper.get('[data-testid="email-blacklist-disable-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.batchDisable).toHaveBeenCalledWith([11])
    expect(mocks.showSuccess).toHaveBeenLastCalledWith(
      'admin.settings.registration.emailBlacklist.disabled:{"count":"1"}'
    )
    expect(wrapper.find('[data-testid="email-blacklist-matched-users"]').exists()).toBe(false)
  })

  it('保存后没有新增项时不查询用户，没有命中时也不弹窗', async () => {
    mocks.updateBlacklist.mockResolvedValue({ emails: [], added: [], max: 20000 })

    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-clear"]').trigger('click')
    expect(wrapper.get('[data-testid="email-blacklist-pending"]').text()).toContain('"removed":"1"')

    await wrapper.get('[data-testid="email-blacklist-save"]').trigger('click')
    await flushPromises()
    expect(mocks.updateBlacklist).toHaveBeenCalledWith([])
    expect(mocks.findUsers).not.toHaveBeenCalled()
    expect(mocks.showInfo).not.toHaveBeenCalled()
  })

  it('手动检查已注册用户覆盖整份已保存名单，无命中时提示', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-check"]').trigger('click')
    await flushPromises()

    expect(mocks.findUsers).toHaveBeenCalledWith(['old@gmail.com'])
    expect(mocks.showInfo).toHaveBeenCalledWith('admin.settings.registration.emailBlacklist.noActiveMatches')
    expect(wrapper.find('[data-testid="email-blacklist-matched-users"]').exists()).toBe(false)
  })

  it('未改动时保存按钮不可用，保存失败时保留弹窗并提示错误', async () => {
    mocks.updateBlacklist.mockRejectedValue({ message: 'invalid email address (1): broken' })

    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    expect(wrapper.get('[data-testid="email-blacklist-save"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="email-blacklist-bulk-input"]').setValue('fresh@gmail.com')
    await wrapper.get('[data-testid="email-blacklist-bulk-add"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-save"]').trigger('click')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('invalid email address (1): broken')
    expect(wrapper.find('[data-testid="email-blacklist-save"]').exists()).toBe(true)
    expect(mocks.findUsers).not.toHaveBeenCalled()
  })

  it('有未保存改动时关闭先确认，放弃后草稿丢弃，继续编辑则保留', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-bulk-input"]').setValue('draft@gmail.com')
    await wrapper.get('[data-testid="email-blacklist-bulk-add"]').trigger('click')

    const findButton = (label: string) => wrapper.findAll('button').find((button) => button.text() === label)
    await findButton('common.cancel')!.trigger('click')
    expect(wrapper.text()).toContain('admin.settings.registration.emailBlacklist.discardTitle')

    await findButton('admin.settings.registration.emailBlacklist.discardCancel')!.trigger('click')
    expect(wrapper.findAll('[data-testid="email-blacklist-row"]')).toHaveLength(2)

    await findButton('common.cancel')!.trigger('click')
    await findButton('admin.settings.registration.emailBlacklist.discardConfirm')!.trigger('click')
    expect(wrapper.find('[data-testid="email-blacklist-bulk-input"]').exists()).toBe(false)
    expect(mocks.updateBlacklist).not.toHaveBeenCalled()
  })

  it('保存后核查新增项期间编辑入口不可用，核查结束后可正常打开和关闭', async () => {
    let resolveFind: (value: { items: never[]; total: number }) => void = () => {}
    mocks.findUsers.mockReturnValue(new Promise((resolve) => { resolveFind = resolve }))
    mocks.updateBlacklist.mockResolvedValue({ emails: ['x@gmail.com', 'old@gmail.com'], added: ['x@gmail.com'], max: 20000 })

    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-bulk-input"]').setValue('x@gmail.com')
    await wrapper.get('[data-testid="email-blacklist-bulk-add"]').trigger('click')
    await wrapper.get('[data-testid="email-blacklist-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="email-blacklist-edit"]').attributes('disabled')).toBeDefined()
    resolveFind({ items: [], total: 0 })
    await flushPromises()
    expect(wrapper.get('[data-testid="email-blacklist-edit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="email-blacklist-edit"]').trigger('click')
    const cancel = wrapper.findAll('button').find((button) => button.text() === 'common.cancel')!
    expect(cancel.attributes('disabled')).toBeUndefined()
    await cancel.trigger('click')
    expect(wrapper.find('[data-testid="email-blacklist-bulk-input"]').exists()).toBe(false)
  })

  it('命中用户超过单批上限时分批禁用并汇总人数', async () => {
    const users = Array.from({ length: 1001 }, (_, i) => ({ id: i + 1, email: `u${i}@gmail.com`, username: '' }))
    mocks.findUsers.mockResolvedValue({ items: users, total: users.length })
    mocks.batchDisable.mockImplementation(async (ids: number[]) => ({ affected: ids.length }))

    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-check"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-disable-confirm"]').trigger('click')
    await flushPromises()

    expect(mocks.batchDisable).toHaveBeenCalledTimes(2)
    expect(mocks.batchDisable.mock.calls[0]![0]).toHaveLength(1000)
    expect(mocks.batchDisable.mock.calls[1]![0]).toEqual([1001])
    expect(mocks.showSuccess).toHaveBeenLastCalledWith(
      'admin.settings.registration.emailBlacklist.disabled:{"count":"1,001"}'
    )
  })

  it('分批禁用中途失败时只保留未提交的用户，重试从断点继续', async () => {
    const users = Array.from({ length: 1500 }, (_, i) => ({ id: i + 1, email: `u${i}@gmail.com`, username: '' }))
    mocks.findUsers.mockResolvedValue({ items: users, total: users.length })
    mocks.batchDisable
      .mockResolvedValueOnce({ affected: 1000 })
      .mockRejectedValueOnce({ message: 'timeout' })
      .mockResolvedValueOnce({ affected: 500 })

    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-check"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="email-blacklist-disable-confirm"]').trigger('click')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('timeout')
    expect(wrapper.findAll('[data-testid="email-blacklist-matched-users"] li')).toHaveLength(500)

    await wrapper.get('[data-testid="email-blacklist-disable-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.batchDisable.mock.calls[2]![0]).toEqual(users.slice(1000).map((user) => user.id))
    expect(wrapper.find('[data-testid="email-blacklist-matched-users"]').exists()).toBe(false)
  })
})
