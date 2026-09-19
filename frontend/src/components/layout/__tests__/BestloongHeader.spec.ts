import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const testDir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(testDir, '../AppHeader.vue'), 'utf8')
const homeSource = readFileSync(resolve(testDir, '../../../views/HomeView.vue'), 'utf8')

describe('Bestloong top navigation', () => {
  it('keeps the announcement bell', () => {
    expect(headerSource).toContain('<AnnouncementBell v-if="user" />')
  })

  it('removes model plaza links from both authenticated and public headers', () => {
    expect(headerSource).not.toContain('/model-plaza')
    expect(homeSource).not.toContain('/model-plaza')
  })

  it('removes the subscription progress widget from the customer header', () => {
    expect(headerSource).not.toContain('SubscriptionProgressMini')
  })
})
