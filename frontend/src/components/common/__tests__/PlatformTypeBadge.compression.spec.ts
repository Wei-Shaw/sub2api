import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('PlatformTypeBadge request compression', () => {
  it.each([
    ['br', 'Brotli', 'Content-Encoding: br'],
    ['zstd', 'Zstd', 'Content-Encoding: zstd'],
    ['deflate', 'Deflate', 'Content-Encoding: deflate'],
  ] as const)('shows the icon and name for %s', (algorithm, label, encoding) => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'apikey',
        compressRequestAlgorithm: algorithm,
      },
    })

    const badge = wrapper.get('[data-testid="request-compression-algorithm"]')
    expect(badge.text()).toBe(label)
    expect(badge.attributes('title')).toContain(encoding)
    expect(badge.getComponent(Icon).props('name')).toBe('compress')
  })

  it('renders nothing before negotiation and removes the badge when the state clears', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'anthropic', type: 'oauth' },
    })
    expect(wrapper.find('[data-testid="request-compression-algorithm"]').exists()).toBe(false)

    await wrapper.setProps({ compressRequestAlgorithm: 'zstd' })
    expect(wrapper.get('[data-testid="request-compression-algorithm"]').text()).toBe('Zstd')

    await wrapper.setProps({ compressRequestAlgorithm: undefined })
    expect(wrapper.find('[data-testid="request-compression-algorithm"]').exists()).toBe(false)
  })
})
