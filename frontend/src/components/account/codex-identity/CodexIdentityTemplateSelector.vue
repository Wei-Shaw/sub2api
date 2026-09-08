<template>
  <section class="space-y-4" :aria-labelledby="`${idPrefix}-title`" data-testid="codex-identity-template-selector">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <h3 :id="`${idPrefix}-title`" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.codexIdentity.assignmentTitle') }}
        </h3>
        <p :id="`${idPrefix}-description`" class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
          {{ t('admin.accounts.codexIdentity.assignmentDescription') }}
        </p>
        <a
          href="/admin/settings?tab=codexProfiles"
          class="mt-1 inline-flex text-xs font-medium text-primary-600 hover:underline dark:text-primary-400"
        >
          {{ t('admin.accounts.codexIdentity.manageTemplates') }}
        </a>
      </div>
      <button
        :id="`${idPrefix}-enabled`"
        type="button"
        role="switch"
        :aria-checked="modelValue.enabled"
        :aria-labelledby="`${idPrefix}-title`"
        :aria-describedby="`${idPrefix}-description`"
        :disabled="disabled"
        class="relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
        :class="modelValue.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
        data-testid="codex-template-assignment-toggle"
        @click="toggleEnabled"
      >
        <span
          aria-hidden="true"
          class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform"
          :class="modelValue.enabled ? 'translate-x-5' : 'translate-x-0'"
        />
      </button>
    </div>

    <div v-if="modelValue.enabled" class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div v-if="loading" class="flex items-center gap-2 text-sm text-gray-500" role="status">
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('admin.accounts.codexIdentity.loadingTemplates') }}
      </div>
      <div v-else-if="loadError" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 dark:border-red-900 dark:bg-red-900/15" role="alert">
        <p class="text-xs text-red-700 dark:text-red-300">{{ loadError }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-2" @click="loadTemplates">
          {{ t('admin.accounts.codexIdentity.retryTemplates') }}
        </button>
      </div>
      <div v-else-if="templates.length === 0" class="border-y border-gray-100 py-3 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400" role="status">
        {{ t('admin.accounts.codexIdentity.noTemplates') }}
      </div>
      <template v-else>
        <div>
          <label :for="`${idPrefix}-template`" class="input-label">
            {{ t('admin.accounts.codexIdentity.templateLabel') }}
          </label>
          <Select
            :id="`${idPrefix}-template`"
            :model-value="modelValue.template_id ?? null"
            :options="templateOptions"
            :disabled="disabled"
            :placeholder="t('admin.accounts.codexIdentity.templatePlaceholder')"
            :aria-label="t('admin.accounts.codexIdentity.templateLabel')"
            searchable="auto"
            data-testid="codex-template-assignment-select"
            @update:model-value="selectTemplate"
          />
          <p v-if="modelValue.enabled && !modelValue.template_id" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
            {{ t('admin.accounts.codexIdentity.templateRequired') }}
          </p>
        </div>

        <div v-if="selectedTemplate" class="border-y border-gray-100 py-3 dark:border-dark-700">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-gray-800 dark:text-dark-100">{{ selectedTemplate.name }}</span>
            <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">
              {{ t('admin.accounts.codexIdentity.templateRevision', { revision: selectedTemplate.revision }) }}
            </span>
          </div>
          <p v-if="selectedTemplate.description" class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
            {{ selectedTemplate.description }}
          </p>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <span
              v-for="profile in selectedTemplate.profiles"
              :key="`${profile.os_class}:${profile.canonical_surface}`"
              class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 dark:border-dark-600 dark:text-dark-300"
            >
              {{ profileLabel(profile) }} · {{ profile.slot_count }}
            </span>
          </div>
        </div>
      </template>
    </div>

    <p v-else class="border-y border-gray-100 py-3 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400" role="status">
      {{ t('admin.accounts.codexIdentity.assignmentOff') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CodexIdentityAssignment,
  CodexIdentityTemplate,
  CodexOSProfilePolicy,
} from '@/types/codexIdentity'
import { extractI18nErrorMessage } from '@/utils/apiError'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  modelValue: CodexIdentityAssignment
  disabled?: boolean
  idPrefix?: string
}>(), {
  disabled: false,
  idPrefix: 'codex-template-assignment',
})
const emit = defineEmits<{
  'update:modelValue': [value: CodexIdentityAssignment]
}>()

const { t } = useI18n()
const templates = ref<CodexIdentityTemplate[]>([])
const loading = ref(false)
const loadError = ref('')
const lastTemplateID = ref<number | undefined>(props.modelValue.template_id)
const lastTemplateRevision = ref<number | undefined>(props.modelValue.expected_revision)

watch(() => props.modelValue, (value) => {
  if (value.template_id && value.template_id > 0) {
    lastTemplateID.value = value.template_id
    lastTemplateRevision.value = value.expected_revision
  }
}, { deep: true })

const loadTemplates = async () => {
  loading.value = true
  loadError.value = ''
  try {
    templates.value = await adminAPI.codexIdentityTemplates.list()
  } catch (error) {
    loadError.value = extractI18nErrorMessage(
      error,
      t,
      'admin.accounts.codexIdentity.templateErrors',
      t('admin.accounts.codexIdentity.loadTemplatesFailed'),
    )
  } finally {
    loading.value = false
  }
}

const templateOptions = computed(() => templates.value.map((item) => ({
  value: item.id,
  label: item.name,
})))
const selectedTemplate = computed(() =>
  templates.value.find((item) => item.id === props.modelValue.template_id) ?? null,
)

const toggleEnabled = () => {
  if (props.modelValue.enabled) {
    emit('update:modelValue', { enabled: false })
    return
  }
  emit('update:modelValue', {
    enabled: true,
    ...(lastTemplateID.value ? { template_id: lastTemplateID.value } : {}),
    ...(lastTemplateRevision.value ? { expected_revision: lastTemplateRevision.value } : {}),
  })
}
const selectTemplate = (value: string | number | boolean | null) => {
  const templateID = typeof value === 'number' ? value : Number(value)
  const template = templates.value.find((item) => item.id === templateID)
  emit('update:modelValue', Number.isInteger(templateID) && templateID > 0 && template
    ? { enabled: true, template_id: templateID, expected_revision: template.revision }
    : { enabled: true })
}
const profileLabel = (profile: CodexOSProfilePolicy): string => {
  const osKey = profile.os_class === 'generic' ? 'genericOS' : profile.os_class
  const surfaceKey = profile.canonical_surface === 'third_party' ? 'thirdParty' : profile.canonical_surface
  return `${t(`admin.accounts.codexIdentity.${osKey}`)} / ${t(`admin.accounts.codexIdentity.${surfaceKey}`)}`
}

onMounted(loadTemplates)
</script>
