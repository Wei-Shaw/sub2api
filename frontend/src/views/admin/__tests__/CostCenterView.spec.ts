const apiMocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  getEvents: vi.fn(),
  createExpense: vi.fn(),
  listAccounts: vi.fn(),
}))

const appStoreMocks = vi.hoisted(() => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    costCenter: {
      getSummary: apiMocks.getSummary,
      getEvents: apiMocks.getEvents,
      createExpense: apiMocks.createExpense,
      updateEventStatus: vi.fn(),
      reverseEvent: vi.fn(),
    },
    accounts: { list: apiMocks.listAccounts },
  },
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => appStoreMocks }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => ({
        'admin.costCenter.appendCost': 'Append cost',
        'admin.costCenter.expenseAccount': 'Expense account',
        'admin.costCenter.selectAccount': 'Select account',
        'admin.costCenter.noAccount': 'No linked account',
        'admin.costCenter.note': 'Note',
        'admin.costCenter.notePlaceholder': 'Evidence',
        'admin.costCenter.amountUsd': 'Amount',
        'admin.costCenter.category': 'Category',
        'admin.costCenter.occurredAt': 'Occurred at',
        'admin.costCenter.costAppended': 'Cost appended',
        'admin.costCenter.rebateAmount': 'Rebate amount',
        'admin.costCenter.rebateAmountHelp': 'Included in operating expenses',
        'common.confirm': 'Confirm',
        'common.cancel': 'Cancel',
        'common.loading': 'Loading',
      } as Record<string, string>)[key] ?? key,
    }),
  }
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CostCenterView from '../CostCenterView.vue'

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options', 'placeholder'],
  emits: ['update:modelValue', 'change'],
  template: '<div data-testid="select-stub">{{ placeholder }}</div>',
}

describe('CostCenterView account cost entry', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getSummary.mockResolvedValue({ data: null })
    apiMocks.getEvents.mockResolvedValue({ data: { items: [], total: 0 } })
    apiMocks.createExpense.mockResolvedValue({ data: { id: 1 } })
    apiMocks.listAccounts.mockImplementation(async (page: number) => ({
      items: page === 1
        ? [{ id: 42, name: 'atlas-primary', platform: 'atlascloud' }]
        : [{ id: 43, name: 'atlas-child', platform: 'atlascloud' }],
      total: 2,
      page,
      page_size: 1,
      pages: 2,
    }))
  })

  it('shows the rebate amount in the summary cards', async () => {
    apiMocks.getSummary.mockResolvedValue({
      data: {
        cash_income: 100,
        realized_income: 80,
        settled_expenses: 30,
        rebate_amount: 12.34,
        cash_profit: 70,
        operating_profit: 50,
        deferred_subscription_usd: 20,
        expired_entitlement_usd: 3,
      },
    })

    const wrapper = mount(CostCenterView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: true,
          DateRangePicker: true,
          DataTable: true,
          Icon: true,
          Select: SelectStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Rebate amount')
    expect(wrapper.text()).toContain('$12.34')
    expect(wrapper.text()).toContain('Included in operating expenses')
  })

  it('selects an account and submits an audited settled expense', async () => {
    const wrapper = mount(CostCenterView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
          DateRangePicker: true,
          DataTable: true,
          Icon: true,
          Select: SelectStub,
        },
      },
    })
    await flushPromises()

    const appendButton = wrapper.findAll('button').find(button => button.text() === 'Append cost')
    expect(appendButton).toBeDefined()
    await appendButton!.trigger('click')
    await flushPromises()

    expect(apiMocks.listAccounts).toHaveBeenCalledWith(1, 1000, {
      lite: '1',
      include_scheduler_score: '0',
      sort_by: 'name',
      sort_order: 'asc',
    })
    expect(apiMocks.listAccounts).toHaveBeenCalledWith(2, 1000, {
      lite: '1',
      include_scheduler_score: '0',
      sort_by: 'name',
      sort_order: 'asc',
    })
    const accountSelect = wrapper.findAllComponents(SelectStub).find(select => select.props('placeholder') === 'Select account')
    expect(accountSelect?.props('options')).toEqual([
      { value: null, label: 'No linked account' },
      { value: 42, label: 'atlas-primary · atlascloud · #42' },
      { value: 43, label: 'atlas-child · atlascloud · #43' },
    ])
    accountSelect!.vm.$emit('update:modelValue', 42)

    await wrapper.get('input[type="number"]').setValue('12.5')
    await wrapper.get('textarea').setValue('August account invoice')
    const occurredAt = wrapper.get('input[type="datetime-local"]')
    await occurredAt.setValue('2026-08-14T10:30')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(apiMocks.createExpense).toHaveBeenCalledWith({
      account_id: 42,
      amount_usd: 12.5,
      category: 'account_expense',
      occurred_at: new Date('2026-08-14T10:30').toISOString(),
      note: 'August account invoice',
      status: 'settled',
    })
    expect(appStoreMocks.showSuccess).toHaveBeenCalledWith('Cost appended')
    expect(apiMocks.getSummary).toHaveBeenCalledTimes(2)
    expect(apiMocks.getEvents).toHaveBeenCalledTimes(2)
  })

  it('submits without linking an account or providing a note', async () => {
    const wrapper = mount(CostCenterView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
          DateRangePicker: true,
          DataTable: true,
          Icon: true,
          Select: SelectStub,
        },
      },
    })
    await flushPromises()

    const appendButton = wrapper.findAll('button').find(button => button.text() === 'Append cost')
    await appendButton!.trigger('click')
    await flushPromises()

    const categorySelect = wrapper.findAllComponents(SelectStub).find(select =>
      (select.props('options') as Array<{ value: string }>).map(option => option.value).join(',') ===
        'account_expense,audit_account_expense,account_setup,account_renewal,account_recharge,proxy,other'
    )
    expect(categorySelect).toBeDefined()
    categorySelect!.vm.$emit('update:modelValue', 'audit_account_expense')

    expect(wrapper.text()).toContain('Note')
    expect(wrapper.get('textarea').attributes('required')).toBeUndefined()
    await wrapper.get('input[type="number"]').setValue('23.75')
    await wrapper.get('input[type="datetime-local"]').setValue('2026-08-14T11:45')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(apiMocks.createExpense).toHaveBeenCalledWith({
      amount_usd: 23.75,
      category: 'audit_account_expense',
      occurred_at: new Date('2026-08-14T11:45').toISOString(),
      status: 'settled',
    })
    expect(apiMocks.createExpense.mock.calls[0][0]).not.toHaveProperty('account_id')
    expect(apiMocks.createExpense.mock.calls[0][0]).not.toHaveProperty('note')
    expect(appStoreMocks.showSuccess).toHaveBeenCalledWith('Cost appended')
    expect(apiMocks.getSummary).toHaveBeenCalledTimes(2)
    expect(apiMocks.getEvents).toHaveBeenCalledTimes(2)
  })
})
