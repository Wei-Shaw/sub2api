import { afterEach, describe, expect, it } from 'vitest'

import {
  BUILTIN_PLATFORM_CATALOG,
  resetPlatformCatalog,
  setPlatformCatalog,
  type PlatformCatalog
} from '@/constants/platformCatalog'
import {
  DEFAULT_OPENCODE_GO_PROTOCOL_RULES,
  DEFAULT_OPENCODE_ZEN_PROTOCOL_RULES,
  OPENCODE_GO_ANTHROPIC_BASE_URL,
  OPENCODE_GO_BASE_URL,
  OPENCODE_ZEN_ANTHROPIC_BASE_URL,
  OPENCODE_ZEN_BASE_URL,
  cnBalanceCellVisible,
  cnQuotaCellVisible,
  cnSupportsNativeResponses,
  defaultCNAdaptiveBaseUrls,
  defaultCNBaseUrl,
  defaultOpenCodeProtocolRules,
  defaultProviderProtocolRules,
  isHeaderOverrideCapable,
  isMultiProtocolApiKeyPlatform,
  providerAccountModes,
  providerHasModelCatalog,
  providerModeLabel,
  providerNativeProtocols,
  providerRoutesByModel,
  resolveProviderAccountMode,
  type CnApiProtocol
} from '../credentialsBuilder'

// ===== 改为读取平台清单之前的实现，作为等价基准（补入之后内置登记的 Command Code） =====

const legacyOpenCode = {
  goBase: 'https://opencode.ai/zen/go/v1',
  goAnthropic: 'https://opencode.ai/zen/go',
  zenBase: 'https://opencode.ai/zen/v1',
  zenAnthropic: 'https://opencode.ai/zen'
}

const legacyGoRules = [
  { pattern: 'grok-*', protocol: 'responses' },
  { pattern: 'gpt-*', protocol: 'responses' },
  { pattern: 'muse-spark-*', protocol: 'responses' },
  { pattern: 'minimax-*', protocol: 'anthropic' },
  { pattern: 'qwen*', protocol: 'anthropic' }
]

const legacyZenRules = [
  { pattern: 'grok-*', protocol: 'responses' },
  { pattern: 'gpt-*', protocol: 'responses' },
  { pattern: 'muse-spark-*', protocol: 'responses' },
  { pattern: 'claude-*', protocol: 'anthropic' },
  { pattern: 'qwen3.8-max', protocol: 'chat_completions' },
  { pattern: 'qwen*', protocol: 'anthropic' }
]

function legacyIsMultiProtocol(platform: string): boolean {
  return ['kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'command_code', 'cline'].includes(platform)
}

function legacySupportsResponses(platform: string): boolean {
  return ['deepseek', 'kimi', 'minimax', 'opencode_go', 'command_code'].includes(platform)
}

function legacyHeaderOverride(platform: string, type: string): boolean {
  if (['anthropic', 'openai', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'command_code', 'cline'].includes(platform)) {
    return type === 'apikey'
  }
  if (platform === 'grok') return type === 'apikey' || type === 'oauth'
  return false
}

function legacyDefaultCNBaseUrl(platform: string, mode: string, protocol: CnApiProtocol = 'chat_completions'): string {
  if (protocol === 'anthropic') {
    switch (platform) {
      case 'kimi':
        return mode === 'coding' ? 'https://api.kimi.com/coding' : 'https://api.moonshot.cn/anthropic'
      case 'zhipu':
        return 'https://open.bigmodel.cn/api/anthropic'
      case 'deepseek':
        return 'https://api.deepseek.com/anthropic'
      case 'minimax':
        return 'https://api.minimaxi.com/anthropic'
      case 'opencode_go':
        return mode === 'zen' ? legacyOpenCode.zenAnthropic : legacyOpenCode.goAnthropic
      default:
        return ''
    }
  }
  switch (platform) {
    case 'kimi':
      return mode === 'coding' ? 'https://api.kimi.com/coding/v1' : 'https://api.moonshot.cn/v1'
    case 'zhipu':
      return mode === 'coding' ? 'https://open.bigmodel.cn/api/coding/paas/v4' : 'https://open.bigmodel.cn/api/paas/v4'
    case 'deepseek':
      return 'https://api.deepseek.com'
    case 'minimax':
      return 'https://api.minimaxi.com/v1'
    case 'opencode_go':
      return mode === 'zen' ? legacyOpenCode.zenBase : legacyOpenCode.goBase
    default:
      return ''
  }
}

function legacyAdaptiveBaseUrls(platform: string, mode: string) {
  return {
    chat_completions: legacyDefaultCNBaseUrl(platform, mode, 'chat_completions'),
    anthropic: legacyDefaultCNBaseUrl(platform, mode, 'anthropic'),
    responses: legacySupportsResponses(platform) ? legacyDefaultCNBaseUrl(platform, mode, 'responses') : ''
  }
}

// 各平台在旧实现中可取到的接入模式（旧实现对未知模式回落默认模式）。
const legacyModes: Record<string, string[]> = {
  kimi: ['payg', 'coding'],
  zhipu: ['payg', 'coding'],
  deepseek: ['payg'],
  minimax: ['payg', 'coding'],
  opencode_go: ['go', 'zen']
}

const probePlatforms = [
  ...BUILTIN_PLATFORM_CATALOG.platforms.map(spec => spec.id),
  'composite',
  '',
  'bogus'
]
const probeTypes = ['apikey', 'oauth', 'setup-token', 'upstream', 'bedrock', '']
const protocols: CnApiProtocol[] = ['adaptive', 'chat_completions', 'anthropic', 'responses']

afterEach(() => {
  resetPlatformCatalog()
})

describe('credentialsBuilder derives multi-protocol data from the platform catalog', () => {
  it('matches the legacy predicates for every platform', () => {
    for (const platform of probePlatforms) {
      expect(isMultiProtocolApiKeyPlatform(platform), platform).toBe(legacyIsMultiProtocol(platform))
      expect(cnSupportsNativeResponses(platform), platform).toBe(legacySupportsResponses(platform))
      for (const type of probeTypes) {
        expect(isHeaderOverrideCapable(platform, type), `${platform}/${type}`).toBe(legacyHeaderOverride(platform, type))
      }
    }
  })

  it('matches the legacy default endpoints for every platform, mode and protocol', () => {
    for (const [platform, modes] of Object.entries(legacyModes)) {
      for (const mode of [...modes, 'unknown-mode']) {
        const legacyMode = modes.includes(mode) ? mode : modes[0]
        for (const protocol of protocols) {
          expect(defaultCNBaseUrl(platform, mode, protocol), `${platform}/${mode}/${protocol}`).toBe(
            legacyDefaultCNBaseUrl(platform, legacyMode, protocol)
          )
        }
        expect(defaultCNAdaptiveBaseUrls(platform, mode), `${platform}/${mode}`).toEqual(
          legacyAdaptiveBaseUrls(platform, legacyMode)
        )
      }
    }
    for (const platform of ['anthropic', 'openai', 'grok', 'bogus']) {
      expect(defaultCNBaseUrl(platform, 'payg', 'chat_completions')).toBe('')
    }
  })

  it('keeps the exported OpenCode constants and default rules', () => {
    expect(OPENCODE_GO_BASE_URL).toBe(legacyOpenCode.goBase)
    expect(OPENCODE_GO_ANTHROPIC_BASE_URL).toBe(legacyOpenCode.goAnthropic)
    expect(OPENCODE_ZEN_BASE_URL).toBe(legacyOpenCode.zenBase)
    expect(OPENCODE_ZEN_ANTHROPIC_BASE_URL).toBe(legacyOpenCode.zenAnthropic)
    expect(DEFAULT_OPENCODE_GO_PROTOCOL_RULES).toEqual(legacyGoRules)
    expect(DEFAULT_OPENCODE_ZEN_PROTOCOL_RULES).toEqual(legacyZenRules)
    expect(defaultOpenCodeProtocolRules('go')).toEqual(legacyGoRules)
    expect(defaultOpenCodeProtocolRules('zen')).toEqual(legacyZenRules)
    expect(defaultOpenCodeProtocolRules()).toEqual(legacyGoRules)
  })

  it('exposes routing, modes and native protocols of built-in providers', () => {
    expect(providerRoutesByModel('opencode_go')).toBe(true)
    expect(providerRoutesByModel('kimi')).toBe(false)
    expect(providerAccountModes('opencode_go')).toEqual(['go', 'zen'])
    expect(providerAccountModes('deepseek')).toEqual(['payg'])
    expect(providerNativeProtocols('zhipu', 'coding')).toEqual(['chat_completions', 'anthropic'])
    expect(resolveProviderAccountMode('kimi', 'coding')).toBe('coding')
    expect(resolveProviderAccountMode('deepseek', 'coding')).toBe('payg')
    expect(resolveProviderAccountMode('opencode_go', undefined)).toBe('go')
  })
})

describe('credentialsBuilder built-in Command Code provider', () => {
  it('uses the Command Code endpoints and routes Claude / GPT by model', () => {
    expect(isMultiProtocolApiKeyPlatform('command_code')).toBe(true)
    expect(providerRoutesByModel('command_code')).toBe(true)
    expect(providerAccountModes('command_code')).toEqual(['payg'])
    expect(defaultCNAdaptiveBaseUrls('command_code', 'payg')).toEqual({
      chat_completions: 'https://api.commandcode.ai/provider/v1',
      anthropic: 'https://api.commandcode.ai/provider',
      responses: 'https://api.commandcode.ai/provider/v1'
    })
    expect(defaultProviderProtocolRules('command_code', 'payg')).toEqual([
      { pattern: 'claude-*', protocol: 'anthropic' },
      { pattern: 'gpt-*', protocol: 'responses', extraProtocols: ['chat_completions'] }
    ])
    expect(providerHasModelCatalog('command_code')).toBe(true)
    expect(providerHasModelCatalog('opencode_go')).toBe(false)
  })

  it('shows both the quota windows and the credit balance for official-host API keys only', () => {
    const account = (credentials: Record<string, unknown>, type = 'apikey') => ({ platform: 'command_code', type, credentials })
    expect(cnQuotaCellVisible(account({ account_mode: 'payg' }))).toBe(true)
    expect(cnBalanceCellVisible(account({ account_mode: 'payg' }))).toBe(true)
    expect(cnQuotaCellVisible(account({ base_url: 'https://api.commandcode.ai/provider/v1' }))).toBe(true)
    // 自定义中转与非 https 端点：后端不查官方用量接口，单元格不显示。
    const relay = account({ api_protocol: 'chat_completions', base_url: 'https://relay.example.com/v1' })
    expect(cnQuotaCellVisible(relay)).toBe(false)
    expect(cnBalanceCellVisible(relay)).toBe(false)
    expect(cnQuotaCellVisible(account({ base_url: 'http://api.commandcode.ai/provider/v1' }))).toBe(false)
    // 按模型分流的账号默认 adaptive，看 Chat Completions 分协议端点。
    const adaptiveRelay = account({ api_base_urls: { chat_completions: 'https://relay.example.com/v1' } })
    expect(cnQuotaCellVisible(adaptiveRelay)).toBe(false)
    expect(cnBalanceCellVisible(account({ account_mode: 'payg' }, 'oauth'))).toBe(false)
    expect(cnQuotaCellVisible({ platform: 'deepseek', type: 'apikey', credentials: { account_mode: 'payg' } })).toBe(false)
    expect(cnBalanceCellVisible({ platform: 'zhipu', type: 'apikey', credentials: { account_mode: 'payg' } })).toBe(false)
  })
})

describe('credentialsBuilder built-in Cline provider', () => {
  it('only offers Chat Completions and routes by inbound protocol', () => {
    expect(isMultiProtocolApiKeyPlatform('cline')).toBe(true)
    expect(providerRoutesByModel('cline')).toBe(false)
    // 积分与 ClinePass 共用同一个 Key 与端点，只有一个接入模式（不显示账号类型）。
    expect(providerAccountModes('cline')).toEqual(['payg'])
    expect(defaultCNAdaptiveBaseUrls('cline', 'payg')).toEqual({
      chat_completions: 'https://api.cline.bot/api/v1',
      anthropic: '',
      responses: ''
    })
    expect(providerNativeProtocols('cline', 'payg')).toEqual(['chat_completions'])
    expect(defaultProviderProtocolRules('cline', 'payg')).toEqual([])
  })

  it('shows ClinePass windows and the credit balance for official-host API keys only', () => {
    expect(cnQuotaCellVisible({ platform: 'cline', type: 'apikey', credentials: {} })).toBe(true)
    expect(cnBalanceCellVisible({ platform: 'cline', type: 'apikey', credentials: {} })).toBe(true)
    const relay = { platform: 'cline', type: 'apikey', credentials: { base_url: 'https://relay.example.com/v1' } }
    expect(cnQuotaCellVisible(relay)).toBe(false)
    expect(cnBalanceCellVisible(relay)).toBe(false)
  })

  it('labels provider modes', () => {
    const t = (key: string) => key
    expect(providerModeLabel('payg', t)).toBe('admin.accounts.cnProviders.accountMode.payg')
    expect(providerModeLabel('go', t)).toBe('admin.accounts.opencodeGo.accountMode.go')
    expect(providerModeLabel('standard', t)).toBe('standard')
  })
})

describe('credentialsBuilder picks up newly registered providers from the catalog', () => {
  const serverCatalog: PlatformCatalog = {
    platforms: [
      ...BUILTIN_PLATFORM_CATALOG.platforms,
      {
        id: 'acme_router',
        display_name: 'Acme Router',
        gateway: 'openai',
        cn_provider: false,
        multi_protocol: {
          default_mode: 'standard',
          routing: 'by_model',
          modes: [
            {
              mode: 'standard',
              base_urls: {
                chat_completions: 'https://api.acme-router.example/provider/v1',
                anthropic: 'https://api.acme-router.example/provider'
              },
              protocol_rules: [
                { pattern: 'claude-*', protocol: 'anthropic' },
                { pattern: 'bad-*', protocol: 'not-a-protocol' }
              ]
            }
          ]
        }
      }
    ],
    composite_precedence: [...BUILTIN_PLATFORM_CATALOG.composite_precedence, 'acme_router']
  }

  it('treats the new provider as a multi-protocol API-key platform', () => {
    expect(isMultiProtocolApiKeyPlatform('acme_router')).toBe(false)
    setPlatformCatalog(serverCatalog)

    expect(isMultiProtocolApiKeyPlatform('acme_router')).toBe(true)
    expect(isHeaderOverrideCapable('acme_router', 'apikey')).toBe(true)
    expect(isHeaderOverrideCapable('acme_router', 'oauth')).toBe(false)
    expect(cnSupportsNativeResponses('acme_router')).toBe(false)
    expect(providerNativeProtocols('acme_router')).toEqual(['chat_completions', 'anthropic'])
    expect(providerRoutesByModel('acme_router')).toBe(true)
    expect(resolveProviderAccountMode('acme_router', 'payg')).toBe('standard')
    expect(defaultCNBaseUrl('acme_router', 'standard', 'anthropic')).toBe('https://api.acme-router.example/provider')
    expect(defaultCNBaseUrl('acme_router', 'standard', 'responses')).toBe('https://api.acme-router.example/provider/v1')
    expect(defaultCNAdaptiveBaseUrls('acme_router', 'standard')).toEqual({
      chat_completions: 'https://api.acme-router.example/provider/v1',
      anthropic: 'https://api.acme-router.example/provider',
      responses: ''
    })
    // 非法协议的规则被丢弃。
    expect(defaultProviderProtocolRules('acme_router', 'standard')).toEqual([
      { pattern: 'claude-*', protocol: 'anthropic' }
    ])
  })
})
