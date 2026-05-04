import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const testDir = dirname(fileURLToPath(import.meta.url))
const viewPath = (name: string) => resolve(testDir, `../${name}.vue`)

describe('secondary-development route shells', () => {
  it('models route uses the existing available-channel data source without remaining a placeholder shell', () => {
    const source = readFileSync(viewPath('ModelsView'), 'utf8')

    expect(source).toContain("import userChannelsAPI, { type UserAvailableChannel")
    expect(source).toContain('data-testid="platform-filter"')
    expect(source).toContain('class="model-channel-card')
    expect(source).toContain('class="model-channel-count')
    expect(source).toContain('formatTokenComparison')
    expect(source).toContain('getOfficialModelPricing')
    expect(source).toContain('userChannelsAPI.getAvailable()')
    expect(source).not.toContain("import AvailableChannelsView from '@/views/user/AvailableChannelsView.vue'")
    expect(source).not.toContain("import AvailableChannelsTable from '@/components/channels/AvailableChannelsTable.vue'")
  })

  it('recharge-subscription route keeps existing payment functionality reachable', () => {
    const source = readFileSync(viewPath('RechargeSubscriptionView'), 'utf8')

    expect(source).toContain("import PaymentView from '@/views/user/PaymentView.vue'")
    expect(source).toContain('<PaymentView embedded layout-mode="stacked" hide-tabs @payment-completed="refreshOrders" />')
    expect(source).toContain('<UserOrdersView ref="ordersViewRef" embedded />')
  })
})
