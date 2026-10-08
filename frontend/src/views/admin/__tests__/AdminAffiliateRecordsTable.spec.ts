import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AdminAffiliateRecordsTable from '../affiliates/AdminAffiliateRecordsTable.vue'
import Select from '@/components/common/Select.vue'

const { listInviteRecords, listRebateRecords, listTransferRecords, getUserOverview, showError } = vi.hoisted(() => ({
  listInviteRecords: vi.fn(),
  listRebateRecords: vi.fn(),
  listTransferRecords: vi.fn(),
  getUserOverview: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin/affiliates', () => {
  const affiliatesAPI = {
    listInviteRecords,
    listRebateRecords,
    listTransferRecords,
    getUserOverview
  }

  return {
    affiliatesAPI,
    default: affiliatesAPI
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, fallback?: string) => fallback || key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <table>
      <tbody>
        <tr v-for="row in data" :key="row.ledger_id">
          <td v-for="column in columns" :key="column.key">
            <slot :name="'cell-' + column.key" :row="row" :value="row[column.key]">
              {{ row[column.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  `
}

function rebateResponse(ledgerId: number, redeemCodeMasked: string) {
  return {
    items: [
      {
        ledger_id: ledgerId,
        source_type: 'balance_redeem_code',
        redeem_code_id: 18,
        redeem_code_masked: redeemCodeMasked,
        inviter_id: 60,
        inviter_email: 'inviter@example.com',
        inviter_username: 'inviter',
        invitee_id: 81,
        invitee_email: 'invitee@example.com',
        invitee_username: 'invitee',
        base_amount: 100,
        rebate_amount: 20,
        created_at: '2026-08-27T10:00:00Z'
      }
    ],
    total: 1,
    page: 1,
    page_size: 20,
    pages: 1
  }
}

describe('管理端邀请返利来源筛选', () => {
  beforeEach(() => {
    localStorage.clear()
    listInviteRecords.mockReset()
    listRebateRecords.mockReset()
    listTransferRecords.mockReset()
    getUserOverview.mockReset()
    showError.mockReset()

    listRebateRecords.mockResolvedValue(rebateResponse(91, 'abcd****1234'))
  })

  it('默认查询全部来源，并可切换到余额兑换码来源', async () => {
    const wrapper = mount(AdminAffiliateRecordsTable, {
      props: { type: 'rebates' },
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          OrderStatusBadge: true
        }
      }
    })

    await flushPromises()
    expect(listRebateRecords).toHaveBeenNthCalledWith(1, expect.objectContaining({
      page: 1,
      source_type: 'all'
    }))
    expect(wrapper.text()).toContain('abcd****1234')

    const sourceFilter = wrapper.getComponent(Select)
    sourceFilter.vm.$emit('update:modelValue', 'balance_redeem_code')
    sourceFilter.vm.$emit('change', 'balance_redeem_code')
    await flushPromises()

    expect(listRebateRecords).toHaveBeenLastCalledWith(expect.objectContaining({
      page: 1,
      source_type: 'balance_redeem_code'
    }))
  })

  it('余额兑换码和管理员充值使用统一的成功状态徽标', async () => {
    const response = rebateResponse(91, 'abcd****1234')
    response.items.push({
      ...response.items[0],
      ledger_id: 92,
      source_type: 'admin_recharge'
    })
    response.total = 2
    listRebateRecords.mockResolvedValueOnce(response)

    const wrapper = mount(AdminAffiliateRecordsTable, {
      props: { type: 'rebates' },
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          OrderStatusBadge: true
        }
      }
    })

    await flushPromises()

    const successBadgeClasses = [
      'inline-flex',
      'rounded-full',
      'bg-green-100',
      'text-green-800',
      'dark:bg-green-900/30',
      'dark:text-green-400'
    ]
    for (const label of [
      'admin.affiliates.records.sourceStatuses.redeemed',
      'admin.affiliates.records.sourceStatuses.credited'
    ]) {
      const badge = wrapper.findAll('span').find((node) => node.text() === label)
      expect(badge).toBeDefined()
      expect(badge!.classes()).toEqual(expect.arrayContaining(successBadgeClasses))
    }
  })

  it('旧筛选的慢响应不会覆盖新筛选结果', async () => {
    let resolveFirstRequest!: (value: ReturnType<typeof rebateResponse>) => void
    listRebateRecords
      .mockReset()
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirstRequest = resolve
      }))
      .mockResolvedValueOnce(rebateResponse(102, 'new****2222'))

    const wrapper = mount(AdminAffiliateRecordsTable, {
      props: { type: 'rebates' },
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          OrderStatusBadge: true
        }
      }
    })

    await vi.waitFor(() => expect(listRebateRecords).toHaveBeenCalledTimes(1))
    const sourceFilter = wrapper.getComponent(Select)
    sourceFilter.vm.$emit('update:modelValue', 'balance_redeem_code')
    sourceFilter.vm.$emit('change', 'balance_redeem_code')
    await flushPromises()
    expect(wrapper.text()).toContain('new****2222')

    resolveFirstRequest(rebateResponse(101, 'old****1111'))
    await flushPromises()
    expect(wrapper.text()).toContain('new****2222')
    expect(wrapper.text()).not.toContain('old****1111')
  })
})
