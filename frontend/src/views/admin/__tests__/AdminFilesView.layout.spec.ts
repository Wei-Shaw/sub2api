import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AdminFilesView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('AdminFilesView layout', () => {
  it('renders inside the standard app layout so the sidebar remains visible', () => {
    expect(viewSource).toMatch(/<template>\s*<AppLayout>/)
    expect(viewSource).toContain("import AppLayout from '@/components/layout/AppLayout.vue'")
  })

  it('places refresh, upload, URL import, and search in one toolbar before search', () => {
    const toolbarIndex = viewSource.indexOf('data-testid="file-toolbar"')
    const refreshIndex = viewSource.indexOf("t('common.refresh')")
    const uploadIndex = viewSource.indexOf("t('admin.files.upload')")
    const importIndex = viewSource.indexOf("t('admin.files.importUrl')")
    const searchIndex = viewSource.indexOf('v-model="searchInput"')

    expect(toolbarIndex).toBeGreaterThan(-1)
    expect(refreshIndex).toBeGreaterThan(-1)
    expect(uploadIndex).toBeGreaterThan(refreshIndex)
    expect(importIndex).toBeGreaterThan(uploadIndex)
    expect(searchIndex).toBeGreaterThan(importIndex)
    expect(viewSource.match(/btn btn-secondary btn-sm h-9/g)?.length).toBeGreaterThanOrEqual(4)
    expect(viewSource).toContain('class="input ml-auto h-9 max-w-xs py-1.5"')
  })

  it('places the root breadcrumb directly inside and above the file tree', () => {
    const listIndex = viewSource.indexOf('data-testid="file-list"')
    const breadcrumbsIndex = viewSource.indexOf('data-testid="directory-breadcrumbs"')
    const tableIndex = viewSource.indexOf('<div v-else class="overflow-x-auto">')

    expect(listIndex).toBeGreaterThan(-1)
    expect(breadcrumbsIndex).toBeGreaterThan(listIndex)
    expect(tableIndex).toBeGreaterThan(breadcrumbsIndex)
  })

  it('imports a URL into the active prefix without a modal', () => {
    expect(viewSource).toContain("adminFilesAPI.importFromUrl(url, {")
    expect(viewSource).toContain('prefix: prefix.value')
    expect(viewSource).not.toContain(':title="t(\'admin.files.importUrl\')"')
  })

  it('aligns URL import inputs and the submit button at the same height', () => {
    expect(viewSource.match(/class="input h-9 py-1\.5"/g)).toHaveLength(2)
    expect(viewSource).toContain('class="btn btn-primary btn-sm h-9 shrink-0 self-end"')
  })

  it('requires confirmation before overwriting a conflicting upload or URL import', () => {
    expect(viewSource).toContain("extractApiErrorCode(e) === 'OBJECT_KEY_EXISTS'")
    expect(viewSource).toContain("t('admin.files.overwriteConfirm'")
    expect(viewSource).toContain('overwrite: true')
  })

  it('keeps row actions on one line and scrolls the table horizontally when needed', () => {
    expect(viewSource).toContain('class="overflow-x-auto"')
    expect(viewSource).toContain('class="whitespace-nowrap px-3 py-2"')
    expect(viewSource).toContain('class="flex flex-nowrap items-center justify-end gap-1"')
  })

  it('accepts dropped files in the file list and reuses the upload pipeline', () => {
    expect(viewSource).toContain('@dragenter.prevent="handleDragEnter"')
    expect(viewSource).toContain('@dragover.prevent="handleDragOver"')
    expect(viewSource).toContain('@dragleave.prevent="handleDragLeave"')
    expect(viewSource).toContain('@drop.prevent="handleDrop"')
    expect(viewSource).toContain("t('admin.files.dropToUpload')")
    expect(viewSource).toContain('await uploadFiles(Array.from(ev.dataTransfer?.files ?? []))')
  })
})
