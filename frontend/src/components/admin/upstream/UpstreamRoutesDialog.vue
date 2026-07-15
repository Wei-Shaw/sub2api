<template>
  <BaseDialog
    :show="show"
    :title="t('admin.upstreams.routes.title', { name: station?.name || '' })"
    width="full"
    @close="emit('close')"
  >
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-4 text-sm text-gray-600 dark:text-dark-300">
          <span>{{ t('admin.upstreams.routes.total', { count: routes.length }) }}</span>
          <span v-if="lowestRate != null" class="font-mono font-semibold text-emerald-700 dark:text-emerald-400">
            {{ t('admin.upstreams.routes.lowest') }} {{ formatRate(lowestRate) }}
          </span>
        </div>
        <div class="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
          <SearchInput v-model="modelQuery" class="w-full sm:w-64" :placeholder="t('admin.upstreams.routes.modelSearch')" />
          <button
            v-if="station?.credential_mode === 'api_key'"
            type="button"
            class="btn btn-secondary whitespace-nowrap"
            @click="beginCreate"
          >
            <Icon name="plus" size="sm" />
            {{ t('admin.upstreams.routes.add') }}
          </button>
        </div>
      </div>

      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-dark-900 dark:text-dark-300">
            <tr>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.group') }}</th>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.protocol') }}</th>
              <th class="px-4 py-3 font-mono">X</th>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.rechargeMultiplier') }}</th>
              <th class="px-4 py-3 font-mono">P</th>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.models') }}</th>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.account') }}</th>
              <th class="px-4 py-3">{{ t('admin.upstreams.routes.status') }}</th>
              <th class="px-4 py-3 text-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
            <tr v-if="loading">
              <td colspan="9" class="px-4 py-10 text-center text-gray-500">{{ t('common.loading') }}</td>
            </tr>
            <tr v-else-if="filteredRoutes.length === 0">
              <td colspan="9" class="px-4 py-10 text-center text-gray-500">{{ t('admin.upstreams.routes.empty') }}</td>
            </tr>
            <tr v-for="route in filteredRoutes" v-else :key="route.id" class="text-gray-700 dark:text-gray-200">
              <td class="max-w-52 px-4 py-3">
                <div class="truncate font-medium text-gray-900 dark:text-white">{{ route.remote_group_name }}</div>
                <div class="truncate font-mono text-[11px] text-gray-400">{{ route.remote_group_key }}</div>
              </td>
              <td class="px-4 py-3"><span :class="platformBadge(route.platform)">{{ platformLabel(route.platform) }}</span></td>
              <td class="px-4 py-3 font-mono">{{ formatRate(route.group_rate) }}</td>
              <td class="px-4 py-3 font-mono">{{ formatRate(route.recharge_multiplier) }}</td>
              <td class="px-4 py-3 font-mono font-semibold text-emerald-700 dark:text-emerald-400">{{ formatRate(route.effective_rate) }}</td>
              <td class="px-4 py-3">
                <span class="font-medium">{{ route.models?.length || 0 }}</span>
                <div v-if="route.models?.length" class="mt-0.5 max-w-56 truncate text-[11px] text-gray-400" :title="route.models.join(', ')">
                  {{ route.models.slice(0, 3).join(', ') }}
                </div>
              </td>
              <td class="px-4 py-3 font-mono text-xs">{{ route.managed_account_id || '—' }}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span :class="healthDot(route.health_status)"></span>
                  <span class="text-xs">{{ healthLabel(route.health_status) }}</span>
                  <Toggle :model-value="route.schedulable" @update:model-value="toggleRoute(route, $event)" />
                </div>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button class="icon-button" type="button" :title="t('common.test')" :disabled="busyRouteId === route.id" @click="testRoute(route)">
                    <Icon name="play" size="sm" />
                  </button>
                  <button class="icon-button" type="button" :title="t('common.edit')" @click="beginEdit(route)">
                    <Icon name="edit" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <form v-if="editing" class="space-y-4 border-t border-gray-200 pt-5 dark:border-dark-700" @submit.prevent="saveRoute">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ editing.id ? t('admin.upstreams.routes.edit') : t('admin.upstreams.routes.add') }}
          </h3>
          <button type="button" class="icon-button" :title="t('common.close')" @click="editing = null"><Icon name="x" size="sm" /></button>
        </div>
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div v-if="!editing.id">
            <label class="input-label">{{ t('admin.upstreams.routes.key') }}</label>
            <input v-model="routeForm.remote_group_key" class="input font-mono" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreams.routes.group') }}</label>
            <input v-model="routeForm.remote_group_name" class="input" required />
          </div>
          <div v-if="!editing.id">
            <label class="input-label">{{ t('admin.upstreams.routes.protocol') }}</label>
            <Select v-model="routeForm.platform" :options="platformOptions" />
          </div>
          <div>
            <label class="input-label">X</label>
            <input v-model.number="routeForm.group_rate" class="input font-mono" type="number" min="0" step="0.00000001" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreams.routes.rechargeMultiplier') }}</label>
            <div class="flex h-[42px] items-center rounded-lg border border-gray-200 bg-gray-50 px-3 font-mono text-sm text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200">
              {{ formatRate(station?.recharge_multiplier || 1) }}
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreams.routes.kInherited') }}</p>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.routes.models') }}</label>
          <textarea v-model="routeForm.models" class="input min-h-20 font-mono text-sm"></textarea>
        </div>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="editing = null">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </form>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { UpstreamHealth, UpstreamPlatform, UpstreamRoute, UpstreamStation } from '@/api/admin/upstreamStations'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; station: UpstreamStation | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'changed'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const routes = ref<UpstreamRoute[]>([])
const loading = ref(false)
const saving = ref(false)
const busyRouteId = ref<number | null>(null)
const modelQuery = ref('')
const editing = ref<UpstreamRoute | { id: 0 } | null>(null)
const routeForm = reactive({
  remote_group_key: 'fixed',
  remote_group_name: 'Fixed',
  platform: 'openai' as UpstreamPlatform,
  group_rate: 1,
  models: '',
})

const lowestRate = computed(() => {
  const values = routes.value.filter(route => route.schedulable && route.health_status !== 'error').map(route => route.effective_rate)
  return values.length ? Math.min(...values) : null
})
const filteredRoutes = computed(() => {
  const query = modelQuery.value.trim().toLowerCase()
  if (!query) return routes.value
  return routes.value.filter(route =>
    route.remote_group_name.toLowerCase().includes(query)
    || route.remote_group_key.toLowerCase().includes(query)
    || (route.models || []).some(model => model.toLowerCase().includes(query)),
  )
})
const platformOptions = computed(() => [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'grok', label: 'Grok' },
])

watch(() => [props.show, props.station?.id] as const, ([show]) => {
  if (show) {
    modelQuery.value = ''
    void loadRoutes()
  }
  else editing.value = null
}, { immediate: true })

async function loadRoutes() {
  if (!props.station) return
  loading.value = true
  try {
    routes.value = await adminAPI.upstreamStations.listRoutes(props.station.id)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.routesFailed')))
  } finally {
    loading.value = false
  }
}

function beginCreate() {
  if (!props.station) return
  editing.value = { id: 0 }
  routeForm.remote_group_key = `fixed-${routes.value.length + 1}`
  routeForm.remote_group_name = `Fixed ${routes.value.length + 1}`
  routeForm.platform = 'openai'
  routeForm.group_rate = 1
  routeForm.models = ''
}

function beginEdit(route: UpstreamRoute) {
  editing.value = route
  routeForm.remote_group_key = route.remote_group_key
  routeForm.remote_group_name = route.remote_group_name
  routeForm.platform = route.platform
  routeForm.group_rate = route.group_rate
  routeForm.models = (route.models || []).join('\n')
}

function parseModels(): string[] {
  return [...new Set(routeForm.models.split(/[\n,]+/).map(value => value.trim()).filter(Boolean))]
}

async function saveRoute() {
  if (!editing.value || !props.station || saving.value) return
  saving.value = true
  try {
    if (editing.value.id) {
      await adminAPI.upstreamStations.updateRoute(editing.value.id, {
        remote_group_name: routeForm.remote_group_name.trim(),
        group_rate: Number(routeForm.group_rate),
        models: parseModels(),
      })
    } else {
      await adminAPI.upstreamStations.createFixedRoute(props.station.id, {
        remote_group_key: routeForm.remote_group_key.trim(),
        remote_group_name: routeForm.remote_group_name.trim(),
        platform: routeForm.platform,
        group_rate: Number(routeForm.group_rate),
        models: parseModels(),
        schedulable: true,
      })
    }
    editing.value = null
    await loadRoutes()
    emit('changed')
    appStore.showSuccess(t('admin.upstreams.messages.routeSaved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.routeSaveFailed')))
  } finally {
    saving.value = false
  }
}

async function toggleRoute(route: UpstreamRoute, value: boolean) {
  const previous = route.schedulable
  route.schedulable = value
  try {
    await adminAPI.upstreamStations.setRouteSchedulable(route.id, value)
    emit('changed')
  } catch (error: unknown) {
    route.schedulable = previous
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  }
}

async function testRoute(route: UpstreamRoute) {
  if (busyRouteId.value != null) return
  busyRouteId.value = route.id
  try {
    await adminAPI.upstreamStations.testRoute(route.id)
    await loadRoutes()
    emit('changed')
    appStore.showSuccess(t('admin.upstreams.messages.testSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.testFailed')))
  } finally {
    busyRouteId.value = null
  }
}

function formatRate(value: number): string {
  return Number(value || 0).toFixed(8).replace(/0+$/, '').replace(/\.$/, '') || '0'
}
function platformLabel(platform: UpstreamPlatform): string {
  return platform === 'anthropic' ? 'Claude' : platform === 'openai' ? 'OpenAI' : platform === 'gemini' ? 'Gemini' : 'Grok'
}
function platformBadge(platform: UpstreamPlatform): string {
  const base = 'inline-flex rounded-md px-2 py-1 text-xs font-medium'
  if (platform === 'openai') return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300`
  if (platform === 'anthropic') return `${base} bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300`
  if (platform === 'gemini') return `${base} bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300`
  return `${base} bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200`
}
function healthDot(health: UpstreamHealth): string {
  return health === 'healthy' ? 'h-2 w-2 rounded-full bg-emerald-500' : health === 'error' ? 'h-2 w-2 rounded-full bg-red-500' : 'h-2 w-2 rounded-full bg-gray-400'
}
function healthLabel(health: UpstreamHealth): string {
  return t(`admin.upstreams.health.${health}`)
}
</script>

<style scoped>
.icon-button {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white;
}
</style>
