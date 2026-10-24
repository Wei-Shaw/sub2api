import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('PlatformTypeBadge StepFun', () => {
  it('renders StepFun with its emerald platform color', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'stepfun', type: 'apikey' },
    })

    expect(wrapper.text()).toContain('StepFun')
    expect(wrapper.text()).toContain('Key')
    expect(wrapper.html()).toContain('bg-emerald-100')
  })
})
