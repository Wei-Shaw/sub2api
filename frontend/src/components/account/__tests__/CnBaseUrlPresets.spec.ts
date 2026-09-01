import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CnBaseUrlPresets from '../CnBaseUrlPresets.vue'

describe('CnBaseUrlPresets', () => {
  it('requires mode, protocol, and URL to mark a shared MiniMax endpoint active', () => {
    const wrapper = mount(CnBaseUrlPresets, {
      props: {
        platform: 'minimax',
        mode: 'coding',
        protocol: 'chat_completions',
        currentUrl: 'https://api.minimaxi.com/v1'
      }
    })
    const active = wrapper.findAll('[data-testid="cn-base-url-preset"]')
      .filter(button => button.classes().includes('bg-primary-100'))
    expect(active).toHaveLength(1)
    expect(active[0].text()).toContain('MiniMax Token Plan')
  })
})
