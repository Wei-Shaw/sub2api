import { describe, expect, it, vi } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
const messages = {
  'errors.pageNotFound': '页面未找到',
  'errors.pageNotFoundDescription': '你访问的页面不存在，或已被移动。',
  'errors.goBack': '返回上一页',
  'errors.needHelp': '需要帮助？',
  'errors.contactSupport': '联系客服',
  'home.goToDashboard': '进入控制台',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key as keyof typeof messages] ?? key,
  }),
}))

import NotFoundView from '../NotFoundView.vue'

describe('NotFoundView', () => {
  it('renders localized copy', async () => {
    setActivePinia(createPinia())

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/dashboard', name: 'Dashboard', component: { template: '<div />' } },
        { path: '/:pathMatch(.*)*', name: 'NotFound', component: NotFoundView },
      ],
    })
    await router.push('/missing-page')
    await router.isReady()

    const wrapper = mount(NotFoundView, {
      global: {
        plugins: [router],
        stubs: {
          Icon: { template: '<span />' },
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })

    expect(wrapper.text()).toContain('页面未找到')
    expect(wrapper.text()).toContain('你访问的页面不存在，或已被移动。')
    expect(wrapper.text()).toContain('返回上一页')
    expect(wrapper.text()).toContain('进入控制台')
    expect(wrapper.text()).toContain('需要帮助？')
    expect(wrapper.text()).toContain('联系客服')
  })
})
