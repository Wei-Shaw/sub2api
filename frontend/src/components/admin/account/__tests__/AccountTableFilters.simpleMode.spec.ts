import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountTableFilters from '../AccountTableFilters.vue'
import Select from '@/components/common/Select.vue'
import type { AdminGroup } from '@/types'

const authState = vi.hoisted(() => ({ isSimpleMode: false }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('account group filters', () => {
  it.each([true, false])('keeps account filters compatible with Simple mode=%s', (simple) => {
    authState.isSimpleMode = simple
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '', filters: {},
        groups: [
          { id: 1, name: 'OpenAI accounts', platform: 'openai' },
          { id: 2, name: 'Composite routes', platform: 'composite' },
        ] as AdminGroup[],
      },
      global: { stubs: { Select: true, SearchInput: true } },
    })
    const options = wrapper.findAllComponents(Select).at(-1)!.props('options') as { value: string }[]
    const values = options.map(option => option.value)
    expect(values).toContain('1')
    expect(values).toContain('ungrouped')
    expect(values.includes('2')).toBe(!simple)
    wrapper.unmount()
  })
})
