<template>
  <BaseDialog
    :show="show"
    :title="station ? t('admin.upstreams.form.editTitle') : t('admin.upstreams.form.createTitle')"
    width="wide"
    @close="emit('close')"
  >
    <form id="upstream-station-form" class="space-y-5" @submit.prevent="submit">
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.name') }}</label>
          <input v-model="form.name" class="input" type="text" required maxlength="128" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.baseUrl') }}</label>
          <input v-model="form.base_url" class="input font-mono text-sm" type="url" required placeholder="https://api.example.com" />
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.siteType') }}</label>
          <Select v-model="form.site_type" :options="siteTypeOptions" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.credentialMode') }}</label>
          <div class="grid grid-cols-3 gap-2">
            <button
              v-for="option in credentialModeOptions"
              :key="option.value"
              type="button"
              class="mode-button"
              :class="form.credential_mode === option.value ? 'mode-button-active' : ''"
              @click="form.credential_mode = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="form.credential_mode === 'password'" class="grid gap-4 border-t border-gray-200 pt-5 dark:border-dark-700 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.username') }}</label>
          <input v-model="credentials.username" class="input" type="text" :required="!station" autocomplete="username" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.password') }}</label>
          <input v-model="credentials.password" class="input" type="password" :required="!station" autocomplete="new-password" :placeholder="secretPlaceholder" />
        </div>
      </div>

      <div v-else-if="form.credential_mode === 'token'" class="space-y-4 border-t border-gray-200 pt-5 dark:border-dark-700">
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.accessToken') }}</label>
          <input v-model="credentials.access_token" class="input font-mono text-sm" type="password" :required="!station" :placeholder="secretPlaceholder" />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.upstreams.form.userId') }}</label>
            <input v-model="credentials.user_id" class="input" type="text" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreams.form.refreshToken') }}</label>
            <input v-model="credentials.refresh_token" class="input font-mono text-sm" type="password" :placeholder="secretPlaceholder" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.cookie') }}</label>
          <input v-model="credentials.cookie" class="input font-mono text-sm" type="password" :placeholder="secretPlaceholder" />
        </div>
      </div>

      <div v-else class="space-y-4 border-t border-gray-200 pt-5 dark:border-dark-700">
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.apiKey') }}</label>
          <input v-model="credentials.api_key" class="input font-mono text-sm" type="password" :required="!station" :placeholder="secretPlaceholder" />
        </div>

        <div v-if="!station" class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreams.form.fixedRoute') }}</h3>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreams.form.fixedRouteHint') }}</p>
            </div>
            <span class="rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300">P = X / K</span>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.upstreams.form.platform') }}</label>
              <Select v-model="fixedRoute.platform" :options="platformOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.upstreams.form.groupName') }}</label>
              <input v-model="fixedRoute.remote_group_name" class="input" type="text" required />
            </div>
            <div>
              <label class="input-label">X</label>
              <input v-model.number="fixedRoute.group_rate" class="input font-mono" type="number" min="0" step="0.00000001" required />
            </div>
            <div>
              <label class="input-label">P</label>
              <div class="flex h-[42px] items-center rounded-lg border border-gray-200 bg-gray-50 px-3 font-mono text-sm font-semibold text-gray-800 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-100">
                {{ effectiveRatePreview }}
              </div>
            </div>
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstreams.form.models') }}</label>
            <textarea v-model="fixedModels" class="input min-h-20 font-mono text-sm" :placeholder="t('admin.upstreams.form.modelsPlaceholder')"></textarea>
          </div>
        </div>
      </div>

      <div class="grid gap-4 border-t border-gray-200 pt-5 dark:border-dark-700 sm:grid-cols-2">
        <div>
          <label class="input-label">K</label>
          <input v-model.number="form.recharge_multiplier" class="input font-mono" type="number" min="0.00000001" step="0.00000001" required />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreams.form.rechargeHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreams.form.rechargeSource') }}</label>
          <Select v-model="form.recharge_source" :options="rechargeSourceOptions" :disabled="form.credential_mode === 'api_key'" />
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <label class="toggle-row">
          <span>
            <span class="block text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('admin.upstreams.form.enabled') }}</span>
            <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreams.form.enabledHint') }}</span>
          </span>
          <Toggle v-model="form.enabled" />
        </label>
        <label class="toggle-row">
          <span>
            <span class="block text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('admin.upstreams.form.autoSync') }}</span>
            <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.upstreams.form.autoSyncHint') }}</span>
          </span>
          <Toggle v-model="form.auto_sync" />
        </label>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="upstream-station-form" class="btn btn-primary" :disabled="submitting">
          {{ submitting ? t('common.submitting') : station ? t('common.update') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type {
  CreateUpstreamStationParams,
  FixedRouteInput,
  UpstreamCredentialMode,
  UpstreamCredentials,
  UpstreamSiteType,
  UpstreamStation,
  UpdateUpstreamStationParams,
} from '@/api/admin/upstreamStations'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'

const props = defineProps<{ show: boolean; station: UpstreamStation | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()
const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)

const form = reactive({
  name: '',
  site_type: 'auto' as UpstreamSiteType,
  base_url: '',
  credential_mode: 'password' as UpstreamCredentialMode,
  recharge_multiplier: 1,
  recharge_source: 'manual' as 'manual' | 'auto',
  enabled: true,
  auto_sync: true,
})
const credentials = reactive<UpstreamCredentials>({})
const fixedRoute = reactive<FixedRouteInput>({
  remote_group_key: 'fixed',
  remote_group_name: 'Fixed',
  platform: 'openai',
  group_rate: 1,
  schedulable: true,
})
const fixedModels = ref('')

const siteTypeOptions = computed(() => [
  { value: 'auto', label: t('admin.upstreams.siteTypes.auto') },
  { value: 'newapi', label: 'NewAPI' },
  { value: 'sub2api', label: 'Sub2API' },
])
const credentialModeOptions = computed<{ value: UpstreamCredentialMode; label: string }[]>(() => [
  { value: 'password', label: t('admin.upstreams.credentialModes.password') },
  { value: 'token', label: 'Token' },
  { value: 'api_key', label: 'API Key' },
])
const platformOptions = computed(() => [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'grok', label: 'Grok' },
])
const rechargeSourceOptions = computed(() => [
  { value: 'manual', label: t('admin.upstreams.rechargeSources.manual') },
  { value: 'auto', label: t('admin.upstreams.rechargeSources.auto') },
])
const secretPlaceholder = computed(() => props.station ? t('admin.upstreams.form.secretUnchanged') : '')
const effectiveRatePreview = computed(() => {
  const k = Number(form.recharge_multiplier) > 0 ? Number(form.recharge_multiplier) : 1
  return (Number(fixedRoute.group_rate || 0) / k).toFixed(8).replace(/0+$/, '').replace(/\.$/, '')
})

function resetSecrets() {
  for (const key of Object.keys(credentials) as Array<keyof UpstreamCredentials>) delete credentials[key]
}

function resetForm(station: UpstreamStation | null) {
  resetSecrets()
  if (station) {
    form.name = station.name
    form.site_type = station.site_type
    form.base_url = station.base_url
    form.credential_mode = station.credential_mode
    form.recharge_multiplier = station.recharge_multiplier || 1
    form.recharge_source = station.recharge_source
    form.enabled = station.enabled
    form.auto_sync = station.auto_sync
  } else {
    form.name = ''
    form.site_type = 'auto'
    form.base_url = ''
    form.credential_mode = 'password'
    form.recharge_multiplier = 1
    form.recharge_source = 'manual'
    form.enabled = true
    form.auto_sync = true
    fixedRoute.remote_group_key = 'fixed'
    fixedRoute.remote_group_name = 'Fixed'
    fixedRoute.platform = 'openai'
    fixedRoute.group_rate = 1
    fixedRoute.schedulable = true
    fixedModels.value = ''
  }
}

watch(() => [props.show, props.station] as const, ([show, station]) => {
  if (show) resetForm(station)
}, { immediate: true })

watch(() => form.credential_mode, mode => {
  if (mode === 'api_key') form.recharge_source = 'manual'
})

function hasCredentialInput(): boolean {
  return Object.values(credentials).some(value => typeof value === 'string' && value.trim() !== '')
}

function normalizedCredentials(): UpstreamCredentials {
  const output: Record<string, string> = {}
  for (const [key, value] of Object.entries(credentials)) {
    if (typeof value === 'string' && value.trim()) output[key] = value.trim()
  }
  return output as UpstreamCredentials
}

function parseModels(): string[] {
  return [...new Set(fixedModels.value.split(/[\n,]+/).map(value => value.trim()).filter(Boolean))]
}

async function submit() {
  if (submitting.value) return
  if (!props.station && !hasCredentialInput()) {
    appStore.showError(t('admin.upstreams.form.credentialsRequired'))
    return
  }
  submitting.value = true
  try {
    if (props.station) {
      const payload: UpdateUpstreamStationParams = {
        name: form.name.trim(),
        site_type: form.site_type,
        base_url: form.base_url.trim(),
        credential_mode: form.credential_mode,
        recharge_multiplier: Number(form.recharge_multiplier),
        recharge_source: form.recharge_source,
        enabled: form.enabled,
        auto_sync: form.auto_sync,
      }
      if (hasCredentialInput()) payload.credentials = normalizedCredentials()
      await adminAPI.upstreamStations.update(props.station.id, payload)
      appStore.showSuccess(t('admin.upstreams.messages.updated'))
    } else {
      const payload: CreateUpstreamStationParams = {
        name: form.name.trim(),
        site_type: form.site_type,
        base_url: form.base_url.trim(),
        credential_mode: form.credential_mode,
        credentials: normalizedCredentials(),
        recharge_multiplier: Number(form.recharge_multiplier),
        recharge_source: form.recharge_source,
        enabled: form.enabled,
        auto_sync: form.auto_sync,
      }
      if (form.credential_mode === 'api_key') {
        payload.fixed_routes = [{
          ...fixedRoute,
          remote_group_key: fixedRoute.remote_group_key.trim() || 'fixed',
          remote_group_name: fixedRoute.remote_group_name.trim() || 'Fixed',
          models: parseModels(),
          group_rate: Number(fixedRoute.group_rate),
        }]
      }
      await adminAPI.upstreamStations.create(payload)
      appStore.showSuccess(t('admin.upstreams.messages.created'))
    }
    emit('saved')
    emit('close')
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.saveFailed')))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.mode-button {
  @apply min-h-10 rounded-lg border border-gray-200 bg-white px-2 py-2 text-xs font-medium text-gray-600 transition-colors hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300;
}

.mode-button-active {
  @apply border-primary-500 bg-primary-50 text-primary-700 ring-1 ring-primary-500 dark:border-primary-400 dark:bg-primary-500/10 dark:text-primary-300;
}

.toggle-row {
  @apply flex min-h-16 items-center justify-between gap-4 rounded-lg border border-gray-200 px-4 py-3 dark:border-dark-700;
}
</style>
