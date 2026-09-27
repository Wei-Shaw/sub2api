import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const styleSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css'), 'utf8')

describe('.btn-icon touch target', () => {
  it('is at least 40px on coarse pointers', () => {
    expect(styleSource).toMatch(
      /@media \(pointer: coarse\) \{\s*\.btn-icon \{\s*min-width: 2\.5rem;\s*min-height: 2\.5rem;/
    )
  })
})
