import { describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import AccountTableActions from '../account/AccountTableActions.vue'
import IntervalRow from '../channel/IntervalRow.vue'
import ModelTagInput from '../channel/ModelTagInput.vue'
import HeaderOverrideEditor from '@/components/account/HeaderOverrideEditor.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

// Every button needs a name a screen reader can announce: visible text, aria-label or title.
const unnamedButtons = (wrapper: VueWrapper) =>
  wrapper
    .findAll('button')
    .map((b) => b.element)
    .filter((el) => !el.getAttribute('aria-label') && !el.getAttribute('title') && !el.textContent?.trim())
    .map((el) => el.outerHTML)

const global = { stubs: { Icon: true, HeaderOverrideJsonTools: true } }

describe('admin icon-only buttons', () => {
  it.each([
    ['AccountTableActions', AccountTableActions, { loading: false }],
    ['ModelTagInput', ModelTagInput, { models: ['claude-sonnet-4', 'gpt-5'] }],
    ['HeaderOverrideEditor', HeaderOverrideEditor, { rows: [{ name: 'x-a', value: '1' }] }],
    [
      'IntervalRow',
      IntervalRow,
      {
        mode: 'token',
        interval: {
          min_tokens: 0,
          max_tokens: null,
          tier_label: '',
          input_price: 1,
          output_price: 2,
          cache_write_price: null,
          cache_read_price: null,
          input_multiplier: null,
          output_multiplier: null,
          cache_write_multiplier: null,
          cache_read_multiplier: null,
          per_request_price: null,
          sort_order: 0
        }
      }
    ]
  ])('%s names every button', (_name, component, props) => {
    const wrapper = mount(component as any, { props, global })
    expect(wrapper.findAll('button').length).toBeGreaterThan(0)
    expect(unnamedButtons(wrapper)).toEqual([])
  })
})
