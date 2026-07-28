import { afterEach, describe, expect, it, vi } from 'vitest'
import { AI_AGENT_AVAILABILITY_EVENT, announceAIAgentEnabled, readCachedAIAgentEnabled } from '@/utils/agentAvailability'

describe('AI Agent availability cache', () => {
  afterEach(() => {
    localStorage.removeItem('ai_agent_enabled')
  })

  it('hides the Agent synchronously when no setting has been cached', () => {
    localStorage.removeItem('ai_agent_enabled')
    expect(readCachedAIAgentEnabled()).toBe(false)
  })

  it('preserves the disabled state across page refreshes', () => {
    localStorage.setItem('ai_agent_enabled', 'false')
    expect(readCachedAIAgentEnabled()).toBe(false)
  })

  it('caches and announces availability changes', () => {
    const listener = vi.fn()
    window.addEventListener(AI_AGENT_AVAILABILITY_EVENT, listener)
    announceAIAgentEnabled(false)
    window.removeEventListener(AI_AGENT_AVAILABILITY_EVENT, listener)

    expect(readCachedAIAgentEnabled()).toBe(false)
    expect(listener).toHaveBeenCalledOnce()
  })
})
