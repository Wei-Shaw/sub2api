import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const keyUsageViewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../KeyUsageView.vue'),
  'utf8',
)

describe('KeyUsageView theme control', () => {
  it('does not render the legacy dark-mode toggle', () => {
    expect(keyUsageViewSource).not.toContain('@click="toggleTheme"')
    expect(keyUsageViewSource).not.toContain('useThemeTransition')
    expect(keyUsageViewSource).not.toContain('home.switchToDark')
    expect(keyUsageViewSource).not.toContain('home.switchToLight')
  })
})
