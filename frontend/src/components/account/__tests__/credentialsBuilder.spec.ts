import { describe, it, expect } from 'vitest'
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
  isCustomGrokBaseUrl,
  isHeaderOverrideCapable,
  parseHeaderOverridesJson,
  planTypeDisplayLabel,
  readPlanType,
  serializeHeaderOverrideRows,
  splitHeaderOverridesObject,
  validateHeaderOverrideRows
} from '../credentialsBuilder'

describe('applyInterceptWarmup', () => {
  it('create + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, true, 'create')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('create + enabled=false: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, false, 'create')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, true, 'edit')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('edit + enabled=false + field exists: should delete the field', () => {
    const creds: Record<string, unknown> = { api_key: 'sk', intercept_warmup_requests: true }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=false + field absent: should not throw', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('should not affect other fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      base_url: 'url',
      intercept_warmup_requests: true
    }
    applyInterceptWarmup(creds, false, 'edit')
    expect(creds.api_key).toBe('sk')
    expect(creds.base_url).toBe('url')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })
})

describe('applyAntigravityProjectID', () => {
  it('create + project id: trims and stores configured project fallback', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyAntigravityProjectID(creds, '  configured-project  ', 'create')
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  })

  it('create + empty project id: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyAntigravityProjectID(creds, '   ', 'create')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('edit + empty project id: deletes existing fallback', () => {
    const creds: Record<string, unknown> = {
      access_token: 'tok',
      [ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]: 'old-project'
    }
    applyAntigravityProjectID(creds, '', 'edit')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('does not affect onboard project_id or other credentials', () => {
    const creds: Record<string, unknown> = {
      project_id: 'onboard-project',
      model_mapping: { 'gemini-*': 'gemini-2.5-flash' }
    }
    applyAntigravityProjectID(creds, 'configured-project', 'edit')
    expect(creds.project_id).toBe('onboard-project')
    expect(creds.model_mapping).toEqual({ 'gemini-*': 'gemini-2.5-flash' })
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  })
})

describe('isHeaderOverrideCapable', () => {
  it('anthropic/openai only support apikey accounts', () => {
    expect(isHeaderOverrideCapable('anthropic', 'apikey')).toBe(true)
    expect(isHeaderOverrideCapable('openai', 'apikey')).toBe(true)
    expect(isHeaderOverrideCapable('anthropic', 'oauth')).toBe(false)
    expect(isHeaderOverrideCapable('openai', 'oauth')).toBe(false)
  })

  it('grok supports both apikey and oauth accounts', () => {
    expect(isHeaderOverrideCapable('grok', 'apikey')).toBe(true)
    expect(isHeaderOverrideCapable('grok', 'oauth')).toBe(true)
    expect(isHeaderOverrideCapable('grok', 'bedrock')).toBe(false)
  })

  it('other platforms are not supported', () => {
    expect(isHeaderOverrideCapable('gemini', 'apikey')).toBe(false)
    expect(isHeaderOverrideCapable('antigravity', 'apikey')).toBe(false)
    expect(isHeaderOverrideCapable('', 'apikey')).toBe(false)
  })
})

describe('parseHeaderOverridesJson', () => {
  it('parses a flat object and normalizes values to trimmed strings', () => {
    expect(
      parseHeaderOverridesJson('{"User-Agent": " my-client/1.0 ", "x-num": 3, "x-flag": true}')
    ).toEqual([
      { name: 'User-Agent', value: 'my-client/1.0' },
      { name: 'x-flag', value: 'true' },
      { name: 'x-num', value: '3' }
    ])
  })

  it('drops entries with blank names', () => {
    expect(parseHeaderOverridesJson('{"  ": "v", "x-app": "cli"}')).toEqual([
      { name: 'x-app', value: 'cli' }
    ])
  })

  it('rejects invalid JSON, arrays, primitives and nested values', () => {
    expect(parseHeaderOverridesJson('not json')).toBeNull()
    expect(parseHeaderOverridesJson('[1,2]')).toBeNull()
    expect(parseHeaderOverridesJson('"str"')).toBeNull()
    expect(parseHeaderOverridesJson('null')).toBeNull()
    expect(parseHeaderOverridesJson('{"a": {"b": 1}}')).toBeNull()
    expect(parseHeaderOverridesJson('{"a": null}')).toBeNull()
  })

  it('parses an empty object to an empty row list', () => {
    expect(parseHeaderOverridesJson('{}')).toEqual([])
  })
})

describe('serializeHeaderOverrideRows', () => {
  it('serializes named rows and skips empty placeholder rows', () => {
    const text = serializeHeaderOverrideRows([
      { name: ' user-agent ', value: ' my-client/1.0 ' },
      { name: '', value: 'ignored' },
      { name: 'x-app', value: '' }
    ])
    expect(JSON.parse(text)).toEqual({ 'user-agent': 'my-client/1.0', 'x-app': '' })
  })

  it('round-trips with parseHeaderOverridesJson', () => {
    const rows = [
      { name: 'a-header', value: '1' },
      { name: 'b-header', value: '2' }
    ]
    expect(parseHeaderOverridesJson(serializeHeaderOverrideRows(rows))).toEqual(rows)
  })
})

describe('isCustomGrokBaseUrl', () => {
  it('treats official hosts and their variants as not customized', () => {
    expect(isCustomGrokBaseUrl('https://api.x.ai/v1')).toBe(false)
    expect(isCustomGrokBaseUrl('https://cli-chat-proxy.grok.com/v1')).toBe(false)
    expect(isCustomGrokBaseUrl('HTTPS://API.X.AI:443/')).toBe(false)
    expect(isCustomGrokBaseUrl('https://api.x.ai:8443/v1')).toBe(false)
  })

  it('treats empty, non-string and unparseable values as not customized', () => {
    expect(isCustomGrokBaseUrl('')).toBe(false)
    expect(isCustomGrokBaseUrl('   ')).toBe(false)
    expect(isCustomGrokBaseUrl(undefined)).toBe(false)
    expect(isCustomGrokBaseUrl(42)).toBe(false)
    expect(isCustomGrokBaseUrl('not a url')).toBe(false)
  })

  it('treats third-party hosts as customized', () => {
    expect(isCustomGrokBaseUrl('https://relay.example.com/v1')).toBe(true)
    expect(isCustomGrokBaseUrl('https://relay.example.com/xai/v1')).toBe(true)
    expect(isCustomGrokBaseUrl('http://relay.example.com/v1')).toBe(true)
  })
})

describe('validateHeaderOverrideRows', () => {
  it('accepts valid rows and empty placeholder rows', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'user-agent', value: 'my-agent/1.0' },
        { name: 'x-app', value: '' },
        { name: '', value: '' }
      ])
    ).toBeNull()
  })

  it('rejects empty name with non-empty value', () => {
    expect(validateHeaderOverrideRows([{ name: '', value: 'v' }])).toBe('invalidName')
  })

  it('rejects invalid header names', () => {
    expect(validateHeaderOverrideRows([{ name: 'bad name', value: '' }])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: 'bad:name', value: '' }])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: '名称', value: '' }])).toBe('invalidName')
  })

  it('rejects blocked header names case-insensitively', () => {
    expect(validateHeaderOverrideRows([{ name: 'Authorization', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'X-Api-Key', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'host', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Length', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Type', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Cookie', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-goog-api-key', value: '' }])).toBe('blockedName')
  })

  it('rejects duplicate names case-insensitively', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'User-Agent', value: 'a' },
        { name: 'user-agent', value: 'b' }
      ])
    ).toBe('duplicateName')
  })
})

describe('buildHeaderOverridesObject / splitHeaderOverridesObject', () => {
  it('lowercases names, trims values and drops empty-name rows', () => {
    expect(
      buildHeaderOverridesObject([
        { name: ' User-Agent ', value: ' my-agent ' },
        { name: 'X-App', value: '' },
        { name: '', value: 'ignored' }
      ])
    ).toEqual({ 'user-agent': 'my-agent', 'x-app': '' })
  })

  it('splits an object into sorted rows and ignores non-string values', () => {
    expect(
      splitHeaderOverridesObject({ 'x-app': 'cli', 'user-agent': 'ua', bogus: 42 })
    ).toEqual([
      { name: 'user-agent', value: 'ua' },
      { name: 'x-app', value: 'cli' }
    ])
    expect(splitHeaderOverridesObject(null)).toEqual([])
    expect(splitHeaderOverridesObject(['a'])).toEqual([])
    expect(splitHeaderOverridesObject('str')).toEqual([])
  })

  it('roundtrips through build and split', () => {
    const rows = [
      { name: 'user-agent', value: 'ua' },
      { name: 'x-app', value: 'cli' }
    ]
    expect(splitHeaderOverridesObject(buildHeaderOverridesObject(rows))).toEqual(rows)
  })
})

describe('applyHeaderOverride', () => {
  it('create + enabled: writes enabled flag and overrides object', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyHeaderOverride(creds, true, [{ name: 'User-Agent', value: 'ua' }], 'create')
    expect(creds[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]).toBe(true)
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'user-agent': 'ua' })
  })

  it('create + disabled: does not add fields', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyHeaderOverride(creds, false, [{ name: 'user-agent', value: 'ua' }], 'create')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('edit + disabled: deletes existing fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'user-agent': 'ua' }
    }
    applyHeaderOverride(creds, false, [], 'edit')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
    expect(creds.api_key).toBe('sk')
  })

  it('edit + enabled: replaces overrides object wholesale', () => {
    const creds: Record<string, unknown> = {
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'x-old': 'old' }
    }
    applyHeaderOverride(creds, true, [{ name: 'x-new', value: 'new' }], 'edit')
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'x-new': 'new' })
  })
})

describe('validateHeaderOverrideRows value/entry limits', () => {
  it('rejects websocket handshake headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'Sec-WebSocket-Key', value: '' }])).toBe(
      'blockedName'
    )
  })

  it('rejects control characters in values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\x0bb' }])).toBe('invalidValue')
  })

  it('rejects oversized values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a'.repeat(8193) }])).toBe(
      'invalidValue'
    )
  })

  it('measures value length in UTF-8 bytes to match backend', () => {
    // 3000 个 CJK 字符 = 3000 UTF-16 code units，但 9000 UTF-8 字节 > 8192
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(3000) }])).toBe(
      'invalidValue'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(2000) }])).toBeNull()
  })

  it('rejects too many entries', () => {
    const rows = Array.from({ length: 65 }, (_, i) => ({ name: `x-h-${i}`, value: 'v' }))
    expect(validateHeaderOverrideRows(rows)).toBe('tooManyEntries')
  })
})

describe('validateHeaderOverrideRows session isolation headers', () => {
  it('rejects per-request session headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'session_id', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Conversation_ID', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-codex-turn-state', value: '' }])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'X-Claude-Code-Session-Id', value: '' }])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-client-request-id', value: '' }])).toBe(
      'blockedName'
    )
  })

  it('allows tab inside value', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\tb' }])).toBeNull()
  })

  it('rejects oversized names', () => {
    expect(validateHeaderOverrideRows([{ name: 'x'.repeat(201), value: 'v' }])).toBe('invalidName')
  })
})

describe('plan_type helpers', () => {
  describe('planTypeDisplayLabel', () => {
    it('maps canonical + alias values to friendly labels', () => {
      expect(planTypeDisplayLabel('plus')).toBe('Plus')
      expect(planTypeDisplayLabel('pro')).toBe('Pro')
      expect(planTypeDisplayLabel('chatgptpro')).toBe('Pro')
      expect(planTypeDisplayLabel('free')).toBe('Free')
      expect(planTypeDisplayLabel('team')).toBe('Team')
      expect(planTypeDisplayLabel('CHATGPTPRO')).toBe('Pro')
    })
    it('returns unknown values verbatim', () => {
      expect(planTypeDisplayLabel('self_serve_business')).toBe('self_serve_business')
    })
  })

  describe('readPlanType', () => {
    it('reads a string plan_type', () => {
      expect(readPlanType({ plan_type: 'plus' })).toBe('plus')
    })
    it('treats non-string / missing values as empty', () => {
      expect(readPlanType({ plan_type: 42 })).toBe('')
      expect(readPlanType({ plan_type: true })).toBe('')
      expect(readPlanType({})).toBe('')
      expect(readPlanType(undefined)).toBe('')
      expect(readPlanType(null)).toBe('')
    })
  })

  describe('buildPlanTypeOptions', () => {
    const clear = 'Clear'
    it('returns clear + presets when current is empty', () => {
      expect(buildPlanTypeOptions('', clear)).toEqual([
        { value: '', label: clear },
        { value: 'plus', label: 'Plus' },
        { value: 'pro', label: 'Pro' },
        { value: 'free', label: 'Free' }
      ])
    })
    it('keeps canonical chatgptpro under a single friendly "Pro" option (no duplicate)', () => {
      const opts = buildPlanTypeOptions('chatgptpro', clear)
      const pros = opts.filter(o => o.label === 'Pro')
      expect(pros).toHaveLength(1)
      expect(pros[0].value).toBe('chatgptpro')
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'chatgptpro', 'free'])
    })
    it('appends an unknown-but-labeled value (team) as its own option', () => {
      const opts = buildPlanTypeOptions('team', clear)
      expect(opts.find(o => o.value === 'team')).toEqual({ value: 'team', label: 'Team' })
      // presets untouched
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'pro', 'free', 'team'])
    })
    it('appends a fully custom value with a raw label', () => {
      const opts = buildPlanTypeOptions('weird_x', clear)
      expect(opts.at(-1)).toEqual({ value: 'weird_x', label: 'weird_x' })
    })
    it('does not duplicate an exact preset value', () => {
      const opts = buildPlanTypeOptions('pro', clear)
      expect(opts.filter(o => o.value === 'pro')).toHaveLength(1)
      expect(opts.map(o => o.value)).toEqual(['', 'plus', 'pro', 'free'])
    })
  })

  describe('applyPlanType', () => {
    it('sets plan_type and preserves all other credential keys', () => {
      const creds = {
        chatgpt_account_id: 'acc',
        email: 'a@b.c',
        subscription_expires_at: '2026-01-01',
        model_mapping: { x: 'y' }
      }
      const out = applyPlanType({ ...creds }, 'plus')
      expect(out).toEqual({ ...creds, plan_type: 'plus' })
    })
    it('trims the value', () => {
      expect(applyPlanType({}, '  pro  ')).toEqual({ plan_type: 'pro' })
    })
    it('deletes the key when cleared (empty), keeping other keys', () => {
      const out = applyPlanType({ plan_type: 'pro', email: 'a@b.c' }, '')
      expect(out).toEqual({ email: 'a@b.c' })
      expect('plan_type' in out).toBe(false)
    })
  })
})

