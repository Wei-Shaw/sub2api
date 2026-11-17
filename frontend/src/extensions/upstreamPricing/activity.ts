import { onBeforeUnmount, ref, watch } from 'vue'
import { upstreamPricingAPI } from './api'
import type { ActivityState } from '.'

// This reads local snapshots only. Station polling belongs to the server job.
export function useUpstreamActivity(account: () => number, refresh: () => unknown = () => 0) {
  const state = ref<ActivityState | null>(null)
  const failed = ref(false)
  let controller: AbortController | undefined
  let generation = 0
  let active = true
  async function load() {
    const id = account()
    const current = ++generation
    controller?.abort()
    if (!id) { state.value = null; failed.value = false; return }
    controller = new AbortController()
    try {
      const value = await upstreamPricingAPI.activity(id, controller.signal)
      if (current === generation && active) { state.value = value; failed.value = false }
    } catch { if (current === generation && active) failed.value = true }
  }
  watch([account, refresh], () => { state.value = null; void load() }, { immediate: true })
  const timer = setInterval(() => { void load() }, 60000)
  onBeforeUnmount(() => { active = false; generation++; clearInterval(timer); controller?.abort() })
  return { state, failed }
}

export function walletDetails(state: ActivityState | null, failed: boolean) {
  if (failed) return '本机余额快照暂不可用'
  if (!state?.configured) return '先在账号“上游定价”保存面板读取凭证并应用配置'
  return [
    state.wallet_checked_at && `钱包更新时间：${new Date(state.wallet_checked_at).toLocaleString()}`,
    state.wallet_error,
    '钱包为上游站点账号余额，同一站点钱包可能被多个调用账号共用；不等同于本地余额或 API Key 额度。',
  ].filter(Boolean).join('\n')
}
