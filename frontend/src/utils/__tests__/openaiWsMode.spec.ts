import { describe, expect, it REDACTED from 'vitest'
import {
  OPENAI_WS_MODE_DEDICATED,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_SHARED,
  isOpenAIWSModeEnabled,
  normalizeOpenAIWSMode,
  openAIWSModeFromEnabled,
  resolveOpenAIWSModeFromExtra
REDACTED from '@/utils/openaiWsMode'

describe('openaiWsMode utils', () => {
  it('normalizes mode values', () => {
    expect(normalizeOpenAIWSMode('off')).toBe(OPENAI_WS_MODE_OFF)
    expect(normalizeOpenAIWSMode(' Shared ')).toBe(OPENAI_WS_MODE_SHARED)
    expect(normalizeOpenAIWSMode('DEDICATED')).toBe(OPENAI_WS_MODE_DEDICATED)
    expect(normalizeOpenAIWSMode('invalid')).toBeNull()
  REDACTED)

  it('maps legacy enabled flag to mode', () => {
    expect(openAIWSModeFromEnabled(true)).toBe(OPENAI_WS_MODE_SHARED)
    expect(openAIWSModeFromEnabled(false)).toBe(OPENAI_WS_MODE_OFF)
    expect(openAIWSModeFromEnabled('true')).toBeNull()
  REDACTED)

  it('resolves by mode key first, then enabled, then fallback enabled keys', () => {
    const extra = {
      openai_oauth_responses_websockets_v2_mode: 'dedicated',
      openai_oauth_responses_websockets_v2_enabled: false,
      responses_websockets_v2_enabled: false
    REDACTED
    const mode = resolveOpenAIWSModeFromExtra(extra, {
      modeKey: 'openai_oauth_responses_websockets_v2_mode',
      enabledKey: 'openai_oauth_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled']
    REDACTED)
    expect(mode).toBe(OPENAI_WS_MODE_DEDICATED)
  REDACTED)

  it('falls back to default when nothing is present', () => {
    const mode = resolveOpenAIWSModeFromExtra({REDACTED, {
      modeKey: 'openai_apikey_responses_websockets_v2_mode',
      enabledKey: 'openai_apikey_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
      defaultMode: OPENAI_WS_MODE_OFF
    REDACTED)
    expect(mode).toBe(OPENAI_WS_MODE_OFF)
  REDACTED)

  it('treats off as disabled and shared/dedicated as enabled', () => {
    expect(isOpenAIWSModeEnabled(OPENAI_WS_MODE_OFF)).toBe(false)
    expect(isOpenAIWSModeEnabled(OPENAI_WS_MODE_SHARED)).toBe(true)
    expect(isOpenAIWSModeEnabled(OPENAI_WS_MODE_DEDICATED)).toBe(true)
  REDACTED)
REDACTED)
