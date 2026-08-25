import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('PlazaFilterBar', () => {
  it('shows Clear when sorting is the only active filter', () => {
    const wrapper = mount(PlazaFilterBar, {
      props: {
        platforms: ['openai'],
        groups: [{ id: 1, name: 'OpenAI', platform: 'openai', rate: 1 }],
        rates: [1],
        platform: 'all',
        groupId: 'all',
        rate: 'all',
        search: '',
        sort: 'name'
      },
      global: { stubs: { PlatformIcon: true, Icon: true } }
    })

    expect(wrapper.find('button').text()).toBe('modelPlaza.filters.clear')
  })
})
