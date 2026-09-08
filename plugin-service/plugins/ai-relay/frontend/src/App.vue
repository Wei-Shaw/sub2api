<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { RelayApi } from './api'
import { canonicalPath, mappingRecordFromRows, mappingRowsFromRecord } from './pathMappings'
import Toast from './Toast.vue'
import type { MappingRow, Platform, RelayRoute } from './types'

type CopyToast = { id: number; type: 'success' | 'error'; message: string; duration: number }

const openAISourcePathSuggestions = [
  'v1/models',
  'v1/chat/completions',
  'v1/responses',
  'v1/responses/compact',
  'v1/embeddings',
  'v1/images/generations',
  'v1/images/edits',
]

const props = defineProps<{ api: RelayApi }>()
const routes = ref<RelayRoute[]>([])
const platforms = ref<Platform[]>([])
const loading = ref(false)
const errorMessage = ref('')
const unauthorized = ref(false)
const search = ref('')
const platform = ref('')
const selected = ref<Set<string>>(new Set())
const page = ref(1)
const pageSize = ref(20)
const pagination = reactive({ page: 1, page_size: 20, total: 0, total_pages: 1 })
const dialogOpen = ref(false)
const editing = ref<RelayRoute | null>(null)
const deleteOpen = ref(false)
const formError = ref('')
const form = reactive({ name: '', platform: '', slug: '', base_url: 'https://apihub.agnes-ai.com/v1' })
const mappingRows = ref<MappingRow[]>([])
const runtimeBaseURL = ref('http://127.0.0.1:8091')
const copyToast = ref<CopyToast | null>(null)
let nextID = 1
let nextToastID = 1

const selectedRoutes = computed(() => routes.value.filter(route => selected.value.has(routeKey(route))))
const allSelected = computed(() => routes.value.length > 0 && routes.value.every(route => selected.value.has(routeKey(route))))
function routeKey(route: Pick<RelayRoute, 'platform' | 'slug'>) { return `${route.platform}:${route.slug}` }
function relayURL(route: RelayRoute) { return `${runtimeBaseURL.value}/plugins/ai-relay/${route.platform}/${route.slug}` }
function defaultBaseURL(platformKey: string) { return platforms.value.find(item => item.key === platformKey)?.default_base_url || '' }
function setError(error: unknown) {
  const status = typeof error === 'object' && error !== null && 'status' in error ? Number(error.status) : 0
  unauthorized.value = status === 401 || status === 403
  errorMessage.value = error instanceof Error ? error.message : String(error)
}
async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [runtime, availablePlatforms, result] = await Promise.all([
      props.api.getRuntime().catch(() => null),
      props.api.listPlatforms(),
      props.api.listRoutes({ page: page.value, page_size: pageSize.value, platform: platform.value, search: search.value }),
    ])
    const configuredBaseURL = runtime?.base_url?.trim().replace(/\/+$/, '')
    if (configuredBaseURL) runtimeBaseURL.value = configuredBaseURL
    platforms.value = availablePlatforms
    routes.value = result.items || []
    Object.assign(pagination, result.pagination)
    selected.value = new Set()
  } catch (error) {
    setError(error)
  } finally {
    loading.value = false
  }
}
function toggleAll() {
  selected.value = allSelected.value ? new Set() : new Set(routes.value.map(routeKey))
}
function toggleSelected(route: RelayRoute) {
  const next = new Set(selected.value)
  const key = routeKey(route)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  selected.value = next
}
function openCreate() {
  editing.value = null
  formError.value = ''
  const platformKey = platforms.value[0]?.key || 'agnes'
  Object.assign(form, { name: '', platform: platformKey, slug: String(Date.now()), base_url: defaultBaseURL(platformKey) || 'https://apihub.agnes-ai.com/v1' })
  mappingRows.value = []
  dialogOpen.value = true
}
function updateFormPlatform() {
  if (!editing.value) form.base_url = defaultBaseURL(form.platform) || form.base_url
}
function openEdit(route: RelayRoute) {
  editing.value = route
  formError.value = ''
  Object.assign(form, { name: route.name, platform: route.platform, slug: route.slug, base_url: route.base_url })
  mappingRows.value = mappingRowsFromRecord(route.path_mappings, () => nextID++)
  dialogOpen.value = true
}
function addMapping() { mappingRows.value.push({ id: nextID++, source: '', target: '' }) }
function removeMapping(index: number) { mappingRows.value.splice(index, 1) }
async function save() {
  formError.value = ''
  const normalizedMappings = mappingRecordFromRows(mappingRows.value)
  const nextSlug = form.slug.trim().toLowerCase()
  const payload = { ...form, slug: nextSlug, path_mappings: normalizedMappings }
  try {
    if (editing.value) {
      if (nextSlug !== editing.value.slug) {
        await props.api.deleteRoutes([{ platform: editing.value.platform, slug: editing.value.slug }])
        await props.api.createRoute(payload)
      } else {
        await props.api.updateRoute(editing.value.platform, editing.value.slug, payload)
      }
    } else await props.api.createRoute(payload)
    dialogOpen.value = false
    page.value = 1
    await load()
  } catch (error) {
    formError.value = error instanceof Error ? error.message : String(error)
  }
}
async function deleteSelected() {
  try {
    const routeReferences = selectedRoutes.value.map(route => ({ platform: route.platform, slug: route.slug }))
    await props.api.deleteRoutes(routeReferences)
    deleteOpen.value = false
    await load()
  } catch (error) {
    setError(error)
  }
}
function confirmDeleteRoute(route: RelayRoute) {
  selected.value = new Set([routeKey(route)])
  deleteOpen.value = true
}
async function copyRouteURL(route: RelayRoute) {
  try {
    const url = relayURL(route)
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = url
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const copied = document.execCommand('copy')
      textarea.remove()
      if (!copied) throw new Error('Legacy Clipboard API is unavailable')
    }
    copyToast.value = { id: nextToastID++, type: 'success', message: 'Plugin URL copied', duration: 3000 }
  } catch {
    copyToast.value = { id: nextToastID++, type: 'error', message: 'Failed to copy Plugin URL', duration: 5000 }
  }
}
function updateSearch() { page.value = 1; void load() }
function updatePage(nextPage: number) { page.value = Math.max(1, nextPage); void load() }
function updatePageSize(event: Event) { pageSize.value = Number((event.target as HTMLSelectElement).value); page.value = 1; void load() }
onMounted(load)

defineExpose({ canonicalPath })
</script>

<template>
  <main class="relay-shell">
    <header class="relay-header">
      <div>
        <p class="relay-eyebrow">PLUGIN SERVICE</p>
        <h1>AI Relay</h1>
        <p class="relay-subtitle">Manage upstream routes and endpoint path mappings.</p>
      </div>
      <div class="relay-header-actions">
        <button type="button" class="icon-button" aria-label="Refresh routes" :disabled="loading" @click="load">↻</button>
        <button type="button" class="primary-button" data-testid="route-add" @click="openCreate">＋ Add route</button>
      </div>
    </header>

    <section class="relay-toolbar" aria-label="Route filters">
      <label>Search <input v-model="search" type="search" placeholder="Name or slug" @change="updateSearch" /></label>
      <label>Platform
        <select v-model="platform" @change="updateSearch"><option value="">All platforms</option><option v-for="item in platforms" :key="item.key" :value="item.key">{{ item.display_name }}</option></select>
      </label>
      <span class="route-count">{{ pagination.total }} routes</span>
    </section>

    <p v-if="errorMessage" class="relay-alert" role="alert">{{ unauthorized ? 'Administrator access is required. ' : '' }}{{ errorMessage }}</p>
    <section v-if="selectedRoutes.length" class="selection-bar">
      <span>{{ selectedRoutes.length }} selected</span>
      <button type="button" class="danger-button" data-testid="route-delete-selected" @click="deleteOpen = true">Delete selected</button>
    </section>

    <section class="relay-table-wrap" aria-live="polite">
      <div v-if="loading" class="relay-state">Loading routes…</div>
      <div v-else-if="!routes.length" class="relay-state"><strong>No routes configured</strong><span>Add a route to start forwarding requests.</span><button type="button" class="secondary-button" @click="openCreate">Add route</button></div>
      <table v-else class="relay-table">
        <thead><tr><th><input type="checkbox" :checked="allSelected" aria-label="Select all routes" @change="toggleAll" /></th><th>Name</th><th>Platform</th><th>Target URL</th><th>Plugin URL</th><th>Mappings</th><th>Actions</th></tr></thead>
        <tbody>
          <tr v-for="route in routes" :key="routeKey(route)">
            <td><input type="checkbox" :checked="selected.has(routeKey(route))" :aria-label="`Select ${route.name || route.slug}`" @change="toggleSelected(route)" /></td>
            <td data-testid="route-name"><strong>{{ route.name || route.slug }}</strong></td>
            <td>{{ route.platform }}</td>
            <td><code>{{ route.base_url }}</code></td>
            <td data-testid="route-plugin-url"><code>{{ relayURL(route) }}</code></td>
            <td><span class="mapping-badge">{{ Object.keys(route.path_mappings || {}).length }}</span></td>
            <td class="action-cell"><button type="button" class="icon-button" aria-label="Copy route URL" @click="copyRouteURL(route)">⧉</button><button type="button" class="icon-button" data-testid="route-edit" aria-label="Edit route" @click="openEdit(route)">✎</button><button type="button" class="icon-button" data-testid="route-delete" aria-label="Delete route" @click="confirmDeleteRoute(route)">×</button></td>
          </tr>
        </tbody>
      </table>
    </section>
    <nav v-if="pagination.total" class="relay-pagination" aria-label="Route pagination"><button type="button" class="secondary-button" :disabled="page <= 1" @click="updatePage(page - 1)">Previous</button><span>Page {{ page }} of {{ pagination.total_pages }}</span><label>Rows <select :value="pageSize" @change="updatePageSize"><option :value="20">20</option><option :value="50">50</option><option :value="100">100</option></select></label><button type="button" class="secondary-button" :disabled="page >= pagination.total_pages" @click="updatePage(page + 1)">Next</button></nav>

    <div v-if="dialogOpen" class="dialog-backdrop" role="presentation" @click.self="dialogOpen = false">
      <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="route-dialog-title">
        <div class="dialog-heading"><h2 id="route-dialog-title">{{ editing ? 'Edit route' : 'Add route' }}</h2><button type="button" class="icon-button" aria-label="Close dialog" @click="dialogOpen = false">×</button></div>
        <form @submit.prevent="save">
          <div class="form-grid"><label>Name<input v-model.trim="form.name" required maxlength="80" /></label><label>Platform<select v-model="form.platform" :disabled="!!editing" required @change="updateFormPlatform"><option v-for="item in platforms" :key="item.key" :value="item.key">{{ item.display_name }}</option></select></label><label>Slug<input v-model.trim="form.slug" required pattern="[a-z0-9][a-z0-9_-]{0,63}" /></label><label>Target Base URL<input v-model.trim="form.base_url" type="url" required /></label></div>
          <div class="mapping-editor"><div class="mapping-heading"><div><h3>Path mappings</h3><p>Map request paths relative to the Target Base URL.</p></div><button type="button" class="secondary-button" data-testid="path-mapping-add" @click="addMapping">＋ Add mapping</button></div><datalist id="openai-source-paths"><option v-for="path in openAISourcePathSuggestions" :key="path" :value="path" /></datalist><div v-for="(mapping, index) in mappingRows" :key="mapping.id" class="mapping-row"><label :for="`source-${mapping.id}`">Source path<input :id="`source-${mapping.id}`" v-model="mapping.source" list="openai-source-paths" data-testid="path-mapping-source" placeholder="v1/chat/completions" /></label><span class="mapping-arrow" aria-hidden="true">→</span><label :for="`target-${mapping.id}`">Target path<input :id="`target-${mapping.id}`" v-model="mapping.target" data-testid="path-mapping-target" placeholder="chat/completions" /></label><button type="button" class="icon-button" aria-label="Remove path mapping" data-testid="path-mapping-remove" @click="removeMapping(index)">×</button></div></div>
          <p v-if="formError" class="relay-alert" role="alert">{{ formError }}</p>
          <div class="dialog-actions"><button type="button" class="secondary-button" @click="dialogOpen = false">Cancel</button><button type="submit" class="primary-button" data-testid="route-save">Save route</button></div>
        </form>
      </section>
    </div>
    <div v-if="deleteOpen" class="dialog-backdrop" role="presentation"><section class="dialog narrow" role="alertdialog" aria-modal="true" aria-labelledby="delete-title"><h2 id="delete-title">Delete selected routes?</h2><p>This action cannot be undone.</p><div class="dialog-actions"><button type="button" class="secondary-button" @click="deleteOpen = false">Cancel</button><button type="button" class="danger-button" data-testid="route-delete-confirm" @click="deleteSelected">Delete</button></div></section></div>
    <Toast v-if="copyToast" :key="copyToast.id" :type="copyToast.type" :message="copyToast.message" :duration="copyToast.duration" @dismiss="copyToast = null" />
  </main>
</template>
