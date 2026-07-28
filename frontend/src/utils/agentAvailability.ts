export const AI_AGENT_AVAILABILITY_EVENT = 'ai-agent-availability-changed'

const storageKey = 'ai_agent_enabled'

export function readCachedAIAgentEnabled(): boolean {
  try {
    const cached = localStorage.getItem(storageKey)
    return cached === 'true'
  } catch {
    return false
  }
}

export function cacheAIAgentEnabled(enabled: boolean): void {
  try {
    localStorage.setItem(storageKey, String(enabled))
  } catch {
    // Storage can be unavailable in restricted browser contexts.
  }
}

export function announceAIAgentEnabled(enabled: boolean): void {
  cacheAIAgentEnabled(enabled)
  window.dispatchEvent(new CustomEvent(AI_AGENT_AVAILABILITY_EVENT, { detail: { enabled } }))
}
