import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PlazaModelDetailDrawer from '../PlazaModelDetailDrawer.vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.model ? `${key}:${params.model}` : key
    })
  }
})

const model: PlazaModel = {
  name: 'gpt-test',
  platform: 'openai',
  pricing: {
    billing_mode: 'token',
    input_price: 3e-6,
    output_price: 1.5e-5,
    cache_write_price: 3.75e-6,
    cache_read_price: 3e-7,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  },
  official_pricing: {
    input_price: 5e-6,
    output_price: 3e-5,
    cache_write_price: null,
    cache_read_price: null
  }
}

const group: ModelPlazaGroup = {
  id: 1,
  name: 'Balance',
  description: 'Stable production group',
  platform: 'openai',
  subscription_type: 'standard',
  rate_multiplier: 1,
  user_rate_multiplier: 0.5,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  is_exclusive: false,
  models: [model]
}

afterEach(() => {
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

describe('PlazaModelDetailDrawer', () => {
  it('展示折后价、官方参考价和真实分组字段', () => {
    setActivePinia(createPinia())
    const wrapper = mount(PlazaModelDetailDrawer, {
      attachTo: document.body,
      props: { open: true, model, group }
    })

    const text = document.body.textContent ?? ''
    expect(text).toContain('gpt-test')
    expect(text).toContain('$1.50')
    expect(text).toContain('$7.50')
    expect(text).toContain('$5.00')
    expect(text).toContain('$30.00')
    expect(text).toContain('Balance')
    expect(text).toContain('Stable production group')
    expect(text).toContain('0.5x')
    wrapper.unmount()
  })

  it('Escape 关闭抽屉', async () => {
    setActivePinia(createPinia())
    const wrapper = mount(PlazaModelDetailDrawer, {
      attachTo: document.body,
      props: { open: true, model, group }
    })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
