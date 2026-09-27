import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))

import EndpointPopover from '../EndpointPopover.vue'

describe('EndpointPopover on small screens', () => {
  it('lets long URLs wrap and gives the copy and speed-test controls a 40px target', () => {
    const wrapper = mount(EndpointPopover, {
      props: { apiBaseUrl: 'https://a-very-long-subdomain.gateway.example.com/v1', customEndpoints: [] },
    })

    expect(wrapper.get('code').classes()).toEqual(expect.arrayContaining(['min-w-0', 'break-all']))
    for (const control of [wrapper.get('button'), wrapper.get('a')]) {
      expect(control.classes()).toEqual(expect.arrayContaining(['min-h-10', 'min-w-10']))
    }
  })
})
