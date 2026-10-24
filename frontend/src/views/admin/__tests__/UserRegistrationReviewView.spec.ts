import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserRegistrationReviewView from '../UserRegistrationReviewView.vue'

const { getById, toggleStatus, showError, showSuccess } = vi.hoisted(() => ({
  getById: vi.fn(), toggleStatus: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { getById, toggleStatus } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))

const applicant = {
  id: 42, email: 'applicant@example.com', username: 'Applicant', role: 'user',
  status: 'pending', created_at: '2026-10-22T00:00:00Z',
  registration_reason: '研究用途\n<script>alert(1)</script>'
}
const wrappers: ReturnType<typeof mount>[] = []
function mountPage(id = '42') {
  const wrapper = mount(UserRegistrationReviewView, {
    props: { id },
    global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      LoadingSpinner: true
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.resetAllMocks()
  getById.mockResolvedValue({ ...applicant })
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('registration application details', () => {
  it('loads the applicant by URL ID and renders the full reason as plain text', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(getById).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain(applicant.email)
    expect(wrapper.text()).toContain(applicant.username)
    expect(wrapper.get('[data-test="reason"]').text()).toBe(applicant.registration_reason)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.get('a').attributes('href')).toBe('/admin/users')
  })

  it.each(['active', 'disabled'] as const)('reviews to %s and removes review controls', async status => {
    let finish!: (value: unknown) => void
    toggleStatus.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountPage()
    await flushPromises()
    const button = wrapper.get(`[data-test="${status}"]`)
    await button.trigger('click')
    expect(wrapper.get('[data-test="active"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="disabled"]').attributes('disabled')).toBeDefined()
    await button.trigger('click')
    expect(toggleStatus).toHaveBeenCalledTimes(1)
    expect(toggleStatus).toHaveBeenCalledWith(42, status)
    finish({ ...applicant, status })
    await flushPromises()
    expect(wrapper.find('[data-test="active"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(applicant.registration_reason)
    expect(showSuccess).toHaveBeenCalledWith(status === 'active' ? 'admin.users.reviewApproved' : 'admin.users.reviewRejected')
  })

  it('keeps the reason and allows retry after a failed review', async () => {
    toggleStatus.mockRejectedValue(new Error('offline'))
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="active"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.users.failedToToggle')
    expect(wrapper.get('[data-test="active"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain(applicant.registration_reason)
  })

  it.each([{ status: 'active' }, { role: 'admin' }])('does not offer review for %j', async overrides => {
    getById.mockResolvedValue({ ...applicant, ...overrides })
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('[data-test="active"]').exists()).toBe(false)
  })

  it('shows a retry when the applicant cannot be loaded', async () => {
    getById.mockRejectedValueOnce(new Error('missing'))
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.users.reviewDetailsLoadFailed')
    expect(wrapper.find('[data-test="active"]').exists()).toBe(false)
    await wrapper.get('[data-test="retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain(applicant.email)
  })

  it('shows an empty reason without hiding review controls', async () => {
    getById.mockResolvedValue({ ...applicant, registration_reason: '' })
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="reason"]').text()).toBe('admin.users.noApplicationContent')
    expect(wrapper.find('[data-test="active"]').exists()).toBe(true)
  })

  it('rejects invalid IDs without requesting a different applicant', async () => {
    const wrapper = mountPage('invalid')
    await flushPromises()
    expect(getById).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.users.reviewDetailsLoadFailed')
  })

  it('ignores a stale detail response after the URL ID changes', async () => {
    let finish!: (value: unknown) => void
    getById.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountPage()
    getById.mockResolvedValue({ ...applicant, id: 43, email: 'new@example.com' })
    await wrapper.setProps({ id: '43' })
    // Resolve the second request independently of the first.
    await flushPromises()
    finish({ ...applicant, email: 'stale@example.com' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale@example.com')
    expect(wrapper.text()).toContain('new@example.com')
    expect(getById).toHaveBeenLastCalledWith(43)
  })
})
