import type { CompositeModelRouteInput, CompositeRouteFallbackTarget } from '@/types'

type Route = Pick<CompositeModelRouteInput,
  'public_model' | 'match_type' | 'target_platform' | 'upstream_model' | 'fallback_targets' | 'endpoint'>

function modelClass(platform: string, model: string): string {
  const id = model.trim().toLowerCase()
  if (platform === 'openai') {
    if (['deepseek-v4.1-flash', 'deepseek-v4-flash', 'deepseek-flash'].includes(id)) return 'deepseek-flash'
    if (id === 'deepseek-v4-pro') return 'deepseek-pro'
    if (['minimax-m2.5', 'minimax-m2.7', 'minimax-m3', 'glm-5.3', 'glm-5.3-flash'].includes(id)) return id
  }
  if (platform === 'opencode_go') {
    if (['deepseek-v4.1-flash', 'deepseek-v4-flash'].includes(id)) return 'deepseek-flash'
    if (id === 'deepseek-v4-pro') return 'deepseek-pro'
    if (['minimax-m2.5', 'minimax-m2.7', 'minimax-m3', 'glm-5.3', 'glm-5.3-flash'].includes(id)) return id
  }
  if (platform === 'deepseek') {
    if (['deepseek-flash', 'deepseek-v4-flash'].includes(id)) return 'deepseek-flash'
    if (id === 'deepseek-v4-pro') return 'deepseek-pro'
  }
  if (['opencode_go', 'minimax'].includes(platform) &&
      ['minimax-m2.5', 'minimax-m2.7', 'minimax-m3'].includes(id)) return id
  if (['zhipu', 'opencode_go', 'openai'].includes(platform) && ['glm-5.3', 'glm-5.3-flash'].includes(id)) return id
  return ''
}

export function compositeFallbackModelClass(route: Route): string {
  return modelClass(route.target_platform, route.upstream_model?.trim() || route.public_model)
}

export function compositeFallbackPlatforms(route: Route): string[] {
  const publicModel = route.public_model.trim().toLowerCase()
  const model = compositeFallbackModelClass(route)
  if (route.match_type !== 'exact' || !['any', 'messages', 'responses', 'chat_completions'].includes(route.endpoint) ||
      /free|\*/i.test(publicModel) ||
      (model.startsWith('deepseek-') && publicModel.includes('flash') && model !== 'deepseek-flash') ||
      (model.startsWith('glm-') && publicModel.startsWith('glm') && publicModel.includes('flash') !== model.endsWith('-flash')) ||
      (publicModel.startsWith('deepseek') && publicModel.includes('pro') && model !== 'deepseek-pro')) return []
  if (model.startsWith('deepseek-') && route.target_platform === 'opencode_go') return ['opencode_go', 'deepseek', 'openai']
  if (model.startsWith('minimax-') && route.target_platform === 'minimax') return ['minimax', 'opencode_go', 'openai']
  if (model.startsWith('glm-') && route.target_platform === 'zhipu') return ['zhipu', 'opencode_go', 'openai']
  return []
}

export function compositeFallbackDefaultModelID(route: Route, platform: string): string {
  if (!compositeFallbackPlatforms(route).includes(platform)) return ''
  const model = compositeFallbackModelClass(route)
  if (platform === 'deepseek') {
    if (model === 'deepseek-pro') return 'deepseek-v4-pro'
    if (model === 'deepseek-flash') return 'deepseek-flash'
  }
  if (platform === 'opencode_go' && (model.startsWith('minimax-') || model.startsWith('glm-'))) return model
  if (platform === 'openai' && model) {
    if (model === 'deepseek-pro') return 'deepseek-v4-pro'
    return model
  }
  return ''
}

export function validateCompositeFallback(route: Route): string | null {
  const targets = route.fallback_targets ?? []
  if (!targets.length) return null
  const platforms = compositeFallbackPlatforms(route)
  if (platforms.length === 0 || targets.length > platforms.length - 1) return 'fallbackPolicyError'
  const primaryModel = compositeFallbackModelClass(route)
  const seen = new Set([route.target_platform])
  for (const target of targets) {
    if (!target.upstream_model.trim()) return 'fallbackModelRequired'
    if (!platforms.includes(target.target_platform) || seen.has(target.target_platform) ||
        modelClass(target.target_platform, target.upstream_model) !== primaryModel) return 'fallbackPolicyError'
    seen.add(target.target_platform)
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
