export interface AgentComposerKeyEvent {
  key: string
  keyCode?: number
  altKey: boolean
  ctrlKey: boolean
  metaKey: boolean
  shiftKey: boolean
  isComposing: boolean
}

export function shouldSendAgentComposer(event: AgentComposerKeyEvent, compositionActive: boolean): boolean {
  if (event.key !== 'Enter') return false
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return false
  if (compositionActive || event.isComposing || event.keyCode === 229) return false
  return true
}
