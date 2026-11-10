import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import EmailTemplateEditor from '../EmailTemplateEditor.vue'

const { getEmailTemplates, getEmailTemplate, previewEmailTemplate, showError } = vi.hoisted(() => ({
  getEmailTemplates: vi.fn(), getEmailTemplate: vi.fn(), previewEmailTemplate: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api', () => ({ adminAPI: { settings: { getEmailTemplates, getEmailTemplate, previewEmailTemplate } } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: ref('en') }) }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  getEmailTemplates.mockResolvedValue({ events: ['auth.verify_code', 'auth.password_reset'], locales: ['en'] })
  getEmailTemplate.mockImplementation((event: string) => Promise.resolve({ subject: event, html: `<p>${event}</p>` }))
  previewEmailTemplate.mockResolvedValue({ subject: 'Initial', html: '<p>Initial</p>' })
})

describe('email template previews', () => {
  it('keeps the newer preview loading when the older request finishes', async () => {
    const wrapper = mount(EmailTemplateEditor, { global: { stubs: { Icon: true } } })
    await flushPromises()
    let oldFinish!: (value: object) => void
    let newFinish!: (value: object) => void
    previewEmailTemplate.mockImplementationOnce(() => new Promise(resolve => { oldFinish = resolve }))
    await wrapper.findAll('button').find(button => button.text() === 'admin.settings.emailTemplates.preview')!.trigger('click')
    previewEmailTemplate.mockImplementationOnce(() => new Promise(resolve => { newFinish = resolve }))
    await wrapper.findAll('select')[0].setValue('auth.password_reset')
    await flushPromises()
    oldFinish({ subject: 'Old', html: '<p>Old</p>' })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.settings.emailTemplates.previewing')
    newFinish({ subject: 'Current', html: '<p>Current</p>' })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('srcdoc')).toBe('<p>Current</p>')
  })


  it.each(['success', 'failure'])('ignores an obsolete preview %s after selecting another template', async (outcome) => {
    const wrapper = mount(EmailTemplateEditor, { global: { stubs: { Icon: true } } })
    await flushPromises()
    let resolve!: (value: object) => void
    let reject!: (error: Error) => void
    previewEmailTemplate.mockImplementationOnce(() => new Promise((res, rej) => { resolve = res; reject = rej }))
    const previewButton = wrapper.findAll('button').find(button => button.text() === 'admin.settings.emailTemplates.preview')!
    await previewButton.trigger('click')
    previewEmailTemplate.mockResolvedValueOnce({ subject: 'Newest', html: '<p>Newest</p>' })
    await wrapper.findAll('select')[0].setValue('auth.password_reset')
    await flushPromises()
    expect(wrapper.get('iframe').attributes('srcdoc')).toBe('<p>Newest</p>')
    if (outcome === 'success') resolve({ subject: 'Obsolete', html: '<p>Obsolete</p>' })
    else reject(new Error('obsolete preview failed'))
    await flushPromises()
    expect(wrapper.get('iframe').attributes('srcdoc')).toBe('<p>Newest</p>')
    expect(showError).not.toHaveBeenCalled()
  })
})
