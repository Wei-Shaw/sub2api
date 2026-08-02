import { describe, expect, it, vi } from 'vitest'

import { STATUS_OPERATIONAL } from '@/constants/channelMonitor'
import { useChannelMonitorFormat } from '../useChannelMonitorFormat'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('useChannelMonitorFormat', () => {
  it('renders an operational channel as blue instead of neutral gray', () => {
    const { statusBadgeClass } = useChannelMonitorFormat()

    expect(statusBadgeClass(STATUS_OPERATIONAL)).toContain('bg-blue-100')
    expect(statusBadgeClass(STATUS_OPERATIONAL)).toContain('text-blue-700')
  })
})
