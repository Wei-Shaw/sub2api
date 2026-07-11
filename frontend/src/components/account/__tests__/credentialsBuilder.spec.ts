import { describe, it, expect REDACTED from 'vitest'
import {
  ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY,
  HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY,
  HEADER_OVERRIDES_CREDENTIAL_KEY,
  applyAntigravityProjectID,
  applyHeaderOverride,
  applyInterceptWarmup,
  applyPlanType,
  buildHeaderOverridesObject,
  buildPlanTypeOptions,
  getHeaderOverrideTemplate,
  isHeaderOverridePlatform,
  planTypeDisplayLabel,
  readPlanType,
  splitHeaderOverridesObject,
  validateHeaderOverrideRows
REDACTED from '../credentialsBuilder'

describe('applyInterceptWarmup', () => {
  it('create + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' REDACTED
    applyInterceptWarmup(creds, true, 'create')
    expect(creds.intercept_warmup_requests).toBe(true)
  REDACTED)

  it('create + enabled=false: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' REDACTED
    applyInterceptWarmup(creds, false, 'create')
    expect('intercept_warmup_requests' in creds).toBe(false)
  REDACTED)

  it('edit + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' REDACTED
    applyInterceptWarmup(creds, true, 'edit')
    expect(creds.intercept_warmup_requests).toBe(true)
  REDACTED)

  it('edit + enabled=false + field exists: should delete the field', () => {
    const creds: Record<string, unknown> = { api_key: 'sk', intercept_warmup_requests: true REDACTED
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  REDACTED)

  it('edit + enabled=false + field absent: should not throw', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' REDACTED
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  REDACTED)

  it('should not affect other fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      base_url: 'url',
      intercept_warmup_requests: true
    REDACTED
    applyInterceptWarmup(creds, false, 'edit')
    expect(creds.api_key).toBe('sk')
    expect(creds.base_url).toBe('url')
    expect('intercept_warmup_requests' in creds).toBe(false)
  REDACTED)
REDACTED)

describe('applyAntigravityProjectID', () => {
  it('create + project id: trims and stores configured project fallback', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' REDACTED
    applyAntigravityProjectID(creds, '  configured-project  ', 'create')
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  REDACTED)

  it('create + empty project id: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' REDACTED
    applyAntigravityProjectID(creds, '   ', 'create')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  REDACTED)

  it('edit + empty project id: deletes existing fallback', () => {
    const creds: Record<string, unknown> = {
      access_token: 'tok',
      [ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]: 'old-project'
    REDACTED
    applyAntigravityProjectID(creds, '', 'edit')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  REDACTED)

  it('does not affect onboard project_id or other credentials', () => {
    const creds: Record<string, unknown> = {
      project_id: 'onboard-project',
      model_mapping: { 'gemini-*': 'gemini-2.5-flash' REDACTED
    REDACTED
    applyAntigravityProjectID(creds, 'configured-project', 'edit')
    expect(creds.project_id).toBe('onboard-project')
    expect(creds.model_mapping).toEqual({ 'gemini-*': 'gemini-2.5-flash' REDACTED)
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  REDACTED)
REDACTED)

describe('isHeaderOverridePlatform', () => {
  it('only anthropic and openai are supported', () => {
    expect(isHeaderOverridePlatform('anthropic')).toBe(true)
    expect(isHeaderOverridePlatform('openai')).toBe(true)
    expect(isHeaderOverridePlatform('gemini')).toBe(false)
    expect(isHeaderOverridePlatform('grok')).toBe(false)
    expect(isHeaderOverridePlatform('antigravity')).toBe(false)
    expect(isHeaderOverridePlatform('')).toBe(false)
  REDACTED)
REDACTED)

describe('validateHeaderOverrideRows', () => {
  it('accepts valid rows and empty placeholder rows', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'user-agent', value: 'my-agent/1.0' REDACTED,
        { name: 'x-app', value: '' REDACTED,
        { name: '', value: '' REDACTED
      ])
    ).toBeNull()
  REDACTED)

  it('rejects empty name with non-empty value', () => {
    expect(validateHeaderOverrideRows([{ name: '', value: 'v' REDACTED])).toBe('invalidName')
  REDACTED)

  it('rejects invalid header names', () => {
    expect(validateHeaderOverrideRows([{ name: 'bad name', value: '' REDACTED])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: 'bad:name', value: '' REDACTED])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: '名称', value: '' REDACTED])).toBe('invalidName')
  REDACTED)

  it('rejects blocked header names case-insensitively', () => {
    expect(validateHeaderOverrideRows([{ name: 'Authorization', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'X-Api-Key', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'host', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Length', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Type', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Cookie', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-goog-api-key', value: '' REDACTED])).toBe('blockedName')
  REDACTED)

  it('rejects duplicate names case-insensitively', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'User-Agent', value: 'a' REDACTED,
        { name: 'user-agent', value: 'b' REDACTED
      ])
    ).toBe('duplicateName')
  REDACTED)
REDACTED)

describe('buildHeaderOverridesObject / splitHeaderOverridesObject', () => {
  it('lowercases names, trims values and drops empty-name rows', () => {
    expect(
      buildHeaderOverridesObject([
        { name: ' User-Agent ', value: ' my-agent ' REDACTED,
        { name: 'X-App', value: '' REDACTED,
        { name: '', value: 'ignored' REDACTED
      ])
    ).toEqual({ 'user-agent': 'my-agent', 'x-app': '' REDACTED)
  REDACTED)

  it('splits an object into sorted rows and ignores non-string values', () => {
    expect(
      splitHeaderOverridesObject({ 'x-app': 'cli', 'user-agent': 'ua', bogus: 42 REDACTED)
    ).toEqual([
      { name: 'user-agent', value: 'ua' REDACTED,
      { name: 'x-app', value: 'cli' REDACTED
    ])
    expect(splitHeaderOverridesObject(null)).toEqual([])
    expect(splitHeaderOverridesObject(['a'])).toEqual([])
    expect(splitHeaderOverridesObject('str')).toEqual([])
  REDACTED)

  it('roundtrips through build and split', () => {
    const rows = [
      { name: 'user-agent', value: 'ua' REDACTED,
      { name: 'x-app', value: 'cli' REDACTED
    ]
    expect(splitHeaderOverridesObject(buildHeaderOverridesObject(rows))).toEqual(rows)
  REDACTED)
REDACTED)

describe('getHeaderOverrideTemplate', () => {
  it('returns Claude Code CLI headers with empty values for anthropic', () => {
    const rows = getHeaderOverrideTemplate('anthropic')
    expect(rows.every((r) => r.value === '')).toBe(true)
    const names = rows.map((r) => r.name)
    expect(names).toContain('user-agent')
    expect(names).toContain('x-app')
    expect(names).toContain('anthropic-beta')
    expect(names).toContain('x-stainless-lang')
    expect(validateHeaderOverrideRows(rows)).toBeNull()
  REDACTED)

  it('returns Codex CLI headers with empty values for openai', () => {
    const rows = getHeaderOverrideTemplate('openai')
    expect(rows.every((r) => r.value === '')).toBe(true)
    const names = rows.map((r) => r.name)
    expect(names).toContain('user-agent')
    expect(names).toContain('originator')
    expect(names).toContain('openai-beta')
    expect(validateHeaderOverrideRows(rows)).toBeNull()
  REDACTED)
REDACTED)

describe('applyHeaderOverride', () => {
  it('create + enabled: writes enabled flag and overrides object', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' REDACTED
    applyHeaderOverride(creds, true, [{ name: 'User-Agent', value: 'ua' REDACTED], 'create')
    expect(creds[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]).toBe(true)
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'user-agent': 'ua' REDACTED)
  REDACTED)

  it('create + disabled: does not add fields', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' REDACTED
    applyHeaderOverride(creds, false, [{ name: 'user-agent', value: 'ua' REDACTED], 'create')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
  REDACTED)

  it('edit + disabled: deletes existing fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'user-agent': 'ua' REDACTED
    REDACTED
    applyHeaderOverride(creds, false, [], 'edit')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
    expect(creds.api_key).toBe('sk')
  REDACTED)

  it('edit + enabled: replaces overrides object wholesale', () => {
    const creds: Record<string, unknown> = {
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'x-old': 'old' REDACTED
    REDACTED
    applyHeaderOverride(creds, true, [{ name: 'x-new', value: 'new' REDACTED], 'edit')
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'x-new': 'new' REDACTED)
  REDACTED)
REDACTED)

describe('validateHeaderOverrideRows value/entry limits', () => {
  it('rejects websocket handshake headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'Sec-WebSocket-Key', value: '' REDACTED])).toBe(
      'blockedName'
    )
  REDACTED)

  it('rejects control characters in values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\x0bb' REDACTED])).toBe('invalidValue')
  REDACTED)

  it('rejects oversized values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a'.repeat(8193) REDACTED])).toBe(
      'invalidValue'
    )
  REDACTED)

  it('measures value length in UTF-8 bytes to match backend', () => {
    // 3000 个 CJK 字符 = 3000 UTF-16 code units，但 9000 UTF-8 字节 > 8192
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(3000) REDACTED])).toBe(
      'invalidValue'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(2000) REDACTED])).toBeNull()
  REDACTED)

  it('rejects too many entries', () => {
    const rows = Array.from({ length: 65 REDACTED, (_, i) => ({ name: `x-h-${iREDACTED`, value: 'v' REDACTED))
    expect(validateHeaderOverrideRows(rows)).toBe('tooManyEntries')
  REDACTED)
REDACTED)

describe('validateHeaderOverrideRows session isolation headers', () => {
  it('rejects per-request session headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'session_id', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Conversation_ID', value: '' REDACTED])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-codex-turn-state', value: '' REDACTED])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'X-Claude-Code-Session-Id', value: '' REDACTED])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-client-request-id', value: '' REDACTED])).toBe(
      'blockedName'
    )
  REDACTED)

  it('allows tab inside value', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\tb' REDACTED])).toBeNull()
  REDACTED)

  it('rejects oversized names', () => {
    expect(validateHeaderOverrideRows([{ name: 'x'.repeat(201), value: 'v' REDACTED])).toBe('invalidName')
  REDACTED)
REDACTED)

describe('plan_type helpers', () => {
  describe('planTypeDisplayLabel', () => {
    it('maps canonical + alias values to friendly labels', () => {
      expect(planTypeDisplayLabel('plus')).toBe('Plus')
      expect(planTypeDisplayLabel('pro')).toBe('Pro')
      expect(planTypeDisplayLabel('chatgptpro')).toBe('Pro')
      expect(planTypeDisplayLabel('free')).toBe('Free')
      expect(planTypeDisplayLabel('team')).toBe('Team')
      expect(planTypeDisplayLabel('CHATGPTPRO')).toBe('Pro')
    REDACTED)
    it('returns unknown values verbatim', () => {
      expect(planTypeDisplayLabel('self_serve_business')).toBe('self_serve_business')
    REDACTED)
  REDACTED)

  describe('readPlanType', () => {
    it('reads a string plan_type', () => {
      expect(readPlanType({ plan_type: 'plus' REDACTED)).toBe('plus')
    REDACTED)
    it('treats non-string / missing values as empty', () => {
      expect(readPlanType({ plan_type: 42 REDACTED)).toBe('')
      expect(readPlanType({ plan_type: true REDACTED)).toBe('')
      expect(readPlanType({REDACTED)).toBe('')
      expect(readPlanType(undefined)).toBe('')
      expect(readPlanType(null)).toBe('')
    REDACTED)
  REDACTED)

  describe('buildPlanTypeOptions', () => {
    const clear = 'Clear'
    it('returns clear + presets when current is empty', () => {
      expect(buildPlanTypeOptions('', clear)).toEqual([
        { value: '', label: clear REDACTED,
        { value: 'plus', label: 'Plus' REDACTED,
        { value: 'pro', label: 'Pro' REDACTED,
        { value: 'free', label: 'Free' REDACTED
      ])
    REDACTED)
    it('keeps canonical chatgptpro under a single friendly "Pro" option (no duplicate)', () => {
      const opts = buildPlanTypeOptions('chatgptpro', clear)
      const pros = opts.filter(o => o.label === 'Pro')
      expect(pros).toHaveLength(1)
      expect(pros[0].value).toBe('chatgptpro')
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'chatgptpro', 'free'])
    REDACTED)
    it('appends an unknown-but-labeled value (team) as its own option', () => {
      const opts = buildPlanTypeOptions('team', clear)
      expect(opts.find(o => o.value === 'team')).toEqual({ value: 'team', label: 'Team' REDACTED)
      // presets untouched
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'pro', 'free', 'team'])
    REDACTED)
    it('appends a fully custom value with a raw label', () => {
      const opts = buildPlanTypeOptions('weird_x', clear)
      expect(opts.at(-1)).toEqual({ value: 'weird_x', label: 'weird_x' REDACTED)
    REDACTED)
    it('does not duplicate an exact preset value', () => {
      const opts = buildPlanTypeOptions('pro', clear)
      expect(opts.filter(o => o.value === 'pro')).toHaveLength(1)
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'pro', 'free'])
    REDACTED)
  REDACTED)

  describe('applyPlanType', () => {
    it('sets plan_type and preserves all other credential keys', () => {
      const creds = {
        chatgpt_account_id: 'acc',
        email: 'a@b.c',
        subscription_expires_at: '2026-01-01',
        model_mapping: { x: 'y' REDACTED
      REDACTED
      const out = applyPlanType({ ...creds REDACTED, 'plus')
      expect(out).toEqual({ ...creds, plan_type: 'plus' REDACTED)
    REDACTED)
    it('trims the value', () => {
      expect(applyPlanType({REDACTED, '  pro  ')).toEqual({ plan_type: 'pro' REDACTED)
    REDACTED)
    it('deletes the key when cleared (empty), keeping other keys', () => {
      const out = applyPlanType({ plan_type: 'pro', email: 'a@b.c' REDACTED, '')
      expect(out).toEqual({ email: 'a@b.c' REDACTED)
      expect('plan_type' in out).toBe(false)
    REDACTED)
  REDACTED)
REDACTED)

