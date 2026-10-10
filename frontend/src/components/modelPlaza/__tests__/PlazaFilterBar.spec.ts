import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.count != null ? `${key}:${params.count}` : key
    })
  }
})

function mountFilterBar() {
  return mount(PlazaFilterBar, {
    props: {
      platforms: [
        { name: 'openai', count: 8 },
        { name: 'anthropic', count: 4 }
      ],
      groups: [
        { id: 1, name: 'Balance', platform: 'openai', rate: 1 },
        { id: 2, name: 'Claude', platform: 'anthropic', rate: 0.8 }
      ],
      rates: [0.8, 1],
      billingModes: ['token', 'image'],
      platform: 'all',
      groupId: 'all',
      rate: 'all',
      billingMode: 'all',
      search: '',
      viewMode: 'card',
      resultCount: 12,
      totalCount: 12,
      activeFilterCount: 0
    }
  })
}

describe('PlazaFilterBar', () => {
  it('平台标签展示模型计数，工具栏展示结果数', () => {
    const wrapper = mountFilterBar()

    expect(wrapper.text()).toContain('openai')
    expect(wrapper.text()).toContain('8')
    expect(wrapper.text()).toContain('modelPlaza.filters.resultCount:12')
  })

  it('平台和视图选中态使用可适配深浅主题的主色样式', () => {
    const wrapper = mountFilterBar()
    const selectedPlatform = wrapper.get('[data-test="platform-filter-option"]')
    const selectedView = wrapper
      .findAll('[data-test="view-mode-option"]')
      .find((button) => button.attributes('aria-pressed') === 'true')

    for (const control of [selectedPlatform, selectedView]) {
      expect(control?.classes()).toContain('bg-primary-50')
      expect(control?.classes()).toContain('text-primary-700')
      expect(control?.classes()).toContain('dark:bg-primary-500/15')
      expect(control?.classes()).toContain('dark:text-primary-300')
      expect(control?.classes()).not.toContain('bg-gray-900')
      expect(control?.classes()).not.toContain('dark:bg-white')
    }
  })

  it('展开高级筛选后可以选择计费类型', async () => {
    const wrapper = mountFilterBar()

    await wrapper
      .findAll('button')
      .find((button) => button.text().includes('modelPlaza.filters.advanced'))!
      .trigger('click')
    const imageButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'modelPlaza.table.perImage')
    expect(imageButton).toBeDefined()

    await imageButton!.trigger('click')
    expect(wrapper.emitted('update:billingMode')).toEqual([['image']])
  })
})
