import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')
const read = (path: string) => readFileSync(resolve(root, path), 'utf8')

describe('AI Agent admin integration', () => {
  it('registers the admin route and sidebar entry', () => {
    expect(read('router/index.ts')).toContain("path: '/admin/ai-agent'")
    expect(read('components/layout/AppSidebar.vue')).toContain("path: '/admin/ai-agent'")
  })

  it('supports a backend-enforced availability switch and hides disabled navigation', () => {
    const view = read('views/admin/AIAgentView.vue')
    const sidebar = read('components/layout/AppSidebar.vue')
    const settings = read('views/admin/SettingsView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(api).toContain('enabled: boolean')
    expect(view).toContain('v-model="settingsForm.enabled"')
    expect(view).toContain('config && !config.enabled')
    expect(view).toContain('announceAIAgentEnabled(config.value.enabled)')
    expect(sidebar).toContain('ref(readCachedAIAgentEnabled())')
    expect(sidebar).toContain('featureFlag: () => aiAgentEnabled.value')
    expect(sidebar).toContain("aiAgentAPI.getConfig()")
    expect(settings).toContain('updateAIAgentAvailability')
    expect(settings).toContain(':model-value="aiAgentEnabled"')
  })

  it('keeps writes behind an explicit confirmation surface', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain('confirmPending')
    expect(view).toContain('cancelPending')
    expect(view).toContain("t('admin.aiAgent.pendingTitle')")
  })

  it('does not submit unfinished IME composition text', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain('@compositionstart="composerCompositionActive = true"')
    expect(view).toContain('@compositionend="composerCompositionActive = false"')
    expect(view).toContain('@keydown="handleComposerKeydown"')
    expect(view).not.toContain('@keydown.enter.exact.prevent="sendMessage"')
  })

  it('keeps the composer anchored while messages scroll independently', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain('ref="messagePane" class="min-h-0 flex-1')
    expect(view).toContain('overflow-y-auto overscroll-contain')
    expect(view).toContain('fixed inset-x-0 bottom-0 z-20 flex-none')
    expect(view).toContain('lg:sticky lg:inset-x-auto')
  })

  it('shows a stable, reduced-motion-aware AI thinking animation', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain('agent-thinking-bars')
    expect(view).toContain('agent-thinking-dots')
    expect(view).toContain('@media (prefers-reduced-motion: reduce)')
  })

  it('offers all model protocols and manual thinking configuration', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain("value: 'chat_completions'")
    expect(view).toContain("value: 'responses'")
    expect(view).toContain("value: 'messages'")
    expect(view).toContain('v-model="settingsForm.thinking_mode"')
    expect(api).toContain("'chat_completions' | 'responses' | 'messages'")
  })

  it('configures token-budgeted context and automatic compression', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain('v-model.trim="settingsForm.context_window"')
    expect(view).toContain("context_window: '150k'")
    expect(api).toContain('context_window_tokens: number')
    expect(view).toContain("case 'context_compressed'")
    expect(view).toContain("config?.streaming ? t('admin.aiAgent.streamingMode')")
    expect(api).toContain('streaming: boolean')
    expect(api).toContain('response_cache: boolean')
    expect(view).toContain("t('admin.aiAgent.responseCache')")
    expect(view).toContain("t('admin.aiAgent.cacheHit'")
  })

  it('supports persistent history, process display, and stopping a running response', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain("process_display: 'off' | 'compact' | 'full'")
    expect(view).toContain("busy ? 'stop' : 'arrowUp'")
    expect(view).toContain('schedulePoll')
    expect(api).toContain("'/admin/ai-agent/conversations'")
    expect(api).toContain("'/admin/ai-agent/stop'")
  })

  it('does not replay persisted conversation errors when opening history', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain("previousStatus === 'running' || previousStatus === 'stopping'")
    expect(view).toContain("session.conversation.status === 'error'")
    expect(view).toContain('notifyNewFailure && session.error')
  })

  it('routes sensitive credentials through confirmation and step-up', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain('sensitiveStepUp.run')
    expect(view).toContain('pending.sensitive_fields')
    expect(api).toContain('confirm-sensitive')
  })

  it('reviews batch and dependency plans in a dedicated confirmation dialog', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain('showPlanConfirmation && !!pending?.plan')
    expect(view).toContain('pending.plan.nodes')
    expect(view).toContain('planFailurePolicyLabel')
    expect(view).toContain('node.depends_on')
    expect(api).toContain('interface AIAgentExecutionPlan')
    expect(api).toContain("'rollback_on_failure'")
  })

  it('previews deterministic rollback impact and offers supervised Agent recovery', () => {
    const view = read('views/admin/AIAgentView.vue')
    const api = read('api/admin/aiAgent.ts')
    expect(view).toContain('showRollbackConfirmation && !!selectedRollback')
    expect(view).toContain('showPlanConfirmation && !!pending?.plan && confirmationReady')
    expect(view).toContain('v-if="pending && confirmationReady"')
    expect(view).toContain('rollbackDiffGroups.length')
    expect(view).toContain('rollbackDiffLines')
    expect(view).toContain('rollbackTargetMeta')
    expect(view).toContain('rollbackDiffGroups')
    expect(view).toContain('rollbackGroupTargetName')
    expect(view).toContain("line.kind === 'remove' ? '−' : '+'")
    expect(view).toContain('assistRollback')
    expect(view).not.toContain('@click="runRollback(rollback.id)"')
    expect(api).toContain('/preview')
    expect(api).toContain('/assist')
    expect(api).toContain('/confirm-sensitive')
  })

  it('renders sanitized Markdown for assistant messages while keeping user input as text', () => {
    const view = read('views/admin/AIAgentView.vue')
    expect(view).toContain('v-html="renderAgentMarkdown(messageContent(item.message))"')
    expect(view).toContain("item.message.role === 'user'")
    expect(view).toContain("import { renderAgentMarkdown } from '@/utils/agentMarkdown'")
  })

  it('uses the dedicated authenticated admin API', () => {
    const api = read('api/admin/aiAgent.ts')
    expect(api).toContain("'/admin/ai-agent/chat'")
    expect(api).toContain('/admin/ai-agent/actions/')
    expect(api).not.toContain('x-api-key')
  })
})
