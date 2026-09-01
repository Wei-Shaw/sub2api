import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('Composite channel platform options', () => {
  it('includes the CN concrete providers for pricing and model mapping', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    const declaration = source.match(/const compositePlatforms:[^=]+=[^\n]+/)?.[0]

    expect(declaration).toContain("'kimi'")
    expect(declaration).toContain("'zhipu'")
    expect(declaration).toContain("'deepseek'")
  })

  it('adds MiniMax and MiMo only to direct channel platforms', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    const direct = source.match(/const platformOrder:[^=]+=[^\n]+/)?.[0]
    const composite = source.match(/const compositePlatforms:[^=]+=[^\n]+/)?.[0]

    expect(direct).toContain("'minimax'")
    expect(direct).toContain("'mimo'")
    expect(composite).not.toContain("'minimax'")
    expect(composite).not.toContain("'mimo'")
  })
})
