import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountImportNaming from '../AccountImportNaming.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: { name: string }) => params?.name ?? key }) }))

describe('account import naming rule', () => {
  it('starts disabled and offers the requested GPT-Free sequence', async () => {
    const wrapper = mount(AccountImportNaming, { props: { modelValue: null } })
    expect(wrapper.find('[data-testid="import-name-preview"]').exists()).toBe(false)
    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ prefix: 'GPT-Free-', suffix: '', start_number: 1, padding: 2 }])
  })
  it('shows the exact first name without truncating digits', () => {
    const wrapper = mount(AccountImportNaming, { props: { modelValue: { prefix: 'GPT-Free-', suffix: '-A', start_number: 100, padding: 2 } } })
    expect(wrapper.get('[data-testid="import-name-preview"]').text()).toBe('GPT-Free-100-A')
  })
})
