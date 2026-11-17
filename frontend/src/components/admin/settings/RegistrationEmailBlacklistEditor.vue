<template>
  <div class="border-t border-gray-100 pt-4 dark:border-dark-700" data-testid="email-blacklist-editor">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0">
        <label class="font-medium text-gray-900 dark:text-white">
          {{ t('admin.settings.registration.emailBlacklist.title') }}
        </label>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.registration.emailBlacklist.hint') }}
        </p>
        <p class="mt-2 text-sm" data-testid="email-blacklist-summary">
          <span v-if="loading" class="text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</span>
          <span v-else-if="loadError" class="text-red-600 dark:text-red-400">
            {{ t('admin.settings.registration.emailBlacklist.loadFailed') }}
            <button type="button" class="ml-2 underline" @click="load">{{ t('admin.settings.registration.emailBlacklist.retry') }}</button>
          </span>
          <span v-else class="text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.registration.emailBlacklist.count', { count: formatCount(savedEmails.length) }) }}
          </span>
        </p>
      </div>
      <div class="flex shrink-0 gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading || loadError || savedEmails.length === 0 || checking"
          data-testid="email-blacklist-check"
          @click="checkRegisteredUsers(savedEmails)"
        >
          <Icon name="search" size="sm" class="mr-1" />
          {{ t('admin.settings.registration.emailBlacklist.checkUsers') }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loading || loadError || checking"
          data-testid="email-blacklist-edit"
          @click="openEditor"
        >
          {{ t('admin.settings.registration.emailBlacklist.edit') }}
        </button>
      </div>
    </div>

    <BaseDialog
      :show="editorOpen"
      :title="t('admin.settings.registration.emailBlacklist.editorTitle')"
      width="wide"
      :close-on-escape="false"
      @close="closeEditor"
    >
      <div class="space-y-5">
        <section>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="email-blacklist-bulk">
            {{ t('admin.settings.registration.emailBlacklist.bulkLabel') }}
          </label>
          <textarea
            id="email-blacklist-bulk"
            v-model="bulkInput"
            rows="5"
            class="input mt-2 w-full font-mono text-sm"
            :placeholder="t('admin.settings.registration.emailBlacklist.bulkPlaceholder')"
            data-testid="email-blacklist-bulk-input"
          />
          <div class="mt-2 flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="!bulkInput.trim()"
              data-testid="email-blacklist-bulk-add"
              @click="addBulk"
            >
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.settings.registration.emailBlacklist.bulkAdd') }}
            </button>
            <span v-if="addResult" class="text-sm text-gray-600 dark:text-gray-400" data-testid="email-blacklist-add-result">
              {{
                t('admin.settings.registration.emailBlacklist.addResult', {
                  added: formatCount(addResult.added),
                  duplicates: formatCount(addResult.duplicates)
                })
              }}
            </span>
          </div>
          <p
            v-if="addResult && addResult.invalid.length > 0"
            class="mt-2 break-all text-sm text-amber-600 dark:text-amber-400"
            data-testid="email-blacklist-invalid"
          >
            {{
              t('admin.settings.registration.emailBlacklist.invalidResult', {
                count: formatCount(addResult.invalid.length),
                sample: invalidSample
              })
            }}
          </p>
        </section>

        <section>
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.registration.emailBlacklist.listLabel', { count: formatCount(draft.length) }) }}
              <span
                v-if="pendingAdded > 0 || pendingRemoved > 0"
                class="ml-2 font-normal text-primary-600 dark:text-primary-400"
                data-testid="email-blacklist-pending"
              >
                {{
                  t('admin.settings.registration.emailBlacklist.pending', {
                    added: formatCount(pendingAdded),
                    removed: formatCount(pendingRemoved)
                  })
                }}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <input
                v-model="search"
                type="search"
                class="input w-full text-sm sm:w-64"
                :placeholder="t('admin.settings.registration.emailBlacklist.searchPlaceholder')"
                data-testid="email-blacklist-search"
              />
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0"
                :disabled="draft.length === 0"
                data-testid="email-blacklist-clear"
                @click="clearDraft"
              >
                {{ t('admin.settings.registration.emailBlacklist.clear') }}
              </button>
            </div>
          </div>

          <div class="mt-2 max-h-80 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
            <p v-if="draft.length === 0" class="p-4 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.registration.emailBlacklist.empty') }}
            </p>
            <p v-else-if="filtered.length === 0" class="p-4 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.registration.emailBlacklist.noMatch') }}
            </p>
            <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
              <li
                v-for="email in visible"
                :key="email"
                class="flex items-center justify-between gap-3 px-3 py-1.5"
                data-testid="email-blacklist-row"
              >
                <span class="min-w-0 truncate font-mono text-sm text-gray-800 dark:text-gray-200" :title="email">
                  {{ email }}
                  <span
                    v-if="!savedSet.has(email)"
                    class="ml-2 rounded bg-primary-50 px-1.5 py-0.5 font-sans text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                  >
                    {{ t('admin.settings.registration.emailBlacklist.newTag') }}
                  </span>
                </span>
                <button
                  type="button"
                  class="shrink-0 rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-dark-700 dark:hover:text-red-400"
                  :title="t('admin.settings.registration.emailBlacklist.remove')"
                  :aria-label="t('admin.settings.registration.emailBlacklist.remove')"
                  @click="removeEmail(email)"
                >
                  <Icon name="x" size="sm" />
                </button>
              </li>
            </ul>
          </div>
          <p v-if="filtered.length > VISIBLE_LIMIT" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.registration.emailBlacklist.truncated', { limit: VISIBLE_LIMIT, total: formatCount(filtered.length) }) }}
          </p>
          <p v-if="draft.length > maxEntries" class="mt-2 text-sm text-red-600 dark:text-red-400" data-testid="email-blacklist-too-many">
            {{ t('admin.settings.registration.emailBlacklist.tooMany', { max: formatCount(maxEntries) }) }}
          </p>
        </section>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeEditor">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving || !dirty || draft.length > maxEntries"
            data-testid="email-blacklist-save"
            @click="save"
          >
            {{ saving ? t('common.saving') : t('admin.settings.registration.emailBlacklist.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="discardConfirmOpen"
      :title="t('admin.settings.registration.emailBlacklist.discardTitle')"
      :message="
        t('admin.settings.registration.emailBlacklist.discardMessage', {
          added: formatCount(pendingAdded),
          removed: formatCount(pendingRemoved)
        })
      "
      :confirm-text="t('admin.settings.registration.emailBlacklist.discardConfirm')"
      :cancel-text="t('admin.settings.registration.emailBlacklist.discardCancel')"
      danger
      @confirm="discardDraft"
      @cancel="discardConfirmOpen = false"
    />

    <BaseDialog
      :show="disableDialogOpen"
      :title="t('admin.settings.registration.emailBlacklist.disableTitle')"
      width="normal"
      :z-index="60"
      @close="closeDisableDialog"
    >
      <div class="space-y-3">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('admin.settings.registration.emailBlacklist.disableMessage', { count: formatCount(matchedUsers.length) }) }}
        </p>
        <ul
          class="max-h-72 divide-y divide-gray-100 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-600"
          data-testid="email-blacklist-matched-users"
        >
          <li v-for="user in matchedUsers" :key="user.id" class="flex items-center justify-between gap-3 px-3 py-1.5 text-sm">
            <span class="min-w-0 truncate font-mono text-gray-800 dark:text-gray-200" :title="user.email">{{ user.email }}</span>
            <span class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
              <template v-if="user.username">{{ user.username }} · </template>#{{ user.id }}
            </span>
          </li>
        </ul>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="disabling" @click="closeDisableDialog">
            {{ t('admin.settings.registration.emailBlacklist.disableLater') }}
          </button>
          <button
            type="button"
            class="btn btn-danger"
            :disabled="disabling"
            data-testid="email-blacklist-disable-confirm"
            @click="disableMatchedUsers"
          >
            {{ t('admin.settings.registration.emailBlacklist.disableConfirm', { count: formatCount(matchedUsers.length) }) }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { BlacklistedEmailUser } from '@/api/admin/users'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { mergeIntoBlacklist, parseBlacklistInput } from '@/utils/registrationEmailBlacklist'

const VISIBLE_LIMIT = 200
const INVALID_SAMPLE_LIMIT = 10
const DISABLE_BATCH_SIZE = 1000

const { t, locale } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const loadError = ref(false)
const savedEmails = ref<string[]>([])
const maxEntries = ref(20000)

const editorOpen = ref(false)
const draft = ref<string[]>([])
const bulkInput = ref('')
const search = ref('')
const saving = ref(false)
const discardConfirmOpen = ref(false)
const addResult = ref<{ added: number; duplicates: number; invalid: string[] } | null>(null)

const checking = ref(false)
const disableDialogOpen = ref(false)
const disabling = ref(false)
const matchedUsers = ref<BlacklistedEmailUser[]>([])

const savedSet = computed(() => new Set(savedEmails.value))
const draftSet = computed(() => new Set(draft.value))
const pendingAdded = computed(() => draft.value.filter((email) => !savedSet.value.has(email)).length)
const pendingRemoved = computed(() => savedEmails.value.filter((email) => !draftSet.value.has(email)).length)
const dirty = computed(() => pendingAdded.value > 0 || pendingRemoved.value > 0)

const filtered = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) {
    return draft.value
  }
  return draft.value.filter((email) => email.includes(keyword))
})
const visible = computed(() => filtered.value.slice(0, VISIBLE_LIMIT))
const invalidSample = computed(() => {
  const invalid = addResult.value?.invalid ?? []
  const sample = invalid.slice(0, INVALID_SAMPLE_LIMIT).join(', ')
  return invalid.length > INVALID_SAMPLE_LIMIT ? `${sample} …` : sample
})

function formatCount(value: number): string {
  return value.toLocaleString(locale.value)
}

async function load() {
  loading.value = true
  loadError.value = false
  try {
    const result = await adminAPI.settings.getRegistrationEmailBlacklist()
    savedEmails.value = result.emails ?? []
    if (result.max > 0) {
      maxEntries.value = result.max
    }
  } catch (error) {
    loadError.value = true
    console.error('Failed to load registration email blacklist:', error)
  } finally {
    loading.value = false
  }
}

function openEditor() {
  draft.value = [...savedEmails.value]
  bulkInput.value = ''
  search.value = ''
  addResult.value = null
  editorOpen.value = true
}

function closeEditor() {
  if (saving.value) {
    return
  }
  if (dirty.value) {
    discardConfirmOpen.value = true
    return
  }
  editorOpen.value = false
}

function discardDraft() {
  discardConfirmOpen.value = false
  editorOpen.value = false
}

function addBulk() {
  const parsed = parseBlacklistInput(bulkInput.value)
  const merged = mergeIntoBlacklist(draft.value, parsed.valid)
  draft.value = merged.list
  addResult.value = {
    added: merged.added.length,
    duplicates: parsed.duplicates + merged.duplicates,
    invalid: parsed.invalid
  }
  bulkInput.value = parsed.invalid.join('\n')
  search.value = ''
}

function removeEmail(email: string) {
  draft.value = draft.value.filter((item) => item !== email)
}

function clearDraft() {
  draft.value = []
  search.value = ''
}

async function save() {
  saving.value = true
  let added: string[] = []
  try {
    const result = await adminAPI.settings.updateRegistrationEmailBlacklist(draft.value)
    savedEmails.value = result.emails ?? []
    added = result.added ?? []
    appStore.showSuccess(
      t('admin.settings.registration.emailBlacklist.saved', { count: formatCount(savedEmails.value.length) })
    )
    editorOpen.value = false
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
    return
  } finally {
    saving.value = false
  }
  if (added.length > 0) {
    await checkRegisteredUsers(added, { silentWhenNone: true })
  }
}

async function checkRegisteredUsers(emails: string[], options: { silentWhenNone?: boolean } = {}) {
  if (emails.length === 0) {
    return
  }
  checking.value = true
  try {
    const result = await adminAPI.users.findBlacklistedEmailUsers(emails)
    matchedUsers.value = result.items ?? []
    if (matchedUsers.value.length > 0) {
      disableDialogOpen.value = true
    } else if (!options.silentWhenNone) {
      appStore.showInfo(t('admin.settings.registration.emailBlacklist.noActiveMatches'))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    checking.value = false
  }
}

function closeDisableDialog() {
  if (disabling.value) {
    return
  }
  disableDialogOpen.value = false
}

// 分批提交；失败时弹窗只保留尚未提交的用户，重试从断点继续（已禁用的用户后端会跳过）。
async function disableMatchedUsers() {
  disabling.value = true
  let affected = 0
  try {
    while (matchedUsers.value.length > 0) {
      const batch = matchedUsers.value.slice(0, DISABLE_BATCH_SIZE)
      const result = await adminAPI.users.batchDisable(batch.map((user) => user.id))
      affected += result.affected
      matchedUsers.value = matchedUsers.value.slice(batch.length)
    }
    appStore.showSuccess(
      t('admin.settings.registration.emailBlacklist.disabled', { count: formatCount(affected) })
    )
    disableDialogOpen.value = false
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    disabling.value = false
  }
}

onMounted(load)
</script>
