/**
 * Composable: data + CRUD logic for MonitorTemplateManagerDialog.
 *
 * Extracted to keep MonitorTemplateManagerDialog.vue under 300 lines.
 */
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BodyOverrideMode, Provider } from '../api/admin/channelMonitor'
import {
  channelMonitorTemplateAPI,
  type ChannelMonitorTemplate,
} from '../api/admin/channelMonitorTemplate'
import {
  PROVIDER_ANTHROPIC,
  PROVIDER_OPENAI,
  PROVIDER_GEMINI,
} from '../utils/channelMonitorConstants'
import { getSdk } from '../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'

interface TemplateForm {
  id: number | null
  name: string
  provider: Provider
  description: string
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

function emptyForm(provider: Provider): TemplateForm {
  return {
    id: null,
    name: '',
    provider,
    description: '',
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
  }
}

export function useTemplateManager(
  props: { show: boolean },
  emitFn: { updated: () => void },
) {
  const { t } = useI18n()
  const sdk = getSdk()

  const activeProvider = ref<Provider>(PROVIDER_ANTHROPIC)
  const templates = ref<ChannelMonitorTemplate[]>([])
  const loading = ref(false)
  const editing = ref<null | 'new' | number>(null)
  const submitting = ref(false)
  const form = reactive<TemplateForm>(emptyForm(PROVIDER_ANTHROPIC))

  const providerTabs = computed<{ value: Provider; label: string }[]>(() => [
    { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
    { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
    { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
  ])

  const templatesForActiveProvider = computed(() =>
    templates.value.filter(tpl => tpl.provider === activeProvider.value),
  )

  const countByProvider = computed<Record<Provider, number>>(() => {
    const out: Record<Provider, number> = { anthropic: 0, openai: 0, gemini: 0 }
    for (const tpl of templates.value) out[tpl.provider]++
    return out
  })

  function openCreateForm() {
    Object.assign(form, emptyForm(activeProvider.value))
    editing.value = 'new'
  }

  function openEditForm(tpl: ChannelMonitorTemplate) {
    form.id = tpl.id
    form.name = tpl.name
    form.provider = tpl.provider
    form.description = tpl.description
    form.extra_headers = { ...(tpl.extra_headers || {}) }
    form.body_override_mode = tpl.body_override_mode
    form.body_override = tpl.body_override ? { ...tpl.body_override } : null
    editing.value = tpl.id
  }

  function backToList() { editing.value = null }

  async function fetchTemplates() {
    loading.value = true
    try {
      const { items } = await channelMonitorTemplateAPI.list()
      templates.value = items
    } catch (err: unknown) {
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
    } finally {
      loading.value = false
    }
  }

  watch(() => props.show, (show) => {
    if (show) { editing.value = null; fetchTemplates() }
  }, { immediate: true })

  async function handleSubmit() {
    if (submitting.value) return
    if (!form.name.trim()) {
      sdk.notify.error(t('admin.channelMonitor.template.missingName'))
      return
    }
    submitting.value = true
    try {
      if (editing.value === 'new') {
        await channelMonitorTemplateAPI.create({
          name: form.name.trim(),
          provider: form.provider,
          description: form.description.trim(),
          extra_headers: form.extra_headers,
          body_override_mode: form.body_override_mode,
          body_override: form.body_override,
        })
        sdk.notify.success(t('admin.channelMonitor.template.createSuccess'))
      } else if (typeof editing.value === 'number') {
        await channelMonitorTemplateAPI.update(editing.value, {
          name: form.name.trim(),
          description: form.description.trim(),
          extra_headers: form.extra_headers,
          body_override_mode: form.body_override_mode,
          body_override: form.body_override,
        })
        sdk.notify.success(t('admin.channelMonitor.template.updateSuccess'))
      }
      await fetchTemplates()
      emitFn.updated()
      editing.value = null
    } catch (err: unknown) {
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
    } finally {
      submitting.value = false
    }
  }

  // --- apply ---
  const applyPicker = reactive<{ show: boolean; tpl: ChannelMonitorTemplate | null }>({
    show: false,
    tpl: null,
  })

  function confirmApply(tpl: ChannelMonitorTemplate) {
    applyPicker.tpl = tpl
    applyPicker.show = true
  }

  async function onApplied(_affected: number) {
    await fetchTemplates()
    emitFn.updated()
  }

  // --- delete ---
  const confirmDelete = reactive<{ show: boolean; tpl: ChannelMonitorTemplate | null }>({
    show: false,
    tpl: null,
  })

  function handleDelete(tpl: ChannelMonitorTemplate) {
    confirmDelete.tpl = tpl
    confirmDelete.show = true
  }

  const confirmDeleteMessage = computed(() => {
    const tpl = confirmDelete.tpl
    if (!tpl) return ''
    return t('admin.channelMonitor.template.deleteConfirm', {
      name: tpl.name,
      n: tpl.associated_monitors,
    })
  })

  async function doDelete() {
    const tpl = confirmDelete.tpl
    confirmDelete.show = false
    if (!tpl) return
    try {
      await channelMonitorTemplateAPI.del(tpl.id)
      sdk.notify.success(t('admin.channelMonitor.template.deleteSuccess'))
      await fetchTemplates()
      emitFn.updated()
    } catch (err: unknown) {
      sdk.notify.error(extractApiErrorMessage(err, t('common.error')))
    }
  }

  function tabClass(value: Provider): string {
    return activeProvider.value === value
      ? 'border-b-2 border-primary-500 text-primary-600 dark:text-primary-400'
      : 'border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
  }

  return {
    activeProvider,
    providerTabs,
    templatesForActiveProvider,
    countByProvider,
    loading,
    editing,
    submitting,
    form,
    applyPicker,
    confirmDelete,
    confirmDeleteMessage,
    openCreateForm,
    openEditForm,
    backToList,
    handleSubmit,
    confirmApply,
    onApplied,
    handleDelete,
    doDelete,
    tabClass,
  }
}
