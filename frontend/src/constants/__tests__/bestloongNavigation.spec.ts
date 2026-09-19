import { describe, expect, it } from 'vitest'
import {
  BESTLOONG_USER_NAV_ITEMS,
  isBestloongHiddenCustomerPath,
} from '@/constants/bestloongNavigation'

describe('Bestloong customer navigation', () => {
  it('keeps the R1 menu contract in the required order', () => {
    expect(BESTLOONG_USER_NAV_ITEMS.map((item) => item.path)).toEqual([
      '/dashboard',
      '/keys',
      '/usage',
      '/model-plaza',
      '/monitor',
      '/redeem',
      '/balance',
      '/tutorials',
      '/about',
      '/profile',
    ])
  })

  it.each([
    '/subscriptions',
    '/orders/123',
    '/purchase',
    '/payment/result',
    '/available-channels',
    '/affiliate',
    '/batch-image',
    '/custom/legacy-page',
  ])('hides retired customer route %s', (path) => {
    expect(isBestloongHiddenCustomerPath(path)).toBe(true)
  })

  it.each(['/dashboard', '/model-plaza', '/monitor', '/redeem']) (
    'keeps TOB route %s available',
    (path) => {
      expect(isBestloongHiddenCustomerPath(path)).toBe(false)
    },
  )
})
