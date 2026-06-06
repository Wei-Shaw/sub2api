import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserBreakdownSubTable from '../UserBreakdownSubTable.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('UserBreakdownSubTable', () => {
  const baseItem = {
    user_id: 7,
    email: 'invite-8794805d94e3@example.com',
    requests: 12,
    total_tokens: 3456,
    cost: 1.23,
    actual_cost: 0.99,
    account_cost: 1.11
  }

  it('renders device_code before generated email or username labels', () => {
    const wrapper = mount(UserBreakdownSubTable, {
      props: {
        items: [{
          ...baseItem,
          username: 'invite-8794805d94e3',
          device_code: 'DLG-ABCD-1234'
        }],
        loading: false
      }
    })

    const text = wrapper.text()
    expect(text).toContain('DLG-ABCD-1234')
    expect(text).not.toContain('invite-8794805d94e3@example.com')
    expect(text).not.toContain('invite-8794805d94e3')
  })

  it('preserves username/email/id fallback when no device_code exists', () => {
    const wrapper = mount(UserBreakdownSubTable, {
      props: {
        items: [
          { ...baseItem, user_id: 8, username: 'friendly-user', email: 'friendly@example.com' },
          { ...baseItem, user_id: 9, email: 'email-only@example.com' },
          { ...baseItem, user_id: 10, email: '' }
        ],
        loading: false
      }
    })

    const text = wrapper.text()
    expect(text).toContain('friendly-user')
    expect(text).toContain('email-only@example.com')
    expect(text).toContain('User #10')
  })
})
