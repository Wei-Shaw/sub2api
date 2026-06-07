import { afterEach, describe, expect, it } from 'vitest'
import { computed, ref, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

import { useRechargePromoDot } from '@/composables/useRechargePromoDot'
import type { RechargePromo } from '@/types/payment'

/**
 * 用一个简单的宿主组件挂载 composable，这样 onMounted/onBeforeUnmount 才会触发
 * （直接调用 setup 里的 hook 会因为没有 vue 生命周期上下文而报警告）。
 */
function harness(userId: number | null, promo: RechargePromo | null) {
  const userIdRef = ref<number | null>(userId)
  const promoRef = ref<RechargePromo | null>(promo)
  let api: ReturnType<typeof useRechargePromoDot> | null = null
  const wrapper = mount({
    setup() {
      api = useRechargePromoDot({
        userId: computed(() => userIdRef.value),
        promo: computed(() => promoRef.value),
      })
      return () => null
    },
  })
  return {
    api: api as unknown as ReturnType<typeof useRechargePromoDot>,
    wrapper,
    userIdRef,
    promoRef,
  }
}

const samplePromo: RechargePromo = {
  enabled: true,
  tiers: [{ min_amount: 100, bonus_rate: 0.05 }],
  version: 'abc123',
}

describe('useRechargePromoDot', () => {
  afterEach(() => {
    localStorage.clear()
  })

  it('shows red dot when promo is enabled and user has not seen this version', () => {
    const { api, wrapper } = harness(42, samplePromo)
    expect(api.shouldShow.value).toBe(true)
    wrapper.unmount()
  })

  it('hides the dot once dismissed (persisted via localStorage)', async () => {
    const { api, wrapper } = harness(42, samplePromo)
    expect(api.shouldShow.value).toBe(true)
    api.dismiss()
    await nextTick()
    expect(api.shouldShow.value).toBe(false)
    expect(localStorage.getItem('recharge-promo-seen:42:abc123')).toBe('1')
    wrapper.unmount()
  })

  it('reappears when promo version changes (new campaign)', async () => {
    const { api, wrapper, promoRef } = harness(42, samplePromo)
    api.dismiss()
    await nextTick()
    expect(api.shouldShow.value).toBe(false)

    promoRef.value = { ...samplePromo, version: 'def456' }
    await nextTick()
    expect(api.shouldShow.value).toBe(true)
    wrapper.unmount()
  })

  it('hides for unauthenticated users (no userId → no key)', () => {
    const { api, wrapper } = harness(null, samplePromo)
    expect(api.shouldShow.value).toBe(false)
    wrapper.unmount()
  })

  it('hides when promo is disabled', () => {
    const { api, wrapper } = harness(42, { ...samplePromo, enabled: false })
    expect(api.shouldShow.value).toBe(false)
    wrapper.unmount()
  })

  it('hides when promo is null (out of window or not configured)', () => {
    const { api, wrapper } = harness(42, null)
    expect(api.shouldShow.value).toBe(false)
    wrapper.unmount()
  })

  // 同 tab 内可能同时存在多个实例：如 PaymentView 内的 tab/preset 红点和侧边栏 /purchase 红点。
  // 之前每个实例各自维护 tick，A 实例 dismiss 后 B 实例不会刷新（storage 事件只跨 tab 触发）。
  // 这里验证：在同一 tab 内任一实例 dismiss 后，其他实例的 shouldShow 立刻翻为 false。
  it('propagates dismiss across instances in the same tab', async () => {
    const a = harness(42, samplePromo)
    const b = harness(42, samplePromo)
    expect(a.api.shouldShow.value).toBe(true)
    expect(b.api.shouldShow.value).toBe(true)

    a.api.dismiss()
    await nextTick()

    expect(a.api.shouldShow.value).toBe(false)
    expect(b.api.shouldShow.value).toBe(false)

    a.wrapper.unmount()
    b.wrapper.unmount()
  })
})
