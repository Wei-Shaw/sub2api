/**
 * Composable: form state + submit logic for MonitorFormDialog.
 *
 * Extracted from MonitorFormDialog.vue to keep the dialog under 300 lines.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  channelMonitorAPI,
  type BodyOverrideMode,
  type ChannelMonitor,
  type CreateParams,
  type Provider,
  type UpdateParams,
} from '../api/admin/channelMonitor'
import { DEFAULT_INTERVAL_SECONDS } from '../utils/channelMonitorConstants'
import { getSdk } from '../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'

export interface MonitorFormState {
  name: string
  provider: Provider
  endpoint: string
  api_key: string
  primary_model: string
  extra_models: string[]
  group_name: string
  enabled: boolean
  interval_seconds: number
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

function emptyForm(): MonitorFormState {
  return {
    name: '', provider: 'anthropic', endpoint: '', api_key: '',
    primary_model: '', extra_models: [], group_name: '', enabled: true,
    interval_seconds: DEFAULT_INTERVAL_SECONDS, extra_headers: {},
    body_override_mode: 'off', body_override: null,
  }
}

export function useMonitorFormLogic(props: {
  show: boolean
  monitor: ChannelMonitor | null
}, emitFn: {
  saved: () => void
  close: () => void
}) {
  const { t } = useI18n()
  const sdk = getSdk()

  const form = ref<MonitorFormState>(emptyForm())
  const extraModelsText = ref('')
  const submitting = ref(false)
  const editing = computed(() => props.monitor)

  watch(
    () => [props.show, props.monitor] as const,
    ([show, monitor]) => {
      if (!show) return
      if (monitor) {
        form.value = {
          name: monitor.name, provider: monitor.provider,
          endpoint: monitor.endpoint, api_key: '',
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

  function useCurrentDomain() {
    form.value.endpoint = window.location.origin
  }

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
          name: form.value.name.trim(), provider: form.value.provider,
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
        if (form.value.api_key.trim()) update.api_key = form.value.api_key.trim()
        await channelMonitorAPI.update(editing.value.id, update)
        sdk.notify.success(t('admin.channelMonitor.updateSuccess'))
      } else {
        const create: CreateParams = {
          name: form.value.name.trim(), provider: form.value.provider,
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
      emitFn.saved()
      emitFn.close()
    } catch (err: unknown) {
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
    } finally {
      submitting.value = false
    }
  }

  return { form, extraModelsText, submitting, editing, useCurrentDomain, handleSubmit }
}
