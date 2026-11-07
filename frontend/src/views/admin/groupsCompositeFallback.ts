import type { CompositeModelRouteInput, CompositeRouteFallbackTarget } from '@/types'

type Route = Pick<CompositeModelRouteInput,
  'public_model' | 'match_type' | 'target_platform' | 'upstream_model' | 'fallback_targets' | 'endpoint'>

function modelClass(platform: string, model: string): string {
  const id = model.trim().toLowerCase()
  if (platform === 'opencode_go') {
    if (['deepseek-v4.1-flash', 'deepseek-v4-flash'].includes(id)) return 'deepseek-flash'
    if (id === 'deepseek-v4-pro') return 'deepseek-pro'
  }
  if (platform === 'deepseek') {
    if (['deepseek-flash', 'deepseek-v4-flash'].includes(id)) return 'deepseek-flash'
    if (id === 'deepseek-v4-pro') return 'deepseek-pro'
  }
  if (['opencode_go', 'minimax'].includes(platform) &&
      ['minimax-m2.5', 'minimax-m2.7', 'minimax-m3'].includes(id)) return id
  return ''
}

export function compositeFallbackModelClass(route: Route): string {
  return modelClass(route.target_platform, route.upstream_model?.trim() || route.public_model)
}

export function compositeFallbackPlatform(route: Route): string | null {
  const publicModel = route.public_model.trim().toLowerCase()
  const model = compositeFallbackModelClass(route)
  if (route.match_type !== 'exact' || !['any', 'messages', 'responses', 'chat_completions'].includes(route.endpoint) ||
      /free|\*|(^|\/)(glm|kimi|mimo)[-\d]/i.test(publicModel) ||
      (publicModel.includes('flash') && model !== 'deepseek-flash') ||
      (publicModel.startsWith('deepseek') && publicModel.includes('pro') && model !== 'deepseek-pro')) return null
  if (route.target_platform === 'opencode_go' && model.startsWith('deepseek-')) return 'deepseek'
  if (route.target_platform === 'minimax' && model.startsWith('minimax-')) return 'opencode_go'
  return null
}

export function compositeFallbackDefaultModelID(route: Route): string {
  const platform = compositeFallbackPlatform(route)
  const model = compositeFallbackModelClass(route)
  if (platform === 'deepseek') {
    if (model === 'deepseek-pro') return 'deepseek-v4-pro'
    if (model === 'deepseek-flash') return 'deepseek-flash'
  }
  if (platform === 'opencode_go' && model.startsWith('minimax-')) return model
  return ''
}

export function validateCompositeFallback(route: Route): string | null {
  const targets = route.fallback_targets ?? []
  if (!targets.length) return null
  const platform = compositeFallbackPlatform(route)
  if (!platform || targets.length > 1) return 'fallbackPolicyError'
  const primaryModel = compositeFallbackModelClass(route)
  for (const target of targets) {
    if (!target.upstream_model.trim()) return 'fallbackModelRequired'
    if (target.target_platform !== platform ||
        modelClass(target.target_platform, target.upstream_model) !== primaryModel) return 'fallbackPolicyError'
  }
  return null
}

export function copyCompositeFallbackTargets(
  targets?: CompositeRouteFallbackTarget[] | null
): CompositeRouteFallbackTarget[] {
  return (targets ?? []).map(target => ({
    target_platform: target.target_platform,
    upstream_model: target.upstream_model.trim()
  }))
}
