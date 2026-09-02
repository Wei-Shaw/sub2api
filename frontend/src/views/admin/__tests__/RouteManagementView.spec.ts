import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../RouteManagementView.vue')
const sidebarPath = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../components/layout/AppSidebar.vue'
)
const groupsPath = resolve(dirname(fileURLToPath(import.meta.url)), '../GroupsView.vue')

const viewSource = readFileSync(viewPath, 'utf8')
const sidebarSource = readFileSync(sidebarPath, 'utf8')
const groupsSource = readFileSync(groupsPath, 'utf8')

describe('Route Management placement', () => {
  it('registers route management as a child of group management in the sidebar', () => {
    expect(sidebarSource).toContain("path: '/admin/groups/routes'")
    expect(sidebarSource).toContain("label: t('nav.routeManagement')")
    expect(sidebarSource).toContain("label: t('nav.groupList')")
  })

  it('lets Composite groups pick a route scheme instead of editing routes inline', () => {
    expect(groupsSource).toContain('createForm.composite_route_scheme_id')
    expect(groupsSource).toContain('editForm.composite_route_scheme_id')
    expect(groupsSource).toContain('to="/admin/groups/routes"')
    expect(groupsSource).not.toContain('handleCompositeRoutes')
    expect(groupsSource).not.toContain('showCompositeRoutesModal')
  })

  it('creates schemes from an existing scheme as a template', () => {
    expect(viewSource).toContain('copy_from_scheme_id')
    expect(viewSource).toContain('adminAPI.routeSchemes.duplicate')
    expect(viewSource).toContain('CompositeRouteEditor')
  })

  it('shows a scheme list first and only opens the editor from edit or create', () => {
    expect(viewSource).toContain('v-if="!editingScheme"')
    expect(viewSource).toContain('openDetail(row)')
    expect(viewSource).toContain('openCreateDialog')
    expect(viewSource).toContain('key: "name"')
    expect(viewSource).toContain('key: "actions"')
    expect(viewSource).toContain('<CompositeRouteEditor :scheme-id="editingScheme.id"')
    const listBlock = viewSource.slice(
      viewSource.indexOf('v-if="!editingScheme"'),
      viewSource.indexOf('v-else class="space-y-5"')
    )
    expect(listBlock).toContain('DataTable')
    expect(listBlock).not.toContain('CompositeRouteEditor')
  })
})
