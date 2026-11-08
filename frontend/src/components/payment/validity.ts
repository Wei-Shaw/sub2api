import type { SubscriptionPlan } from '@/types/payment'

type TranslateFn = (key: string) => string

/**
 * 用户侧套餐有效期后缀（"$9.9 / 月"、"$9.9 / 30天"）。
 *
 * 管理端表单保存的单位是复数（days/weeks/months），而数据库默认值与部分
 * 历史数据是单数（day）。后端计费 psComputeValidityDays 对 week/weeks、
 * month/months 分别按 ×7、×30 天换算，其余一律按天。此前用户侧只匹配
 * 单数 'month'，管理端存的 'months' 永远落进默认分支，「1 个月」的套餐
 * 被显示成「1天」（#4607）；'weeks' 则会显示成周数的「N天」。
 *
 * 这里把单位归一化后与计费语义一一对应，保证用户看到的周期与实际
 * 生效周期一致。
 */
export function planValiditySuffix(
  plan: Pick<SubscriptionPlan, 'validity_days' | 'validity_unit'>,
  t: TranslateFn,
): string {
  const unit = String(plan.validity_unit || 'day').trim().toLowerCase()
  const base = unit.endsWith('s') ? unit.slice(0, -1) : unit
  const days = plan.validity_days
  if (base === 'month') {
    return days === 1 ? t('payment.perMonth') : `${days}${t('payment.months')}`
  }
  if (base === 'week') {
    return `${days}${t('payment.weeks')}`
  }
  // 其余单位（含数据库默认的 day 与未知值）后端一律按天计费，展示保持一致。
  return `${days}${t('payment.days')}`
}
