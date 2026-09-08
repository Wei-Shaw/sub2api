<template>
  <section class="card" data-testid="codex-identity-templates-settings">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:px-6 dark:border-dark-700">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.codexProfiles.title') }}
        </h2>
        <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.codexProfiles.description') }}
        </p>
      </div>
      <button type="button" class="btn btn-primary shrink-0" data-testid="create-codex-template" @click="openCreate()">
        <Icon name="plus" size="sm" class="mr-1" />
        {{ t('admin.settings.codexProfiles.create') }}
      </button>
    </div>

    <div class="p-4 sm:p-6">
      <div v-if="loading" class="flex items-center justify-center gap-2 py-12 text-sm text-gray-500" role="status">
        <Icon name="refresh" size="md" class="animate-spin" />
        {{ t('admin.settings.codexProfiles.loading') }}
      </div>

      <div v-else-if="loadError" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-900 dark:bg-red-900/15" role="alert">
        <p class="text-sm text-red-700 dark:text-red-300">{{ loadError }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadTemplates">
          {{ t('admin.settings.codexProfiles.retry') }}
        </button>
      </div>

      <div v-else-if="templates.length === 0" class="border-y border-gray-100 py-10 text-center dark:border-dark-700">
        <Icon name="terminal" size="xl" class="mx-auto text-gray-400" />
        <h3 class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.codexProfiles.emptyTitle') }}
        </h3>
        <p class="mx-auto mt-1 max-w-xl text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.codexProfiles.emptyDescription') }}
        </p>
      </div>

      <div v-else class="divide-y divide-gray-100 border-y border-gray-100 dark:divide-dark-700 dark:border-dark-700">
        <article
          v-for="item in templates"
          :key="item.id"
          class="grid grid-cols-1 gap-4 py-4 lg:grid-cols-[minmax(0,1fr)_12rem_11rem_auto] lg:items-center"
          :data-testid="`codex-template-${item.id}`"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="break-words text-sm font-semibold text-gray-900 dark:text-white">{{ item.name }}</h3>
              <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">
                {{ t('admin.settings.codexProfiles.revision', { revision: item.revision }) }}
              </span>
            </div>
            <p v-if="item.description" class="mt-1 break-words text-sm text-gray-500 dark:text-gray-400">
              {{ item.description }}
            </p>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <span
                v-for="profile in item.profiles"
                :key="`${profile.os_class}:${profile.canonical_surface}`"
                class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 dark:border-dark-600 dark:text-dark-300"
              >
                {{ profileLabel(profile) }} · {{ profile.slot_count }}
              </span>
            </div>
          </div>

          <div class="text-sm text-gray-600 dark:text-dark-300">
            <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.settings.codexProfiles.accounts') }}</span>
            <span class="mt-1 block font-medium tabular-nums">
              {{ t('admin.settings.codexProfiles.accountCount', { count: item.assigned_account_count }) }}
            </span>
          </div>

          <div class="text-sm text-gray-600 dark:text-dark-300">
            <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.settings.codexProfiles.updatedAt') }}</span>
            <time class="mt-1 block tabular-nums" :datetime="item.updated_at">{{ formatUpdatedAt(item.updated_at) }}</time>
          </div>

          <div class="flex items-center gap-1 lg:justify-end">
            <button
              type="button"
              class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('admin.settings.codexProfiles.edit')"
              :aria-label="`${t('admin.settings.codexProfiles.edit')}: ${item.name}`"
              :disabled="openingEditorID === item.id"
              @click="openEdit(item)"
            >
              <Icon :name="openingEditorID === item.id ? 'refresh' : 'edit'" size="sm" :class="openingEditorID === item.id && 'animate-spin'" />
            </button>
            <button
              type="button"
              class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('admin.settings.codexProfiles.copy')"
              :aria-label="`${t('admin.settings.codexProfiles.copy')}: ${item.name}`"
              @click="openCreate(item)"
            >
              <Icon name="copy" size="sm" />
            </button>
            <button
              type="button"
              class="rounded p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('admin.settings.codexProfiles.delete')"
              :aria-label="`${t('admin.settings.codexProfiles.delete')}: ${item.name}`"
              @click="requestDelete(item)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </article>
      </div>
    </div>

    <BaseDialog
      :show="editorOpen"
      :title="editingTemplate ? t('admin.settings.codexProfiles.editTitle') : t('admin.settings.codexProfiles.createTitle')"
      width="full"
      @close="closeEditor"
    >
      <div class="space-y-6" data-testid="codex-template-editor">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label for="codex-template-name" class="input-label">{{ t('admin.settings.codexProfiles.form.name') }}</label>
            <input
              id="codex-template-name"
              v-model="editor.name"
              type="text"
              maxlength="100"
              class="input"
              :placeholder="t('admin.settings.codexProfiles.form.namePlaceholder')"
              data-testid="codex-template-name"
            />
          </div>
          <div>
            <label for="codex-template-description" class="input-label">{{ t('admin.settings.codexProfiles.form.description') }}</label>
            <input
              id="codex-template-description"
              v-model="editor.description"
              type="text"
              maxlength="500"
              class="input"
              :placeholder="t('admin.settings.codexProfiles.form.descriptionPlaceholder')"
            />
          </div>
        </div>

        <div class="border-t border-gray-200 pt-5 dark:border-dark-600">
          <CodexIdentityPolicyEditor
            v-model="editor.policy"
            :proxies="proxies"
            :show-mode-toggle="false"
            template-context
            :title="t('admin.settings.codexProfiles.form.profilesTitle')"
            :description="t('admin.settings.codexProfiles.form.profilesDescription')"
            id-prefix="codex-template-policy"
          />
        </div>
      </div>

      <template #footer>
        <div class="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeEditor">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="saving" data-testid="save-codex-template" @click="saveTemplate">
            <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ saving ? t('admin.settings.codexProfiles.saving') : t('admin.settings.codexProfiles.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showUpdateConfirmation"
      :title="t('admin.settings.codexProfiles.updateImpactTitle')"
      :message="updateImpactMessage"
      :confirm-text="t('admin.settings.codexProfiles.confirmUpdate')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmAssignedTemplateUpdate"
      @cancel="showUpdateConfirmation = false"
    />

    <ConfirmDialog
      :show="Boolean(deletingTemplate)"
      :title="t('admin.settings.codexProfiles.deleteTitle')"
      :message="deleteMessage"
      :confirm-text="t('admin.settings.codexProfiles.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="deletingTemplate = null"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type {
  CodexIdentityPolicy,
  CodexIdentityProxyOption,
  CodexIdentityTemplate,
  CodexIdentityTemplateWriteRequest,
  CodexOSProfilePolicy,
} from '@/types/codexIdentity'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'
import {
  createDefaultCodexIdentityPolicy,
  serializeCodexIdentityPolicy,
  validateCodexIdentityPolicy,
} from '@/utils/codexIdentityValidation'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { CodexIdentityPolicyEditor } from '@/components/account/codex-identity'

const { t, locale } = useI18n()
const appStore = useAppStore()
const templates = ref<CodexIdentityTemplate[]>([])
const proxies = ref<CodexIdentityProxyOption[]>([])
const loading = ref(false)
const loadError = ref('')
const editorOpen = ref(false)
const saving = ref(false)
const openingEditorID = ref<number | null>(null)
const editingTemplate = ref<CodexIdentityTemplate | null>(null)
const deletingTemplate = ref<CodexIdentityTemplate | null>(null)
const showUpdateConfirmation = ref(false)

const createEditorPolicy = (): CodexIdentityPolicy => ({
  ...createDefaultCodexIdentityPolicy(),
  mode: 'os_profile_device_pool',
})
const editor = reactive({
  name: '',
  description: '',
  policy: createEditorPolicy(),
})

const cloneProfile = (profile: CodexOSProfilePolicy): CodexOSProfilePolicy => ({
  os_class: profile.os_class,
  canonical_surface: profile.canonical_surface,
  architecture: profile.architecture,
  slot_count: profile.slot_count,
  proxy_mode: profile.proxy_mode,
  ...(profile.proxy_id !== undefined ? { proxy_id: profile.proxy_id } : {}),
  slots: (profile.slots ?? [])
    .filter((slot) => slot.index >= 0 && slot.index < profile.slot_count)
    .sort((left, right) => left.index - right.index)
    .map((slot) => ({
      index: slot.index,
      proxy_mode: slot.proxy_mode,
      ...(slot.proxy_id !== undefined ? { proxy_id: slot.proxy_id } : {}),
      client_version_mode: slot.client_version_mode ?? 'inherit',
      ...(slot.client_version_mode === 'pinned' && slot.client_version ? { client_version: slot.client_version } : {}),
    })),
})

const policyFromTemplate = (item: CodexIdentityTemplate): CodexIdentityPolicy => ({
  mode: 'os_profile_device_pool',
  binding_scope: 'api_key_os_surface',
  session_policy: JSON.parse(JSON.stringify(item.session_policy)),
  affinity_ttl_seconds: item.affinity_ttl_seconds,
  unsupported_policy: 'reject',
  profiles: item.profiles.map(cloneProfile),
})

const writeRequest = (): CodexIdentityTemplateWriteRequest => {
  const policy = serializeCodexIdentityPolicy(editor.policy)
  return {
    name: editor.name.trim(),
    description: editor.description.trim(),
    session_policy: policy.session_policy,
    affinity_ttl_seconds: policy.affinity_ttl_seconds,
    unsupported_policy: 'reject',
    profiles: (policy.profiles ?? []).map((profile) => ({
      ...profile,
      catalog_version: 1,
    })),
  }
}

const loadTemplates = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const [templateItems, proxyResponse] = await Promise.all([
      adminAPI.codexIdentityTemplates.list(),
      adminAPI.proxies.getAll(),
    ])
    templates.value = templateItems
    proxies.value = proxyResponse as CodexIdentityProxyOption[]
  } catch (error) {
    loadError.value = extractI18nErrorMessage(
      error,
      t,
      'admin.settings.codexProfiles.errors',
      t('admin.settings.codexProfiles.loadFailed'),
    )
  } finally {
    loading.value = false
  }
}

const setEditor = (item?: CodexIdentityTemplate) => {
  editor.name = item?.name ?? ''
  editor.description = item?.description ?? ''
  editor.policy = item ? policyFromTemplate(item) : createEditorPolicy()
}

const openCreate = (source?: CodexIdentityTemplate) => {
  editingTemplate.value = null
  setEditor(source)
  if (source) {
    editor.name = t('admin.settings.codexProfiles.copyName', { name: source.name })
  }
  editorOpen.value = true
}

const openEdit = async (item: CodexIdentityTemplate) => {
  openingEditorID.value = item.id
  try {
    const detail = await adminAPI.codexIdentityTemplates.getByID(item.id)
    editingTemplate.value = detail
    setEditor(detail)
    editorOpen.value = true
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(
      error,
      t,
      'admin.settings.codexProfiles.errors',
      t('admin.settings.codexProfiles.loadFailed'),
    ))
  } finally {
    openingEditorID.value = null
  }
}

const closeEditor = () => {
  if (saving.value) return
  editorOpen.value = false
  editingTemplate.value = null
}

const saveTemplate = async () => {
  if (!editor.name.trim()) {
    appStore.showError(t('admin.settings.codexProfiles.validation.nameRequired'))
    return
  }
  const validation = validateCodexIdentityPolicy(editor.policy, {
    availableProxyIDs: new Set(
      proxies.value
        .filter((proxy) => proxy.status === undefined || proxy.status === 'active')
        .map((proxy) => proxy.id),
    ),
  })
  if (!validation.valid) {
    appStore.showError(t('admin.settings.codexProfiles.validation.profilesInvalid'))
    return
  }

  if (editingTemplate.value && editingTemplate.value.assigned_account_count > 0) {
    showUpdateConfirmation.value = true
    return
  }
  await persistTemplate(false)
}

const persistTemplate = async (confirmAssignedAccounts: boolean) => {
  saving.value = true
  try {
    const payload = writeRequest()
    if (editingTemplate.value) {
      await adminAPI.codexIdentityTemplates.update(editingTemplate.value.id, {
        ...payload,
        expected_revision: editingTemplate.value.revision,
        confirm_assigned_accounts: confirmAssignedAccounts,
      })
      appStore.showSuccess(t('admin.settings.codexProfiles.updated'))
    } else {
      await adminAPI.codexIdentityTemplates.create(payload)
      appStore.showSuccess(t('admin.settings.codexProfiles.created'))
    }
    editorOpen.value = false
    editingTemplate.value = null
    await loadTemplates()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(
      error,
      t,
      'admin.settings.codexProfiles.errors',
      t('admin.settings.codexProfiles.saveFailed'),
    ))
  } finally {
    saving.value = false
  }
}

const updateImpactMessage = computed(() => editingTemplate.value
  ? t('admin.settings.codexProfiles.updateImpactMessage', {
      count: editingTemplate.value.assigned_account_count,
    })
  : '')
const confirmAssignedTemplateUpdate = async () => {
  showUpdateConfirmation.value = false
  await persistTemplate(true)
}

const requestDelete = (item: CodexIdentityTemplate) => {
  deletingTemplate.value = item
}
const deleteMessage = computed(() => deletingTemplate.value
  ? t('admin.settings.codexProfiles.deleteConfirm', {
      name: deletingTemplate.value.name,
      count: deletingTemplate.value.assigned_account_count,
    })
  : '')
const confirmDelete = async () => {
  const item = deletingTemplate.value
  if (!item) return
  try {
    await adminAPI.codexIdentityTemplates.delete(item.id)
    deletingTemplate.value = null
    appStore.showSuccess(t('admin.settings.codexProfiles.deleted'))
    await loadTemplates()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(
      error,
      t,
      'admin.settings.codexProfiles.errors',
      t('admin.settings.codexProfiles.deleteFailed'),
    ))
  }
}

const profileLabel = (profile: CodexOSProfilePolicy): string => {
  const os = t(`admin.accounts.codexIdentity.${profile.os_class === 'generic' ? 'genericOS' : profile.os_class}`)
  const surfaceKey = profile.canonical_surface === 'third_party' ? 'thirdParty' : profile.canonical_surface
  return `${os} / ${t(`admin.accounts.codexIdentity.${surfaceKey}`)}`
}
const formatUpdatedAt = (value: string): string =>
  formatDateTimeToMinute(value, locale.value) || t('admin.settings.codexProfiles.unknownTime')

onMounted(loadTemplates)
</script>
