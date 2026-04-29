<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <MonitorFormBasicSection
        v-model:name="form.name"
        v-model:provider="form.provider"
        v-model:endpoint="form.endpoint"
        v-model:api-key="form.api_key"
        :editing="editing"
        @open-key-picker="openMyKeyPicker"
      />

      <MonitorFormModelSection
        v-model:primary-model="form.primary_model"
        v-model:extra-models="form.extra_models"
        v-model:group-name="form.group_name"
        :provider="form.provider"
      />

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="text-red-500">*</span></label>
        <input v-model.number="form.interval_seconds" type="number" min="15" max="3600" required class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') }}</p>
      </div>

      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.channelMonitor.form.enabled') }}</label>
        <Toggle v-model="form.enabled" />
      </div>

      <MonitorFormAdvancedSection
        v-model:template-id="form.template_id"
        v-model:extra-headers="form.extra_headers"
        v-model:body-override-mode="form.body_override_mode"
        v-model:body-override="form.body_override"
        :active="show"
        :provider="form.provider"
      />
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
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type {
  BodyOverrideMode,
  ChannelMonitor,
  CreateParams,
  Provider,
  UpdateParams,
} from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import MonitorKeyPickerDialog from '@/components/admin/monitor/MonitorKeyPickerDialog.vue'
import MonitorFormBasicSection from '@/components/admin/monitor/MonitorFormBasicSection.vue'
import MonitorFormModelSection from '@/components/admin/monitor/MonitorFormModelSection.vue'
import MonitorFormAdvancedSection from '@/components/admin/monitor/MonitorFormAdvancedSection.vue'
import { useMonitorKeyPicker } from '@/composables/useMonitorKeyPicker'
import {
  PROVIDER_ANTHROPIC,
  DEFAULT_INTERVAL_SECONDS,
} from '@/constants/channelMonitor'

const props = defineProps<{
  show: boolean
  monitor: ChannelMonitor | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

// System-configured default interval for new monitors. Falls back to the static
// constant when public settings haven't loaded yet or store the legacy 0 value.
const systemDefaultInterval = computed<number>(() => {
  const configured = appStore.cachedPublicSettings?.channel_monitor_default_interval_seconds
  return configured && configured > 0 ? configured : DEFAULT_INTERVAL_SECONDS
})

// editing is true when we have an existing monitor
const editing = computed<ChannelMonitor | null>(() => props.monitor)

const submitting = ref(false)

interface MonitorForm {
  name: string
  provider: Provider
  endpoint: string
  api_key: string
  primary_model: string
  extra_models: string[]
  group_name: string
  interval_seconds: number
  enabled: boolean
  // 高级设置快照
  template_id: number | null
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

const form = reactive<MonitorForm>({
  name: '',
  provider: PROVIDER_ANTHROPIC,
  endpoint: '',
  api_key: '',
  primary_model: '',
  extra_models: [],
  group_name: '',
  interval_seconds: systemDefaultInterval.value,
  enabled: true,
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
})

// Clear api_key whenever provider changes to avoid cross-provider key mismatch.
// Editing mode loads api_key='' via loadFromMonitor and only sets it on user
// typing, so clearing on provider change is always a safe no-op until the user
// picks a new key.
// 同时清空 template_id（模板有 provider 归属，跨平台不通用）。
watch(() => form.provider, () => {
  form.api_key = ''
  form.template_id = null
})

function resetForm() {
  form.name = ''
  form.provider = PROVIDER_ANTHROPIC
  form.endpoint = ''
  form.api_key = ''
  form.primary_model = ''
  form.extra_models = []
  form.group_name = ''
  form.interval_seconds = systemDefaultInterval.value
  form.enabled = true
  form.template_id = null
  form.extra_headers = {}
  form.body_override_mode = 'off'
  form.body_override = null
}

function loadFromMonitor(m: ChannelMonitor) {
  form.name = m.name
  form.provider = m.provider
  form.endpoint = m.endpoint
  form.api_key = ''
  form.primary_model = m.primary_model
  form.extra_models = [...(m.extra_models || [])]
  form.group_name = m.group_name || ''
  form.interval_seconds = m.interval_seconds || systemDefaultInterval.value
  form.enabled = m.enabled
  form.template_id = m.template_id ?? null
  form.extra_headers = { ...(m.extra_headers || {}) }
  form.body_override_mode = m.body_override_mode || 'off'
  form.body_override = m.body_override ? { ...m.body_override } : null
}

// Re-sync form whenever the dialog is opened or the target monitor changes.
// 模板列表的拉取已下放到 MonitorFormAdvancedSection（active=props.show 触发）。
watch(
  () => [props.show, props.monitor] as const,
  ([show, m]) => {
    if (!show) return
    if (m) loadFromMonitor(m)
    else resetForm()
  },
  { immediate: true },
)

// "选择我的 API key" picker — 状态机封装在 composable，回调里把选中的 key 写到 form。
const {
  showKeyPicker,
  myKeysLoading,
  myActiveKeys,
  userGroupRates,
  openMyKeyPicker,
  pickMyKey,
  closeKeyPicker,
} = useMonitorKeyPicker((key) => {
  form.api_key = key
})

function buildPayload(): CreateParams {
  return {
    name: form.name.trim(),
    provider: form.provider,
    endpoint: form.endpoint.trim(),
    api_key: form.api_key.trim(),
    primary_model: form.primary_model.trim(),
    extra_models: form.extra_models,
    group_name: form.group_name.trim(),
    enabled: form.enabled,
    interval_seconds: form.interval_seconds,
    template_id: form.template_id,
    extra_headers: form.extra_headers,
    body_override_mode: form.body_override_mode,
    body_override: form.body_override,
  }
}

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channelMonitor.nameRequired'))
    return
  }
  if (!form.primary_model.trim()) {
    appStore.showError(t('admin.channelMonitor.primaryModelRequired'))
    return
  }

  submitting.value = true
  try {
    const target = editing.value
    if (target) {
      const { api_key, ...rest } = buildPayload()
      const req: UpdateParams = { ...rest }
      // Only send api_key if user typed a new value
      if (api_key) req.api_key = api_key
      // template_id=null 用 clear_template=true 明确告诉后端清空（pointer 语义）
      if (form.template_id == null) {
        req.clear_template = true
        delete req.template_id
      }
      await adminAPI.channelMonitor.update(target.id, req)
      appStore.showSuccess(t('admin.channelMonitor.updateSuccess'))
    } else {
      await adminAPI.channelMonitor.create(buildPayload())
      appStore.showSuccess(t('admin.channelMonitor.createSuccess'))
    }
    emit('saved')
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}
</script>
