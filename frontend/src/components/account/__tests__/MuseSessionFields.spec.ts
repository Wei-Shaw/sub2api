import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MuseSessionFields from '../MuseSessionFields.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Muse session setup', () => {
  it('provides the local exporter download and actionable setup steps', () => {
    const wrapper = mount(MuseSessionFields, { props: { ownerId: 1, sessionJson: '' } })
    expect(wrapper.get('a[download]').attributes('href')).toBe('/muse-session-exporter.zip?v=1')
    expect(wrapper.findAll('ol li')).toHaveLength(5)
    expect(wrapper.text()).toContain('chrome://extensions')
    expect(wrapper.text()).toContain('edge://extensions')
    expect(wrapper.get('a[href="https://muse.ai/"]').attributes('rel')).toContain('noopener')
    expect(wrapper.text()).toContain('admin.accounts.muse.afterSave')
  })

  it('imports a selected session file into the existing JSON field locally', async () => {
    const wrapper = mount(MuseSessionFields, { props: { ownerId: 1, sessionJson: '' } })
    const input = wrapper.get('input[type="file"]')
    const raw = '{"cookies":{"hatch_sess":"synthetic-session"}}'
    Object.defineProperty(input.element, 'files', { value: [{ size: raw.length, text: async () => raw }] })
    await input.trigger('change'); await flushPromises()
    expect(wrapper.emitted('update:sessionJson')).toEqual([[raw]])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('rejects oversized or malformed files without exposing their contents', async () => {
    for (const file of [{ size: 65537, text: async () => '{}' }, { size: 24, text: async () => 'synthetic-private-value' }, { size: 2, text: async () => '[]' }]) {
      const wrapper = mount(MuseSessionFields, { props: { ownerId: 1, sessionJson: '' } })
      const input = wrapper.get('input[type="file"]')
      Object.defineProperty(input.element, 'files', { value: [file] })
      await input.trigger('change'); await flushPromises()
      expect(wrapper.emitted('update:sessionJson')).toBeUndefined()
      expect(wrapper.get('[role="alert"]').text()).toBe('admin.accounts.muse.importFileError')
      expect(wrapper.text()).not.toContain('synthetic-private-value')
    }
  })
})
