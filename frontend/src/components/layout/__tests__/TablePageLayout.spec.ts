import { readFileSync REDACTED from 'node:fs'
import { dirname, resolve REDACTED from 'node:path'
import { fileURLToPath REDACTED from 'node:url'

import { describe, expect, it REDACTED from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../TablePageLayout.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('TablePageLayout responsive table scrolling', () => {
  it('does not disable the table horizontal scroll container in mobile mode', () => {
    const tableWrapperBlocks = Array.from(
      componentSource.matchAll(/([^{REDACTED]*:deep\(\.table-wrapper\)[^{REDACTED]*)\{([^{REDACTED]*)\REDACTED/g)
    )

    expect(tableWrapperBlocks.length).toBeGreaterThan(0)

    const baseBlock = tableWrapperBlocks.find(([selector]) => !selector.includes('.mobile-mode'))
    const mobileBlocks = tableWrapperBlocks.filter(([selector]) => selector.includes('.mobile-mode'))

    expect(baseBlock?.[2]).toContain('overflow-x-auto')
    expect(mobileBlocks.every(([, , declarations]) => !declarations.includes('overflow-visible'))).toBe(
      true
    )
  REDACTED)
REDACTED)
