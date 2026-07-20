import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import QuotaProviderIcon from '../QuotaProviderIcon.vue'

describe('QuotaProviderIcon', () => {
  it('renders the official NewAPI color mark', () => {
    const wrapper = mount(QuotaProviderIcon, { props: { provider: 'newapi' } })

    const paths = wrapper.findAll('path')
    expect(paths).toHaveLength(3)
    expect(paths[0].attributes('d')).toContain('M23.078 16.34')
    expect(paths.every(path => path.attributes('fill').startsWith('url(#quota-provider-newapi-'))).toBe(true)
    expect(wrapper.findAll('linearGradient')).toHaveLength(3)
    expect(wrapper.findAll('stop').map(stop => stop.attributes('stop-color'))).toEqual([
      '#F85EAD', '#FD75FD', '#11F5EF', '#C738FB', '#11F5EF', '#C738FB'
    ])
  })
})
