import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ProviderLogo from '../ProviderLogo.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

describe('ProviderLogo', () => {
  it('prefers a safe HTTPS custom logo', () => {
    const wrapper = mount(ProviderLogo, {
      props: {
        provider: 'custom',
        logoKey: 'openai',
        logoUrl: 'https://cdn.example.com/logo.svg',
        alt: 'Custom'
      }
    })

    expect(wrapper.get('img').attributes('src')).toBe('https://cdn.example.com/logo.svg')
    expect(wrapper.get('img').attributes('alt')).toBe('Custom')
  })

  it('rejects unsafe URLs and maps moonshot to the built-in Kimi logo', () => {
    const wrapper = mount(ProviderLogo, {
      props: {
        provider: 'moonshot',
        logoKey: 'moonshot',
        logoUrl: 'javascript:alert(1)'
      }
    })

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.findComponent(PlatformIcon).props('platform')).toBe('kimi')
  })

  it('falls back to the built-in logo when a custom image fails', async () => {
    const wrapper = mount(ProviderLogo, {
      props: { provider: 'deepseek', logoUrl: '/brand/deepseek.svg' }
    })

    await wrapper.get('img').trigger('error')
    expect(wrapper.findComponent(PlatformIcon).props('platform')).toBe('deepseek')
  })
})
