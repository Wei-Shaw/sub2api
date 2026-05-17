import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const testDir = dirname(fileURLToPath(import.meta.url))
const keysViewSource = readFileSync(resolve(testDir, '../KeysView.vue'), 'utf8')
const styleSource = readFileSync(resolve(testDir, '../../../style.css'), 'utf8')
const tableLayoutSource = readFileSync(
  resolve(testDir, '../../../components/layout/TablePageLayout.vue'),
  'utf8',
)
const dataTableSource = readFileSync(resolve(testDir, '../../../components/common/DataTable.vue'), 'utf8')
const selectSource = readFileSync(resolve(testDir, '../../../components/common/Select.vue'), 'utf8')
const appLayoutSource = readFileSync(resolve(testDir, '../../../components/layout/AppLayout.vue'), 'utf8')
const appHeaderSource = readFileSync(resolve(testDir, '../../../components/layout/AppHeader.vue'), 'utf8')

function styleBlock(source: string, selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return source.match(new RegExp(`${escaped}\\s*\\{[\\s\\S]*?\\n {2}\\}`))?.[0] ?? ''
}

describe('console visual refresh contract', () => {
  it('uses slate/black global controls instead of mint primary controls', () => {
    expect(styleBlock(styleSource, '.btn-primary')).toContain('bg-slate-950')
    expect(styleBlock(styleSource, '.btn-primary')).not.toContain('bg-primary')

    expect(styleBlock(styleSource, '.input')).toContain('bg-white')
    expect(styleBlock(styleSource, '.input')).toContain('border-slate-200')
    expect(styleBlock(styleSource, '.input')).toContain('focus:border-slate-950')
    expect(styleBlock(styleSource, '.input')).not.toContain('focus:border-cyan')
  })

  it('keeps sidebar active state neutral with a dark left indicator', () => {
    const activeBlock = styleBlock(styleSource, '.sidebar-link-active')

    expect(activeBlock).toContain('bg-slate-100')
    expect(activeBlock).toContain('text-slate-950')
    expect(activeBlock).toContain('border-l-2')
    expect(activeBlock).not.toContain('primary')
  })

  it('renders tables with quiet transparent headers and soft row dividers', () => {
    expect(tableLayoutSource).toContain('rounded-lg border border-slate-200')
    expect(tableLayoutSource).toContain('bg-white')
    expect(tableLayoutSource).toContain('text-xs font-semibold uppercase tracking-wider text-slate-500')
    expect(tableLayoutSource).toContain('border-b border-slate-100')
    expect(tableLayoutSource).not.toContain('thead) {\n  @apply bg-gray-50')

    expect(dataTableSource).toContain('thead class="table-header bg-white dark:bg-dark-900"')
    expect(dataTableSource).toContain('tbody class="table-body divide-y divide-slate-100 bg-white')
  })

  it('keeps select focus states black instead of blue or green', () => {
    expect(selectSource).toContain('focus:border-slate-950')
    expect(selectSource).toContain('ring-slate-950/10')
    expect(selectSource).not.toContain('focus:border-primary-500')
    expect(selectSource).not.toContain('ring-primary-500')
    expect(selectSource).not.toContain('text-primary-500')
  })

  it('keeps the console shell and header account chrome neutral', () => {
    expect(appLayoutSource).toContain('bg-[#F8FAFC]')
    expect(appLayoutSource).not.toContain('bg-gray-50')

    expect(appHeaderSource).toContain('bg-slate-100')
    expect(appHeaderSource).toContain('text-slate-700')
    expect(appHeaderSource).toContain('from-slate-900 to-black')
    expect(appHeaderSource).not.toContain('bg-primary-50')
    expect(appHeaderSource).not.toContain('from-primary-500')
  })
})

describe('API keys table visual refresh contract', () => {
  it('renders API keys as neutral code text and status as dot labels', () => {
    expect(keysViewSource).toContain('api-key-code')
    expect(keysViewSource).toContain('api-key-status')
    expect(keysViewSource).toContain('api-key-status-dot')
    expect(keysViewSource).not.toContain("'badge-success'")
  })

  it('uses subdued row actions with visible labels and accessible titles', () => {
    expect(keysViewSource).toContain('api-key-actions')
    expect(keysViewSource).toContain('api-key-action')
    expect(keysViewSource).toContain('api-key-action-label')
    expect(keysViewSource).not.toContain('sr-only')
    expect(keysViewSource).not.toContain('hover:bg-green-50')
    expect(keysViewSource).not.toContain('hover:bg-blue-50')
    expect(keysViewSource).not.toContain('hover:text-primary-600')
  })
})
