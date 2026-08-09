import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

function messageAt(messages: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((value, key) => {
    if (!value || typeof value !== 'object') return undefined
    return (value as Record<string, unknown>)[key]
  }, messages)
}

describe('admin Web3 deposit messages', () => {
  const keys = [
    'admin.web3Deposits.runtime.title',
    'admin.web3Deposits.runtime.lag',
    'admin.web3Deposits.runtime.scannerLag',
    'admin.web3Deposits.runtime.finalizerLag',
    'admin.web3Deposits.runtime.heights',
    'admin.web3Deposits.stats.manualReview',
    'admin.web3Deposits.stats.failed',
    'admin.web3Deposits.filters.allStatuses',
    'admin.web3Deposits.filters.transactionOrAddress',
    'admin.web3Deposits.filters.search',
    'admin.web3Deposits.filters.boundedRescan',
    'admin.web3Deposits.columns.idUser',
    'admin.web3Deposits.columns.amount',
    'admin.web3Deposits.columns.status',
    'admin.web3Deposits.columns.transaction',
    'admin.web3Deposits.columns.actions',
    'admin.web3Deposits.user',
    'admin.web3Deposits.actions.approve',
    'admin.web3Deposits.actions.ignore',
    'admin.web3Deposits.actions.retry',
    'admin.web3Deposits.dialogs.approveConfirm',
    'admin.web3Deposits.dialogs.ignorePrompt',
    'admin.web3Deposits.dialogs.retryConfirm',
    'admin.web3Deposits.dialogs.rescanTitle',
    'admin.web3Deposits.dialogs.fromBlock',
    'admin.web3Deposits.dialogs.toBlock',
    'admin.web3Deposits.dialogs.cancel',
    'admin.web3Deposits.dialogs.rescan',
    'admin.web3Deposits.messages.loadFailed',
    'admin.web3Deposits.messages.operationCompleted',
    'admin.web3Deposits.messages.operationFailed',
    ...[
      'detected', 'confirming', 'readyToCredit', 'crediting', 'credited',
      'belowMinimum', 'manualReview', 'orphaned', 'failed', 'ignored', 'unknown',
    ].map(status => `admin.web3Deposits.statuses.${status}`),
    ...['disabled', 'standby', 'leader', 'unhealthy', 'stopped', 'unknown'].map(
      state => `admin.web3Deposits.runtimeStates.${state}`,
    ),
    ...[
      'amountAboveAutoCreditLimit', 'amountExceedsPlatformBalance', 'userMissing',
      'userDeleted', 'userInactive', 'depositAddressMissing', 'depositAddressDisabled',
      'depositAddressUserMismatch', 'depositAddressMismatch', 'canonicalBlockMissing',
      'canonicalBlockHashMismatch', 'transactionReceiptMissing', 'transactionReceiptFailed',
      'transactionReceiptBlockHashMismatch', 'transferLogMissing', 'transferTokenMismatch',
      'transferDestinationMismatch', 'transferAmountMismatch',
    ].map(reason => `admin.web3Deposits.reasons.${reason}`),
  ]

  it.each(keys)('provides English and Chinese messages for %s', (key) => {
    expect(messageAt(en, key)).toEqual(expect.any(String))
    expect(messageAt(zh, key)).toEqual(expect.any(String))
  })
})
