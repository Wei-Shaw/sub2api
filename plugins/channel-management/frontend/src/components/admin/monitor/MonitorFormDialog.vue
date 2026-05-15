<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.name') }} <span class="text-red-500">*</span></label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.channelMonitor.form.namePlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.provider') }} <span class="text-red-500">*</span></label>
        <div class="grid grid-cols-3 gap-3">
          <button
            v-for="opt in providerOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="form.provider === opt.value"
            class="flex items-center justify-center gap-2 rounded-lg border-2 px-3 py-2.5 text-sm font-medium transition-colors"
            :class="providerPickerClass(opt.value, form.provider === opt.value)"
            @click="form.provider = opt.value"
          >
            <PlatformIcon :platform="opt.value" size="sm" />
            <span>{{ opt.label }}</span>
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.endpoint') }} <span class="text-red-500">*</span></label>
        <div class="flex gap-2">
          <input
            v-model="form.endpoint"
            type="text"
            required
            class="input flex-1"
            :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')"
          />
          <button type="button" @click="useCurrentDomain" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useCurrentDomain') }}
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.apiKey') }}<span v-if="!editing" class="text-red-500"> *</span>
        </label>
        <div class="flex gap-2">
          <input
            v-model="form.api_key"
            type="password"
            autocomplete="new-password"
            data-1p-ignore
            data-lpignore="true"
            data-bwignore="true"
            :required="!editing"
            class="input flex-1 font-mono"
            :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')"
          />
          <button type="button" @click="openMyKeyPicker" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useMyKey') }}
          </button>
        </div>
        <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">
          {{ editing.api_key_masked }}
        </p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.primaryModel') }} <span class="text-red-500">*</span></label>
        <input
          v-model="form.primary_model"
          type="text"
          required
          class="input font-medium"
          :class="getPlatformTextClass(form.provider)"
          :placeholder="t('admin.channelMonitor.form.primaryModelPlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.extraModels') }}</label>
        <textarea
          v-model="extraModelsText"
          class="input min-h-[64px]"
          :placeholder="t('admin.channelMonitor.form.extraModelsPlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.groupName') }}</label>
        <input
          v-model="form.group_name"
          type="text"
          class="input"
          :placeholder="t('admin.channelMonitor.form.groupNamePlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="text-red-500">*</span></label>
        <input v-model.number="form.interval_seconds" type="number" min="15" max="3600" required class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') }}</p>
      </div>

      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.channelMonitor.form.enabled') }}</label>
        <Toggle v-model="form.enabled" />
      </div>

      <details class="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
        <summary class="cursor-pointer select-none text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channelMonitor.advanced.section') }}
        </summary>
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.advanced.sectionHint') }}</p>
        <div class="mt-4">
          <MonitorAdvancedRequestConfig
            :extra-headers="form.extra_headers"
            :body-override-mode="form.body_override_mode"
            :body-override="form.body_override"
            @update:extra-headers="form.extra_headers = $event"
            @update:body-override-mode="form.body_override_mode = $event"
            @update:body-override="form.body_override = $event"
          />
        </div>
      </details>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="channel-monitor-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          {{ submitting
            ? t('common.submitting')
            : editing ? t('common.update') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <MonitorKeyPickerDialog
    :show="showKeyPicker"
    :loading="myKeysLoading"
    :keys="myActiveKeys"
    :provider="form.provider"
    :user-group-rates="userGroupRates"
    @close="closeKeyPicker"
    @pick="pickMyKey"
  />
</template>

<script setup lang="ts">
/**
 * V5 W7 — Channel Monitor create/edit dialog (plugin version).
 *
 * Aligned with host styling (PlatformIcon grid picker, Toggle, platform
 * text coloring, "use current domain" button, font-mono API key, etc.).
 *
 * Host-only features not available to plugins:
 *   - ModelTagInput (host-only component; uses textarea instead)
 *   - template_id picker (requires host template API)
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, Toggle, PlatformIcon } from '@sub2api/plugin-sdk'
import {
  channelMonitorAPI,
  type BodyOverrideMode,
  type ChannelMonitor,
  type CreateParams,
  type Provider,
  type UpdateParams,
} from '../../../api/admin/channelMonitor'
import { DEFAULT_INTERVAL_SECONDS } from '../../../utils/channelMonitorConstants'
import { getSdk } from '../../../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { useChannelMonitorFormat } from '../../../composables/useChannelMonitorFormat'
import { useMonitorKeyPicker } from '../../../composables/useMonitorKeyPicker'
import { platformTextClass } from '@sub2api/plugin-sdk'
import MonitorAdvancedRequestConfig from './MonitorAdvancedRequestConfig.vue'
import MonitorKeyPickerDialog from './MonitorKeyPickerDialog.vue'

const props = defineProps<{
  show: boolean
  monitor: ChannelMonitor | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const sdk = getSdk()

const editing = computed(() => props.monitor)
const submitting = ref(false)
const { providerPickerClass } = useChannelMonitorFormat()
const getPlatformTextClass = platformTextClass

const {
  showKeyPicker, myKeysLoading, myActiveKeys, userGroupRates,
  openMyKeyPicker, pickMyKey, closeKeyPicker,
} = useMonitorKeyPicker((key) => { form.value.api_key = key })

function useCurrentDomain() {
  form.value.endpoint = window.location.origin
}

interface FormState {
  name: string
  provider: Provider
  endpoint: string
  api_key: string
  primary_model: string
  extra_models: string[]
  group_name: string
  enabled: boolean
  interval_seconds: number
  // Advanced (Worker B') — headers KV 行 + body override.
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

function emptyForm(): FormState {
  return {
    name: '',
    provider: 'anthropic',
    endpoint: '',
    api_key: '',
    primary_model: '',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: DEFAULT_INTERVAL_SECONDS,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
  }
}

const form = ref<FormState>(emptyForm())
const extraModelsText = ref('')

const providerOptions = computed<{ value: Provider; label: string }[]>(() => [
  { value: 'anthropic', label: t('monitorCommon.providers.anthropic') },
  { value: 'openai', label: t('monitorCommon.providers.openai') },
  { value: 'gemini', label: t('monitorCommon.providers.gemini') },
])

watch(
  () => [props.show, props.monitor] as const,
  ([show, monitor]) => {
    if (!show) return
    if (monitor) {
      form.value = {
        name: monitor.name,
        provider: monitor.provider,
        endpoint: monitor.endpoint,
        api_key: '',
        primary_model: monitor.primary_model,
        extra_models: monitor.extra_models || [],
        group_name: monitor.group_name || '',
        enabled: monitor.enabled,
        interval_seconds: monitor.interval_seconds,
        extra_headers: { ...(monitor.extra_headers || {}) },
        body_override_mode: monitor.body_override_mode || 'off',
        body_override: monitor.body_override ? { ...monitor.body_override } : null,
      }
      extraModelsText.value = (monitor.extra_models || []).join('\n')
    } else {
      form.value = emptyForm()
      extraModelsText.value = ''
    }
  },
  { immediate: true },
)

watch(extraModelsText, (txt) => {
  form.value.extra_models = txt
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
})

async function handleSubmit() {
  if (!form.value.name.trim()) {
    sdk.notify.error(t('admin.channelMonitor.nameRequired'))
    return
  }
  if (!form.value.primary_model.trim()) {
    sdk.notify.error(t('admin.channelMonitor.primaryModelRequired'))
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      const update: UpdateParams = {
        name: form.value.name.trim(),
        provider: form.value.provider,
        endpoint: form.value.endpoint.trim(),
        primary_model: form.value.primary_model.trim(),
        extra_models: form.value.extra_models,
        group_name: form.value.group_name.trim(),
        enabled: form.value.enabled,
        interval_seconds: form.value.interval_seconds,
        extra_headers: form.value.extra_headers,
        body_override_mode: form.value.body_override_mode,
        body_override: form.value.body_override,
      }
      // empty api_key means "do not modify" per backend contract
      if (form.value.api_key.trim()) {
        update.api_key = form.value.api_key.trim()
      }
      await channelMonitorAPI.update(editing.value.id, update)
      sdk.notify.success(t('admin.channelMonitor.updateSuccess'))
    } else {
      const create: CreateParams = {
        name: form.value.name.trim(),
        provider: form.value.provider,
        endpoint: form.value.endpoint.trim(),
        api_key: form.value.api_key.trim(),
        primary_model: form.value.primary_model.trim(),
        extra_models: form.value.extra_models,
        group_name: form.value.group_name.trim(),
        enabled: form.value.enabled,
        interval_seconds: form.value.interval_seconds,
        extra_headers: form.value.extra_headers,
        body_override_mode: form.value.body_override_mode,
        body_override: form.value.body_override,
      }
      await channelMonitorAPI.create(create)
      sdk.notify.success(t('admin.channelMonitor.createSuccess'))
    }
    emit('saved')
    emit('close')
  } catch (err: unknown) {
    sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}
</script>
