<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.template.managerTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <!-- provider tabs -->
    <div class="mb-4 border-b border-gray-200 dark:border-dark-700">
      <div role="tablist" class="flex flex-wrap gap-1">
        <button
          v-for="tab in providerTabs"
          :key="tab.value"
          type="button"
          role="tab"
          :aria-selected="activeProvider === tab.value"
          class="px-4 py-2 text-sm font-medium transition-colors"
          :class="tabClass(tab.value)"
          @click="activeProvider = tab.value"
        >
          {{ tab.label }}
          <span
            v-if="countByProvider[tab.value] > 0"
            class="ml-1.5 rounded-full bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700"
          >
            {{ countByProvider[tab.value] }}
          </span>
        </button>
      </div>
    </div>

    <!-- active provider list -->
    <div v-if="!editing" class="space-y-2">
      <div class="flex justify-end">
        <button class="btn btn-primary btn-sm" @click="openCreateForm">
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.channelMonitor.template.createButton') }}
        </button>
      </div>

      <div v-if="loading" class="py-8 text-center text-sm text-gray-400">
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="templatesForActiveProvider.length === 0"
        class="py-8 text-center text-sm text-gray-400"
      >
        {{ t('admin.channelMonitor.template.emptyState') }}
      </div>

      <div
        v-for="tpl in templatesForActiveProvider"
        v-else
        :key="tpl.id"
        class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ tpl.name }}</span>
              <span
                class="inline-flex items-center rounded-md px-1.5 py-0.5 text-xs"
                :class="modeBadgeClass(tpl.body_override_mode)"
              >
                {{ modeLabel(tpl.body_override_mode) }}
              </span>
              <span
                v-if="tpl.provider === PROVIDER_OPENAI"
                class="inline-flex items-center rounded-md px-1.5 py-0.5 text-xs"
                :class="apiModeBadgeClass(tpl.api_mode)"
              >
                {{ apiModeLabel(tpl.api_mode) }}
              </span>
              <span
                v-if="tpl.associated_monitors > 0"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.channelMonitor.template.associatedCount', { n: tpl.associated_monitors }) }}
              </span>
            </div>
            <p v-if="tpl.description" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ tpl.description }}
            </p>
            <p class="mt-1 text-xs text-gray-400">
              {{ t('admin.channelMonitor.template.headersSummary', {
                n: Object.keys(tpl.extra_headers || {}).length,
              }) }}
            </p>
          </div>
          <div class="flex flex-shrink-0 gap-2">
            <button
              class="btn btn-secondary btn-sm"
              :disabled="tpl.associated_monitors === 0"
              :title="t('admin.channelMonitor.template.applyTooltip')"
              @click="confirmApply(tpl)"
            >
              <Icon name="refresh" size="sm" class="mr-1" />
              {{ t('admin.channelMonitor.template.applyButton') }}
            </button>
            <button class="btn btn-secondary btn-sm" @click="openEditForm(tpl)">
              {{ t('common.edit') }}
            </button>
            <button class="btn btn-secondary btn-sm text-red-600" @click="handleDelete(tpl)">
              {{ t('common.delete') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- edit / create form -->
    <div v-else class="space-y-4">
      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.template.form.name') }}
          <span class="text-red-500">*</span>
        </label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.channelMonitor.template.form.namePlaceholder')"
        />
      </div>

      <div v-if="editing === 'new'">
        <label class="input-label">
          {{ t('admin.channelMonitor.form.provider') }}
          <span class="text-red-500">*</span>
        </label>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <button
            v-for="opt in providerTabs"
            :key="opt.value"
            type="button"
            class="rounded-lg border-2 px-3 py-2 text-sm font-medium transition-colors"
            :class="providerPickerClass(opt.value, form.provider === opt.value)"
            @click="form.provider = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <div v-if="form.provider === PROVIDER_OPENAI" class="rounded-lg border border-blue-100 bg-blue-50/50 p-3 dark:border-blue-500/20 dark:bg-blue-500/10">
        <label class="input-label">{{ t('admin.channelMonitor.form.apiMode') }}</label>
        <div class="grid gap-3 sm:grid-cols-2">
          <button
            v-for="opt in apiModeOptions"
            :key="opt.value"
            type="button"
            class="rounded-lg border-2 px-3 py-2 text-left transition-colors"
            :class="apiModeButtonClass(opt.value)"
            @click="form.api_mode = opt.value"
          >
            <span class="block text-sm font-semibold">{{ opt.label }}</span>
            <span class="mt-0.5 block text-xs opacity-80">{{ opt.hint }}</span>
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.template.form.description') }}
        </label>
        <input
          v-model="form.description"
          type="text"
          class="input"
          :placeholder="t('admin.channelMonitor.template.form.descriptionPlaceholder')"
        />
      </div>

      <MonitorAdvancedRequestConfig
        :provider="form.provider"
        :api-mode="form.api_mode"
        :extra-headers="form.extra_headers"
        :body-override-mode="form.body_override_mode"
        :body-override="form.body_override"
        @update:extra-headers="form.extra_headers = $event"
        @update:body-override-mode="form.body_override_mode = $event"
        @update:body-override="form.body_override = $event"
      />
    </div>

    <template #footer>
      <div class="flex w-full items-center justify-between">
        <!-- Left: back to list / nothing -->
        <div>
          <button v-if="editing" class="btn btn-secondary" @click="backToList">
            {{ t('common.back') }}
          </button>
        </div>
        <!-- Right: save or close -->
        <div class="flex gap-2">
          <button class="btn btn-secondary" @click="$emit('close')">
            {{ t('common.close') }}
          </button>
          <button v-if="editing" class="btn btn-primary" :disabled="submitting" @click="handleSubmit">
            {{ submitting ? t('common.submitting') : editing === 'new' ? t('common.create') : t('common.update') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>

  <MonitorTemplateApplyPickerDialog
    :show="applyPicker.show"
    :template-id="applyPicker.tpl ? applyPicker.tpl.id : null"
    :template-name="applyPicker.tpl ? applyPicker.tpl.name : ''"
    @close="applyPicker.show = false"
    @applied="onApplied"
  />

  <ConfirmDialog
    :show="confirmDelete.show"
    :title="t('common.delete')"
    :message="confirmDeleteMessage"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="doDelete"
    @cancel="confirmDelete.show = false"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type {
  APIMode,
  BodyOverrideMode,
  Provider,
} from '@/api/admin/channelMonitor'
import type { ChannelMonitorTemplate } from '@/api/admin/channelMonitorTemplate'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'
import MonitorTemplateApplyPickerDialog from '@/components/admin/monitor/MonitorTemplateApplyPickerDialog.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import {
  PROVIDER_ANTHROPIC,
  PROVIDER_OPENAI,
  PROVIDER_GEMINI,
  PROVIDER_GROK,
  API_MODE_CHAT_COMPLETIONS,
  API_MODE_RESPONSES,
} from '@/constants/channelMonitor'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  /** Fired when any template changed (create / update / delete / apply). */
  (e: 'updated'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { providerPickerClass } = useChannelMonitorFormat()

const providerTabs = computed<{ value: Provider; label: string }[]>(() => [
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
  { value: PROVIDER_GROK, label: t('monitorCommon.providers.grok') },
])

const activeProvider = ref<Provider>(PROVIDER_ANTHROPIC)
const templates = ref<ChannelMonitorTemplate[]>([])
const loading = ref(false)

const templatesForActiveProvider = computed(() =>
  templates.value.filter((t) => t.provider === activeProvider.value),
)

const countByProvider = computed<Record<Provider, number>>(() => {
  const out: Record<Provider, number> = {
    anthropic: 0,
    openai: 0,
    gemini: 0,
    grok: 0,
  }
  for (const t of templates.value) out[t.provider]++
  return out
})

// --- form state ---
interface TemplateForm {
  id: number | null
  name: string
  provider: Provider
  api_mode: APIMode
  description: string
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

const editing = ref<null | 'new' | number>(null) // null = list view; 'new' = create; <id> = edit
const submitting = ref(false)
const form = reactive<TemplateForm>(emptyForm(PROVIDER_ANTHROPIC))

function emptyForm(provider: Provider): TemplateForm {
  return {
    id: null,
    name: '',
    provider,
    api_mode: API_MODE_CHAT_COMPLETIONS,
    description: '',
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
  }
}

function loadForm(tpl: ChannelMonitorTemplate) {
  form.id = tpl.id
  form.name = tpl.name
  form.provider = tpl.provider
  form.api_mode = normalizeAPIMode(tpl.api_mode)
  form.description = tpl.description
  form.extra_headers = { ...(tpl.extra_headers || {}) }
  form.body_override_mode = tpl.body_override_mode
  form.body_override = tpl.body_override ? { ...tpl.body_override } : null
}

function openCreateForm() {
  Object.assign(form, emptyForm(activeProvider.value))
  editing.value = 'new'
}

function openEditForm(tpl: ChannelMonitorTemplate) {
  loadForm(tpl)
  editing.value = tpl.id
}

function backToList() {
  editing.value = null
}

// --- data fetch ---
async function fetchTemplates() {
  loading.value = true
  try {
    const { items } = await adminAPI.channelMonitorTemplate.list()
    templates.value = items
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      editing.value = null
      fetchTemplates()
    }
  },
  { immediate: true },
)

// --- submit ---
async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channelMonitor.template.missingName'))
    return
  }
  submitting.value = true
  try {
    if (editing.value === 'new') {
      await adminAPI.channelMonitorTemplate.create({
        name: form.name.trim(),
        provider: form.provider,
        api_mode: form.provider === PROVIDER_OPENAI ? form.api_mode : API_MODE_CHAT_COMPLETIONS,
        description: form.description.trim(),
        extra_headers: form.extra_headers,
        body_override_mode: form.body_override_mode,
        body_override: form.body_override,
      })
      appStore.showSuccess(t('admin.channelMonitor.template.createSuccess'))
    } else if (typeof editing.value === 'number') {
      await adminAPI.channelMonitorTemplate.update(editing.value, {
        name: form.name.trim(),
        api_mode: form.provider === PROVIDER_OPENAI ? form.api_mode : API_MODE_CHAT_COMPLETIONS,
        description: form.description.trim(),
        extra_headers: form.extra_headers,
        body_override_mode: form.body_override_mode,
        body_override: form.body_override,
      })
      appStore.showSuccess(t('admin.channelMonitor.template.updateSuccess'))
    }
    await fetchTemplates()
    emit('updated')
    editing.value = null
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}

// --- apply to monitors (picker 流程) ---
const applyPicker = reactive<{ show: boolean; tpl: ChannelMonitorTemplate | null }>({
  show: false,
  tpl: null,
})

function confirmApply(tpl: ChannelMonitorTemplate) {
  applyPicker.tpl = tpl
  applyPicker.show = true
}

// picker 提交后触发：刷新模板列表（拿最新 associated_monitors）+ 通知父组件
async function onApplied(_affected: number) {
  await fetchTemplates()
  emit('updated')
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
    await adminAPI.channelMonitorTemplate.del(tpl.id)
    appStore.showSuccess(t('admin.channelMonitor.template.deleteSuccess'))
    await fetchTemplates()
    emit('updated')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

// --- misc ---
function tabClass(value: Provider): string {
  return activeProvider.value === value
    ? 'border-b-2 border-primary-500 text-primary-600 dark:text-primary-400'
    : 'border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
}

function modeBadgeClass(mode: BodyOverrideMode): string {
  switch (mode) {
    case 'merge':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'replace':
      return 'bg-purple-100 text-purple-700 dark:bg-purple-500/15 dark:text-purple-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

function modeLabel(mode: BodyOverrideMode): string {
  return t(`admin.channelMonitor.advanced.bodyMode${mode.charAt(0).toUpperCase()}${mode.slice(1)}`)
}

const apiModeOptions = computed<{ value: APIMode; label: string; hint: string }[]>(() => [
  {
    value: API_MODE_CHAT_COMPLETIONS,
    label: t('admin.channelMonitor.form.apiModeChatCompletions'),
    hint: t('admin.channelMonitor.form.apiModeChatCompletionsHint'),
  },
  {
    value: API_MODE_RESPONSES,
    label: t('admin.channelMonitor.form.apiModeResponses'),
    hint: t('admin.channelMonitor.form.apiModeResponsesHint'),
  },
])

watch(() => form.provider, (provider) => {
  if (provider !== PROVIDER_OPENAI) {
    form.api_mode = API_MODE_CHAT_COMPLETIONS
  }
})

function normalizeAPIMode(mode: APIMode | undefined | null): APIMode {
  return mode === API_MODE_RESPONSES ? API_MODE_RESPONSES : API_MODE_CHAT_COMPLETIONS
}

function apiModeButtonClass(mode: APIMode): string {
  const active = form.api_mode === mode
  if (active) {
    return 'border-primary-500 bg-white text-primary-700 shadow-sm dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300'
  }
  return 'border-blue-100 bg-white/70 text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
}

function apiModeLabel(mode: APIMode): string {
  return normalizeAPIMode(mode) === API_MODE_RESPONSES
    ? t('admin.channelMonitor.form.apiModeResponses')
    : t('admin.channelMonitor.form.apiModeChatCompletions')
}

function apiModeBadgeClass(mode: APIMode): string {
  if (normalizeAPIMode(mode) === API_MODE_RESPONSES) {
    return 'bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
  }
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
}
</script>
