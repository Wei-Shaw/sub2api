import { describe, expect, it } from 'vitest'
import { shouldSendAgentComposer, type AgentComposerKeyEvent } from '../agentComposer'

function keyEvent(overrides: Partial<AgentComposerKeyEvent> = {}): AgentComposerKeyEvent {
  return {
    key: 'Enter',
    keyCode: 13,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    isComposing: false,
    ...overrides
  }
}

describe('Agent composer keyboard handling', () => {
  it('sends on an unmodified Enter after composition has ended', () => {
    expect(shouldSendAgentComposer(keyEvent(), false)).toBe(true)
  })

  it('does not send while the browser reports IME composition', () => {
    expect(shouldSendAgentComposer(keyEvent({ isComposing: true }), false)).toBe(false)
  })

  it('does not send while the component composition state is active', () => {
    expect(shouldSendAgentComposer(keyEvent(), true)).toBe(false)
  })

  it('supports the legacy WebKit IME keyCode fallback', () => {
    expect(shouldSendAgentComposer(keyEvent({ keyCode: 229 }), false)).toBe(false)
  })

  it('keeps Shift+Enter and modified Enter available without sending', () => {
    expect(shouldSendAgentComposer(keyEvent({ shiftKey: true }), false)).toBe(false)
    expect(shouldSendAgentComposer(keyEvent({ ctrlKey: true }), false)).toBe(false)
    expect(shouldSendAgentComposer(keyEvent({ metaKey: true }), false)).toBe(false)
  })
})
