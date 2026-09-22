import { describe, expect, it } from 'vitest'

import en from '../locales/en/admin/accounts'
import zh from '../locales/zh/admin/accounts'

describe('Codex Telemetry i18n', () => {
  it('keeps the switch label in English and describes it as a telemetry switch', () => {
    expect(zh.accounts.openai.codexTelemetry).toBe('Telemetry')
    expect(en.accounts.openai.codexTelemetry).toBe('Telemetry')
    expect(zh.accounts.openai.codexTelemetryDesc).toContain('遥测开关')
    expect(en.accounts.openai.codexTelemetryDesc).toContain('Telemetry switch')
  })

  it('does not promise anti-downgrade behavior', () => {
    expect(JSON.stringify(zh.accounts.openai)).not.toMatch(/抗降智/)
    expect(JSON.stringify(en.accounts.openai)).not.toMatch(/抗降智/)
  })
})
