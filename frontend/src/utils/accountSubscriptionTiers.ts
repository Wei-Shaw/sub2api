import { openAIPlanTypeLabel } from './planType'

// Values identify provider and tier independently so e.g. Claude Pro and
// ChatGPT Pro 200 never share a filter accidentally.
export const ACCOUNT_SUBSCRIPTION_TIER_OPTIONS = [
  ...[
    'free', 'go', 'plus', 'prolite', 'pro', 'promax', 'team',
    'selfservebusinessprolite', 'enterprise', 'enterprisecbpautomation',
    'edu', 'eduplus', 'edupro', 'unknown'
  ].map(key => ({ value: `openai:${key}`, label: `OpenAI · ${openAIPlanTypeLabel(key)}` })),
  ...['free', 'pro', 'max', 'team', 'enterprise'].map(key => ({
    value: `anthropic:${key}`, label: `Claude · ${key[0]!.toUpperCase()}${key.slice(1)}`
  })),
  ...[
    ['freetier', 'Free'], ['g1protier', 'Pro'], ['g1ultratier', 'Ultra']
  ].map(([key, label]) => ({ value: `antigravity:${key}`, label: `Antigravity · ${label}` })),
  ...[
    ['free', 'Grok Free'], ['supergroklite', 'SuperGrok Lite'],
    ['plus', 'Plus'], ['pro', 'Pro'], ['team', 'Team'],
    ['supergrokpro', 'SuperGrokPro'],
    ['supergrok', 'SuperGrok'], ['supergrokplus', 'SuperGrok Plus'],
    ['supergrokheavy', 'SuperGrok Heavy'], ['heavy', 'Heavy'],
    ['xbasic', 'X Basic'], ['abnormal', 'Abnormal']
  ].map(([key, label]) => ({ value: `grok:${key}`, label: `Grok · ${label}` }))
]
