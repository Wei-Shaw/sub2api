import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import { createPinia, setActivePinia REDACTED from 'pinia'
import { readFileSync REDACTED from 'node:fs'
import { resolve REDACTED from 'node:path'

import AnnouncementPopup from '../AnnouncementPopup.vue'
import { useAnnouncementStore REDACTED from '@/stores/announcements'

const announcementMarkdownStyles = readFileSync(
  resolve(process.cwd(), 'src/styles/announcement-markdown.css'),
  'utf8',
)

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    REDACTED),
  REDACTED
REDACTED)

const announcement = {
  id: 1,
  title: 'Preview announcement',
  content: '## Preview heading\n\n<div>HTML content</div><script>window.__xss = true</script>',
  status: 'draft' as const,
  notify_mode: 'popup' as const,
  targeting: { any_of: [] REDACTED,
  created_at: '2026-07-24T07:30:00Z',
  updated_at: '2026-07-24T07:30:00Z',
REDACTED

describe('AnnouncementPopup', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  REDACTED)

  afterEach(() => {
    document.body.innerHTML = ''
    document.body.style.overflow = ''
  REDACTED)

  it('renders mixed Markdown and HTML inside the shared styled container', async () => {
    const store = useAnnouncementStore()
    store.currentPopup = {
      id: 1,
      title: 'Mixed content announcement',
      content: [
        '## Markdown heading',
        '',
        '<div><h3>HTML heading</h3><ul><li>HTML list item</li></ul></div>',
        '',
        '<table><thead><tr><th>Status</th></tr></thead><tbody><tr><td>OK</td></tr></tbody></table>',
        '<script>window.__announcementXss = true</script>',
      ].join('\n'),
      notify_mode: 'popup',
      created_at: '2026-07-24T07:30:00Z',
      updated_at: '2026-07-24T07:30:00Z',
    REDACTED

    const wrapper = mount(AnnouncementPopup)
    await wrapper.vm.$nextTick()

    const content = document.body.querySelector('.markdown-body')
    expect(content?.querySelector('h2')?.textContent).toBe('Markdown heading')
    expect(content?.querySelector('h3')?.textContent).toBe('HTML heading')
    expect(content?.querySelector('li')?.textContent).toBe('HTML list item')
    expect(content?.querySelector('table td')?.textContent).toBe('OK')
    expect(content?.querySelector('script')).toBeNull()

    wrapper.unmount()
  REDACTED)

  it.each(['h2', 'h3', 'ul', 'li', 'blockquote', 'table', 'th', 'td', 'code'])(
    'loads a shared style rule for mixed-content <%s> elements',
    (element) => {
      expect(announcementMarkdownStyles).toContain(`.markdown-body ${elementREDACTED`)
    REDACTED,
  )

  it('previews an admin announcement without marking it as read', async () => {
    const store = useAnnouncementStore()
    const dismissPopup = vi.spyOn(store, 'dismissPopup')
    const wrapper = mount(AnnouncementPopup, {
      props: {
        announcement,
        preview: true,
      REDACTED,
    REDACTED)

    expect(document.body.textContent).toContain('Preview announcement')
    expect(document.body.querySelector('.markdown-body h2')?.textContent).toBe('Preview heading')
    expect(document.body.querySelector('.markdown-body script')).toBeNull()
    expect(document.body.textContent).toContain('common.close')

    const dismissButton = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="announcement-popup-dismiss"]',
    )
    dismissButton?.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(dismissPopup).not.toHaveBeenCalled()

    await wrapper.setProps({ announcement: null REDACTED)
    expect(document.body.style.overflow).toBe('')
    wrapper.unmount()
  REDACTED)

  it('keeps the existing user popup dismissal behavior', async () => {
    const store = useAnnouncementStore()
    store.currentPopup = announcement
    const dismissPopup = vi.spyOn(store, 'dismissPopup').mockResolvedValue()
    const wrapper = mount(AnnouncementPopup)

    const dismissButton = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="announcement-popup-dismiss"]',
    )
    dismissButton?.click()
    await wrapper.vm.$nextTick()

    expect(dismissPopup).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  REDACTED)
REDACTED)
