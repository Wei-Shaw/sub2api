import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('GroupsView Composite route options', () => {
  it('offers Kimi, Zhipu GLM, and DeepSeek as route targets', () => {
    const source = readFileSync(resolve('src/views/admin/GroupsView.vue'), 'utf8')
    const options = source.slice(
      source.indexOf('const compositeRoutePlatformOptions'),
      source.indexOf('const compositeRouteEndpointOptions')
    )

    expect(options).toContain('{ value: "kimi", label: "Kimi" }')
    expect(options).toContain('{ value: "zhipu", label: "Zhipu GLM" }')
    expect(options).toContain('{ value: "deepseek", label: "DeepSeek" }')
  })
})
