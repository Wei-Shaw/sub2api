import { describe, expect, it } from 'vitest'
import type { PromptAuditConfig } from '../types'
import {
  buildUpdateRequest,
  configToDraft,
  draftFingerprint,
  emptyEventFilters,
  eventFilterPayload,
  hasExplicitDeleteRange,
  SCANNER_CATALOG,
} from '../viewModel'

const config = (): PromptAuditConfig => ({
  enabled: true,
  blocking_enabled: false,
  blocking_latest_turn_only: false,
  keyword_blocking_enabled: false,
  ai_blocking_enabled: false,
  store_pass_events: false,
  pre_hash_check_enabled: false,
  blocked_keywords: ['secret'],
  keyword_blocking_mode: 'ai_only',
  effective_mode: 'async_audit',
  strategy: 'priority',
  worker_count: 4,
  queue_capacity: 100,
  scanners: SCANNER_CATALOG.map((item) => item.id),
  all_groups: true,
  group_ids: [],
  proxy_id: null,
  endpoints: [{
    id: 'guard-1', name: 'Guard One', protocol: 'openai_compatible', base_url: 'http://127.0.0.1:8000',
    model: 'sileader/qwen3guard:0.6b', timeout_ms: 3000, input_limit: 4000, enabled: true,
    has_token: true, token_status: 'configured',
  }],
  config_version: 7,
  updated_at: '2026-07-16T00:00:00Z',
  updated_by: 1,
  change_summary: '{}',
})

describe('Prompt Audit view model', () => {
  it('normalizes legacy null collections from the public config', () => {
    const legacy = { ...config(), group_ids: null, scanners: null, endpoints: null } as unknown as PromptAuditConfig
    expect(configToDraft(legacy)).toMatchObject({ group_ids: [], scanners: [], endpoints: [] })
  })

  it('defaults legacy keyword settings to AI-only without changing the save payload shape', () => {
    const legacy = { ...config(), keyword_blocking_enabled: undefined, ai_blocking_enabled: undefined, blocked_keywords: undefined, keyword_blocking_mode: undefined } as unknown as PromptAuditConfig
    const draft = configToDraft(legacy)
    expect(draft.blocked_keywords).toEqual([])
    expect(draft.keyword_blocking_mode).toBe('ai_only')
    expect(draft.keyword_blocking_enabled).toBe(false)
    expect(draft.ai_blocking_enabled).toBe(false)
    expect(buildUpdateRequest(draft)).toMatchObject({ blocked_keywords: [], keyword_blocking_mode: 'ai_only', keyword_blocking_enabled: false, ai_blocking_enabled: false })
  })

  it('includes the selected keyword strategy and independent list in the update payload', () => {
    const draft = configToDraft(config())
    draft.keyword_blocking_mode = 'keyword_and_ai'
    draft.blocked_keywords = ['jailbreak', 'secret']
    draft.keyword_blocking_enabled = true
    draft.ai_blocking_enabled = false
    expect(buildUpdateRequest(draft)).toMatchObject({
      blocked_keywords: ['jailbreak', 'secret'],
      keyword_blocking_mode: 'keyword_and_ai',
      blocking_enabled: true,
      keyword_blocking_enabled: true,
      ai_blocking_enabled: false,
    })
	})

  it('includes the narrow synchronous audit scope in the update payload', () => {
    const draft = configToDraft(config())
    draft.blocking_latest_turn_only = true
    expect(buildUpdateRequest(draft)).toMatchObject({ blocking_latest_turn_only: true })
  })

	 it('preserves the managed proxy selection in the update payload', () => {
    const draft = configToDraft({ ...config(), proxy_id: 18 })
    expect(draft.proxy_id).toBe(18)
    expect(buildUpdateRequest(draft).proxy_id).toBe(18)
  })

  it('models all nine official input scanners', () => {
    expect(SCANNER_CATALOG).toHaveLength(9)
    expect(SCANNER_CATALOG.map((item) => item.id)).toContain('suicide_and_self_harm')
  })

  it('keeps, replaces, or explicitly clears a saved token without copying plaintext from the server', () => {
    const draft = configToDraft(config())
    expect(draft.endpoints[0].token).toBe('')
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: undefined, clear_token: false })

    draft.endpoints[0].token = 'temporary-canary-token'
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: 'temporary-canary-token', clear_token: false })

    draft.endpoints[0].token = ''
    draft.endpoints[0].clear_token = true
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({ token: undefined, clear_token: true })
  })

  it('preserves a moderation endpoint protocol and supplies its default model', () => {
    const draft = configToDraft({
      ...config(),
      endpoints: [{ ...config().endpoints[0], protocol: 'openai_moderation', model: '' }],
    })
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({
      protocol: 'openai_moderation',
      model: 'omni-moderation-latest',
    })
  })

  it('preserves a Nemotron endpoint protocol and supplies its default model', () => {
    const draft = configToDraft({
      ...config(),
      endpoints: [{ ...config().endpoints[0], protocol: 'nemotron_content_safety', model: '' }],
    })
    expect(buildUpdateRequest(draft).endpoints[0]).toMatchObject({
      protocol: 'nemotron_content_safety',
      model: 'nemotron-3.5-content-safety-free',
    })
  })

  it('uses the OpenRouter Nemotron model ID for OpenRouter endpoints', () => {
    const draft = configToDraft({
      ...config(),
      endpoints: [{
        ...config().endpoints[0],
        protocol: 'nemotron_content_safety',
        base_url: 'https://openrouter.ai/api',
        model: 'nemotron-3.5-content-safety-free',
      }],
    })
    expect(buildUpdateRequest(draft).endpoints[0].model).toBe('nvidia/nemotron-3.5-content-safety:free')
  })

  it('tracks dirty state from the full normalized save payload', () => {
    const original = configToDraft(config())
    const changed = configToDraft(config())
    expect(draftFingerprint(changed)).toBe(draftFingerprint(original))
    changed.queue_capacity += 1
    expect(draftFingerprint(changed)).not.toBe(draftFingerprint(original))
  })

  it('requires a valid explicit range and sends canonical ISO timestamps for filter deletion', () => {
    const filters = emptyEventFilters()
    expect(hasExplicitDeleteRange(filters)).toBe(false)
    filters.start_at = '2026-07-15T10:00'
    filters.end_at = '2026-07-16T10:00'
    filters.group_id = '9'
    filters.audit_type = 'keyword'
    expect(hasExplicitDeleteRange(filters)).toBe(true)
    expect(eventFilterPayload(filters)).toMatchObject({
      group_id: 9,
      audit_type: 'keyword',
      start_at: new Date(filters.start_at).toISOString(),
      end_at: new Date(filters.end_at).toISOString(),
    })
  })
})
