<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.name') }} <span class="input-required">*</span>
        </label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.channelMonitor.form.namePlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.provider') }} <span class="input-required">*</span>
        </label>
        <Select v-model="form.provider" :options="providerOptions" class="w-full" />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.endpoint') }} <span class="input-required">*</span>
        </label>
        <input
          v-model="form.endpoint"
          type="text"
          required
          class="input"
          :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.apiKey')
          }}<span v-if="!editing" class="input-required"> *</span>
        </label>
        <input
          v-model="form.api_key"
          type="password"
          :required="!editing"
          class="input"
          :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')"
        />
        <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">
          {{ editing.api_key_masked }}
        </p>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.primaryModel') }} <span class="input-required">*</span>
        </label>
        <input
          v-model="form.primary_model"
          type="text"
          required
          class="input"
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
        <label class="input-label">
          {{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="input-required">*</span>
        </label>
        <input
          v-model.number="form.interval_seconds"
          type="number"
          min="15"
          max="3600"
          required
          class="input"
        />
        <p class="mt-1 text-xs text-gray-500">
          {{ t('admin.channelMonitor.form.intervalSecondsHint') }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <input
          id="monitor-enabled-toggle"
          v-model="form.enabled"
          type="checkbox"
          class="h-4 w-4"
        />
        <label for="monitor-enabled-toggle" class="text-sm">
          {{ t('admin.channelMonitor.form.enabled') }}
        </label>
      </div>
    </form>

    <template #footer>
      <button type="button" @click="$emit('close')" class="btn btn-secondary">
        {{ t('common.cancel') }}
      </button>
      <button
        type="submit"
        form="channel-monitor-form"
        class="btn btn-primary"
        :disabled="submitting"
      >
        {{ submitting ? t('common.saving') : t('common.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * V5 W7 — Simplified Channel Monitor create/edit dialog.
 *
 * 简化范围 (vs 09fd83ab `frontend/src/components/admin/monitor/MonitorFormDialog.vue`):
 *   - 删除 ProviderIcon picker, 改用 SDK Select
 *   - 删除 "Use my key" picker (host APIKey 列表查询; plugin 暂未提供等价 API)
 *   - 删除 ModelTagInput, 改为多行 textarea (空白分隔)
 *   - 删除 advanced (extra_headers / body_override / template_id) UI;
 *     仅在 update 时透传现有值, 避免回传时把这些字段清空
 *   - 删除 useCurrentDomain 按钮 (依赖 host window.location，简化版直接让管
 *     理员手填)
 *
 * 这些被删除的功能仍然由后端支持, 后续 W7.x 可再补 UI.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, Select } from '@sub2api/plugin-sdk'
import {
  channelMonitorAPI,
  type ChannelMonitor,
  type CreateParams,
  type Provider,
  type UpdateParams,
} from '../../../api/admin/channelMonitor'
import { DEFAULT_INTERVAL_SECONDS } from '../../../utils/channelMonitorConstants'
import { getSdk } from '../../../api/sdk'
import { extractApiErrorMessage } from '../../../utils/apiError'

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
}

function emptyForm(): FormState {
  return {
    name: '',
    provider: 'openai',
    endpoint: '',
    api_key: '',
    primary_model: '',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: DEFAULT_INTERVAL_SECONDS,
  }
}

const form = ref<FormState>(emptyForm())
const extraModelsText = ref('')

const providerOptions = computed(() => [
  { value: 'openai', label: t('monitorCommon.providers.openai') },
  { value: 'anthropic', label: t('monitorCommon.providers.anthropic') },
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
