/**
 * 充值赠送活动后台表单的客户端校验器。
 *
 * 与后端 `validateRechargePromo`（backend/internal/service/payment_config_recharge_promo.go）
 * 一一对应，避免无谓 round-trip，同时为 UI 测试提供一个稳定的纯函数靶点。
 *
 * 设计取舍：
 *   - 保留 toast 反馈（`appStore.showError(t(...))`）而非 inline-per-row error，
 *     因为现状对话框形态对 inline error 不友好，留待未来视觉重构再做。
 *   - 返回 i18n key 后缀（不含 namespace 前缀）让调用方拼接，避免在 SFC 内
 *     hardcode 一长串 i18n 路径。
 */

export type RechargePromoFormErrorKey =
  | 'nameRequired'
  | 'tiersRequiredWhenEnabled'
  | 'minAmountInvalid'
  | 'bonusRateOutOfRange'
  | 'tiersNotAscending'
  | 'validUntilBeforeFrom'

export interface RechargePromoFormState {
  name: string
  enabled: boolean
  /** datetime-local input value（"YYYY-MM-DDTHH:mm"），未填为空字符串。 */
  valid_from: string
  valid_until: string
  tiers: Array<{ min_amount: number; bonus_rate: number }>
}

/**
 * 返回首个违反规则的 i18n key 后缀；通过则返回 null。
 *
 * 规则顺序与后端一致：
 *   1. name 必填
 *   2. enabled 时至少 1 档
 *   3. 每档 min_amount > 0、bonus_rate ∈ [0, 1)
 *   4. tiers 按 min_amount 严格升序（不允许重复）
 *   5. valid_from < valid_until（两端都填时才校验）
 */
export function validateRechargePromoForm(
  form: RechargePromoFormState
): RechargePromoFormErrorKey | null {
  if (!form.name.trim()) {
    return 'nameRequired'
  }
  if (form.enabled && form.tiers.length === 0) {
    return 'tiersRequiredWhenEnabled'
  }
  for (const tier of form.tiers) {
    const min = Number(tier.min_amount)
    if (!Number.isFinite(min) || min <= 0) {
      return 'minAmountInvalid'
    }
    const rate = Number(tier.bonus_rate)
    if (!Number.isFinite(rate) || rate < 0 || rate >= 1) {
      return 'bonusRateOutOfRange'
    }
  }
  // 严格升序：与后端一致，min_amount 重复也算违例。
  for (let i = 1; i < form.tiers.length; i++) {
    if (Number(form.tiers[i].min_amount) <= Number(form.tiers[i - 1].min_amount)) {
      return 'tiersNotAscending'
    }
  }
  if (form.valid_from && form.valid_until) {
    const from = new Date(form.valid_from).getTime()
    const until = new Date(form.valid_until).getTime()
    if (Number.isFinite(from) && Number.isFinite(until) && from >= until) {
      return 'validUntilBeforeFrom'
    }
  }
  return null
}
