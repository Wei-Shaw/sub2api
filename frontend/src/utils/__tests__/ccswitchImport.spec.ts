import { describe, expect, it REDACTED from 'vitest'
import {
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
REDACTED from '@/utils/ccswitchImport'
import type { GroupPlatform REDACTED from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
REDACTED

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.5')
  REDACTED)

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  REDACTED

  it('adds the Codex model parameter for OpenAI imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'openai',
        clientType: 'claude'
      REDACTED)
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  REDACTED)

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' REDACTED,
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' REDACTED
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app REDACTED) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      REDACTED)
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
  REDACTED)

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      REDACTED)
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrlREDACTED/antigravity`)
    expect(params.has('model')).toBe(false)
  REDACTED)
REDACTED)
