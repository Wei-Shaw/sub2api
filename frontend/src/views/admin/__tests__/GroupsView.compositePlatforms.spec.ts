import { readFileSync REDACTED from 'node:fs'
import { resolve REDACTED from 'node:path'
import { describe, expect, it REDACTED from 'vitest'

describe('GroupsView Composite route options', () => {
  it('offers Kimi, Zhipu GLM, and DeepSeek as route targets', () => {
    const source = readFileSync(resolve('src/views/admin/GroupsView.vue'), 'utf8')
    const options = source.slice(
      source.indexOf('const compositeRoutePlatformOptions'),
      source.indexOf('const compositeRouteEndpointOptions')
    )

    expect(options).toContain('{ value: "kimi", label: "Kimi" REDACTED')
    expect(options).toContain('{ value: "zhipu", label: "Zhipu GLM" REDACTED')
    expect(options).toContain('{ value: "deepseek", label: "DeepSeek" REDACTED')
  REDACTED)
REDACTED)
