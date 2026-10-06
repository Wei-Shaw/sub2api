import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MuseSessionFields from '../MuseSessionFields.vue'
import Select from '@/components/common/Select.vue'

const { listUsers, getUser } = vi.hoisted(() => ({ listUsers: vi.fn(), getUser: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { list: listUsers, getById: getUser } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const alice = { id: 1, email: 'alice@example.com', username: 'Alice', status: 'active' }
const bob = { id: 7, email: 'bob@example.com', username: 'Bob', status: 'active' }
const mounted: Array<{ unmount: () => void }> = []
function mountFields(props = {}) {
  const wrapper = mount(MuseSessionFields, {
    props: { ownerId: 0, sessionJson: '', ...props },
    global: { stubs: { Teleport: true, Icon: true } },
  })
  mounted.push(wrapper)
  return wrapper
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

beforeEach(() => {
  listUsers.mockReset().mockResolvedValue({ items: [alice], total: 1 })
  getUser.mockReset().mockResolvedValue(bob)
})
afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount())
  vi.restoreAllMocks()
})

describe('Muse session setup', () => {
  it('offers existing users by name and email without a raw ID field or automatic assignment', async () => {
    const wrapper = mountFields()
    await flushPromises()
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
    expect(wrapper.emitted('update:ownerId')).toBeUndefined()
    await wrapper.get('#muse-owner').trigger('click')
    const option = wrapper.get('[role="option"]')
    expect(option.text()).toBe('Alice — alice@example.com')
    await option.trigger('click')
    expect(wrapper.emitted('update:ownerId')).toEqual([[1]])
  })

  it('keeps the assigned user visible when they are outside the current search page', async () => {
    const wrapper = mountFields({ ownerId: 7, replacement: true })
    await flushPromises()
    expect(getUser).toHaveBeenCalledWith(7)
    expect(wrapper.get('#muse-owner').text()).toContain('Bob — bob@example.com')
    listUsers.mockResolvedValueOnce({ items: [], total: 0 })
    wrapper.getComponent(Select).vm.$emit('search', 'unmatched')
    await flushPromises()
    expect(wrapper.get('#muse-owner').text()).toContain('Bob — bob@example.com')
    expect(wrapper.emitted('update:ownerId')).toBeUndefined()
    await wrapper.get('#muse-owner').trigger('click')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('admin.accounts.muse.noUsers')
    expect(wrapper.text()).toContain('admin.accounts.muse.savedSession')
  })

  it('ignores an older user response after a newer search finishes', async () => {
    const initial = deferred<{ items: typeof alice[]; total: number }>()
    listUsers.mockReturnValueOnce(initial.promise).mockResolvedValueOnce({ items: [bob], total: 1 })
    const wrapper = mountFields()
    wrapper.getComponent(Select).vm.$emit('search', 'bob')
    await flushPromises()
    initial.resolve({ items: [alice], total: 1 })
    await flushPromises()
    await wrapper.get('#muse-owner').trigger('click')
    expect(wrapper.get('[role="option"]').text()).toContain('bob@example.com')
    expect(wrapper.text()).not.toContain('alice@example.com')
    expect(listUsers).toHaveBeenLastCalledWith(1, 30, { search: 'bob', include_subscriptions: false }, { signal: expect.any(AbortSignal) })
  })

  it('reports a user-loading failure and retries without showing server error contents', async () => {
    listUsers.mockRejectedValueOnce(new Error('synthetic-private-server-error'))
    const wrapper = mountFields()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.accounts.muse.usersError')
    expect(wrapper.text()).not.toContain('synthetic-private-server-error')
    const retry = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.retry')!
    await retry.trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    await wrapper.get('#muse-owner').trigger('click')
    expect(wrapper.get('[role="option"]').text()).toContain('alice@example.com')
  })

  it('hides old choices while a new user search is loading', async () => {
    const pending = deferred<{ items: typeof bob[]; total: number }>()
    const wrapper = mountFields()
    await flushPromises()
    await wrapper.get('#muse-owner').trigger('click')
    expect(wrapper.get('[role="option"]').text()).toContain('alice@example.com')
    listUsers.mockReturnValueOnce(pending.promise)
    wrapper.getComponent(Select).vm.$emit('search', 'bob')
    await flushPromises()
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('common.loading')
    pending.resolve({ items: [bob], total: 1 })
    await flushPromises()
    expect(wrapper.get('[role="option"]').text()).toContain('bob@example.com')
  })

  it('does not replace the selected user with a late hydration response', async () => {
    const first = deferred<typeof bob>()
    getUser.mockReturnValueOnce(first.promise).mockResolvedValueOnce(alice)
    const wrapper = mountFields({ ownerId: 7 })
    await wrapper.setProps({ ownerId: 1 })
    await flushPromises()
    first.resolve(bob)
    await flushPromises()
    expect(wrapper.get('#muse-owner').text()).toContain('alice@example.com')
    expect(wrapper.emitted('update:ownerId')).toBeUndefined()
  })

  it('keeps session data under an advanced option and provides the exporter setup guide', () => {
    const wrapper = mountFields()
    expect(wrapper.get('[data-testid="muse-manual-session"]').attributes('open')).toBeUndefined()
    expect(wrapper.get('a[download]').attributes('href')).toBe('/muse-session-exporter.zip?v=1')
    expect(wrapper.findAll('ol li')).toHaveLength(5)
    expect(wrapper.text()).toContain('chrome://extensions')
    expect(wrapper.text()).toContain('edge://extensions')
    expect(wrapper.get('a[href="https://muse.ai/"]').attributes('rel')).toContain('noopener')
    expect(wrapper.text()).toContain('admin.accounts.muse.afterSave')
  })

  it('imports a chosen session file and shows it is ready to save without exposing its contents', async () => {
    const wrapper = mountFields()
    const input = wrapper.get('input[type="file"]')
    const raw = '{"cookies":{"hatch_sess":"synthetic-session"}}'
    Object.defineProperty(input.element, 'files', { value: [{ name: 'muse-session.json', size: raw.length, text: async () => raw }] })
    await input.trigger('change')
    await flushPromises()
    expect(wrapper.emitted('update:sessionJson')).toEqual([[raw]])
    await wrapper.setProps({ sessionJson: raw })
    expect(wrapper.get('[role="status"]').text()).toContain('admin.accounts.muse.fileReady')
    expect(wrapper.get('[role="status"]').text()).toContain('muse-session.json')
    expect(wrapper.get('[data-testid="muse-manual-session"]').attributes('open')).toBeUndefined()
    expect(wrapper.text()).not.toContain('synthetic-session')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('accepts a dropped session file through the same importer', async () => {
    const wrapper = mountFields()
    const raw = '{"cookies":{"hatch_sess":"synthetic-session"}}'
    const dropZone = wrapper.get('[data-testid="muse-session-import"] > div.border-dashed')
    await dropZone.trigger('drop', { dataTransfer: { files: [{ name: 'muse-session.json', size: raw.length, text: async () => raw }] } })
    await flushPromises()
    expect(wrapper.emitted('update:sessionJson')).toEqual([[raw]])
  })

  it('does not apply an unfinished file import to another account', async () => {
    const text = deferred<string>()
    const wrapper = mountFields({ accountId: 10 })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [{ name: 'muse-session.json', size: 12, text: () => text.promise }] })
    await input.trigger('change')
    await wrapper.setProps({ accountId: 20 })
    text.resolve('{"opaque":"synthetic-session"}')
    await flushPromises()
    expect(wrapper.emitted('update:sessionJson')).toBeUndefined()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('only marks valid pasted session data ready to save', async () => {
    const wrapper = mountFields({ sessionJson: 'invalid' })
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    await wrapper.setProps({ sessionJson: '{"opaque":"fixture"}' })
    expect(wrapper.get('[role="status"]').text()).toBe('admin.accounts.muse.sessionReady')
  })

  it('rejects oversized or malformed files without changing session data or exposing contents', async () => {
    for (const file of [{ size: 65537, text: async () => '{}' }, { size: 24, text: async () => 'synthetic-private-value' }, { size: 2, text: async () => '[]' }]) {
      const wrapper = mountFields({ sessionJson: '{"opaque":"saved"}' })
      const input = wrapper.get('input[type="file"]')
      Object.defineProperty(input.element, 'files', { value: [file] })
      await input.trigger('change')
      await flushPromises()
      expect(wrapper.emitted('update:sessionJson')).toBeUndefined()
      expect(wrapper.get('[role="alert"]').text()).toBe('admin.accounts.muse.importFileError')
      expect(wrapper.text()).not.toContain('synthetic-private-value')
    }
  })
})
