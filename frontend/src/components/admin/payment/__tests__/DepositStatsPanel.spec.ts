import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DepositStatsPanel from '../DepositStatsPanel.vue'
import type { DepositStats } from '@/types/payment'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => {
      const messages: Record<string, string> = {
        'payment.admin.depositOverview': 'Deposit / package grants',
        'payment.admin.depositOverviewDesc': 'Track deposits by recipient',
        'payment.admin.totalDepositEvents': 'Deposit events',
        'payment.admin.depositLedgerAmount': 'Ledger credited',
        'payment.admin.depositCredits': 'Credits granted',
        'payment.admin.depositPackageAssignments': 'Packages granted',
        'payment.admin.depositAdminAutoBreakdown': 'Admin / auto',
        'payment.admin.adminAdjustments': 'Admin deposits',
        'payment.admin.manualAssignments': 'Manual grants',
        'payment.admin.autoAssignments': 'Auto grants',
        'payment.admin.depositSources': 'Deposit sources',
        'payment.admin.depositRecipients': 'Top deposit recipients',
        'payment.admin.recentDeposits': 'Recent deposits',
        'payment.admin.noData': 'No data',
        'payment.admin.lastDeposit': 'Last deposit',
        'payment.admin.colUser': 'User',
        'payment.admin.colSource': 'Source',
        'payment.admin.colAmount': 'Amount',
        'payment.admin.colPackage': 'Package',
        'payment.admin.operator': 'Operator',
        'payment.admin.days': 'days',
        'payment.admin.depositSourceLabels.admin_balance_adjustment': 'Admin balance deposit',
        'payment.admin.depositSourceLabels.auto_subscription_assignment': 'Automatic package grant',
      }
      return messages[key] || fallback || key
    },
  }),
}))

const stats: DepositStats = {
  total_events: 2,
  total_ledger_amount: { USD: 25, CNY: 70 },
  total_credits: 12500,
  subscription_assignments: 1,
  paid_topups: 0,
  redeem_deposits: 0,
  admin_adjustments: 1,
  manual_assignments: 0,
  auto_assignments: 1,
  by_source: [
    {
      source: 'admin_balance_adjustment',
      count: 1,
      ledger_amount: { USD: 25, CNY: 70 },
      credits: 12500,
      subscription_assignments: 0,
      last_deposit_at: '2026-05-30T01:00:00Z',
    },
    {
      source: 'auto_subscription_assignment',
      count: 1,
      ledger_amount: { USD: 0 },
      credits: 0,
      subscription_assignments: 1,
      last_deposit_at: '2026-05-30T02:00:00Z',
    },
  ],
  top_recipients: [
    {
      user_id: 11,
      email: 'target@example.com',
      username: 'target',
      count: 2,
      ledger_amount: { USD: 25, CNY: 70 },
      credits: 12500,
      subscription_assignments: 1,
      last_deposit_at: '2026-05-30T02:00:00Z',
      last_source: 'auto_subscription_assignment',
    },
  ],
  recent_events: [
    {
      source: 'auto_subscription_assignment',
      user_id: 11,
      email: 'target@example.com',
      username: 'target',
      ledger_amount: 0,
      credits: 0,
      subscription_assignments: 1,
      validity_days: 30,
      group_id: 7,
      group_name: 'OpenAI Pro',
      platform: 'openai',
      reference_type: 'user_subscription',
      reference_id: '99',
      occurred_at: '2026-05-30T02:00:00Z',
    },
    {
      source: 'admin_balance_adjustment',
      user_id: 11,
      email: 'target@example.com',
      username: 'target',
      ledger_amount: 25,
      credits: 12500,
      currency: 'USD',
      subscription_assignments: 0,
      operator_id: 1,
      operator_email: 'admin@example.com',
      reference_type: 'redeem_code',
      reference_id: '66',
      occurred_at: '2026-05-30T01:00:00Z',
    },
  ],
}

describe('DepositStatsPanel', () => {
  it('renders admin deposit and automatic package recipient statistics', () => {
    const wrapper = mount(DepositStatsPanel, { props: { stats } })
    const text = wrapper.text()

    expect(text).toContain('Deposit / package grants')
    expect(text).toContain('2')
    expect(text).toContain('$25.00')
    expect(text).toContain('¥70.00')
    expect(text).toContain('12,500')
    expect(text).toContain('Admin balance deposit')
    expect(text).toContain('Automatic package grant')
    expect(text).toContain('target@example.com')
    expect(text).toContain('OpenAI Pro')
    expect(text).toContain('admin@example.com')
  })
})
