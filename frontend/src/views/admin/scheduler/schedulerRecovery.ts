import type { OpsModelCooldown } from '@/api/admin/ops'

export function affectedAccountCount(rows: OpsModelCooldown[]): number {
  return new Set(rows.map(row => row.account_id)).size
}

export function recoveryStateLabel(row: OpsModelCooldown, locale: string): string {
  const states: Record<string, [string, string]> = {
    cooldown: ['临时冷却', 'Cooling down'],
    awaiting_probe: ['等待试探', 'Awaiting probe'],
    probing: ['试探中', 'Probing'],
    recovering: ['恢复验证中', 'Verifying recovery']
  }
  return (states[row.state || 'cooldown'] || states.cooldown)[locale.startsWith('zh') ? 0 : 1]
}

export function recoveryReasonLabel(row: OpsModelCooldown, locale: string): string {
  const zh = locale.startsWith('zh')
  if (row.reason?.startsWith('anthropic_')) return zh ? '上游流式响应失败（仅此模型）' : 'Upstream stream failure (this model only)'
  if (row.reason?.startsWith('cross_model_')) return zh ? '多个模型持续失败' : 'Persistent failures across models'
  if (row.reason === 'upstream_connection') return zh ? '上游连接失败' : 'Upstream connection failed'
  if (row.reason === 'upstream_transport') return zh ? '上游传输中断' : 'Upstream transport interrupted'
  if (row.reason === 'recovery_probe_failed') return zh ? '恢复试探失败' : 'Recovery probe failed'
  const status = /^upstream_(\d{3})$/.exec(row.reason || '')
  if (status) return `${zh ? '上游' : 'Upstream'} HTTP ${status[1]}`
  return zh ? '上游临时错误' : 'Transient upstream error'
}
