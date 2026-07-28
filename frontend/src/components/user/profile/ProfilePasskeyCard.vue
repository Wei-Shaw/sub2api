<template>
  <div class="card">
    <div class="flex items-start justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-medium text-gray-900 dark:text-white">
          {{ t('profile.passkey.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.passkey.description') }}
        </p>
      </div>
      <button
        v-if="enabled && supported && !showAddForm"
        type="button"
        class="btn btn-primary"
        :disabled="busy"
        @click="showAddForm = true"
      >
        {{ t('profile.passkey.add') }}
      </button>
    </div>

    <div class="px-6 py-6">
      <div v-if="!enabled" class="mb-5 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.passkey.featureDisabled') }}
      </div>
      <div v-if="enabled && !supported" class="mb-5 text-sm text-amber-600 dark:text-amber-400">
        {{ t('profile.passkey.unsupported') }}
      </div>
      <div>
        <form
          v-if="enabled && supported && showAddForm"
          class="mb-5 flex flex-col gap-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700"
          @submit.prevent="addPasskey"
        >
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label for="passkey-name" class="input-label">{{ t('profile.passkey.name') }}</label>
              <input
                id="passkey-name"
                v-model="newName"
                class="input"
                maxlength="100"
                :placeholder="t('profile.passkey.namePlaceholder')"
                autofocus
              />
            </div>
            <div>
              <label for="passkey-add-password" class="input-label">{{
                t('profile.currentPassword')
              }}</label>
              <input
                id="passkey-add-password"
                v-model="newPassword"
                type="password"
                autocomplete="current-password"
                class="input"
                :placeholder="t('profile.passkey.passwordPlaceholder')"
              />
            </div>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" :disabled="busy" @click="cancelAdd">
              {{ t('common.cancel') }}
            </button>
            <button type="submit" class="btn btn-primary" :disabled="busy || newPassword.length === 0">
              {{ busy ? t('common.processing') : t('profile.passkey.continue') }}
            </button>
          </div>
        </form>

        <div v-if="loading" class="flex justify-center py-6">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-500"></div>
        </div>

        <div
          v-else-if="credentials.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        >
          {{ t('profile.passkey.empty') }}
        </div>

        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="credential in credentials"
            :key="credential.id"
            class="flex items-center justify-between gap-4 py-4 first:pt-0 last:pb-0"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Icon name="key" size="md" class="shrink-0 text-primary-500" />
                <p class="truncate font-medium text-gray-900 dark:text-white">
                  {{ credential.name }}
                </p>
                <span
                  v-if="credential.backup"
                  class="rounded-full bg-green-50 px-2 py-0.5 text-xs text-green-700 dark:bg-green-900/30 dark:text-green-300"
                >
                  {{ t('profile.passkey.synced') }}
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('profile.passkey.createdAt', { date: formatDate(credential.created_at) }) }}
                <template v-if="credential.last_used_at">
                  · {{ t('profile.passkey.lastUsed', { date: formatDate(credential.last_used_at) }) }}
                </template>
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="busy"
                @click="renamePasskey(credential)"
              >
                {{ t('common.edit') }}
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30"
                :disabled="busy"
                @click="deletePasskey(credential)"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 删除确认：吊销凭据需验证当前密码，防止被窃会话静默移除 Passkey -->
    <div v-if="deleteTarget" class="fixed inset-0 z-50 overflow-y-auto">
      <div class="flex min-h-full items-center justify-center p-4">
        <div class="fixed inset-0 bg-black/50 transition-opacity" @click="closeDeleteDialog"></div>
        <div
          class="relative w-full max-w-md transform rounded-xl bg-white p-6 shadow-xl transition-all dark:bg-dark-800"
        >
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('profile.passkey.deleteTitle') }}
          </h3>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.passkey.deleteConfirm', { name: deleteTarget.name }) }}
          </p>
          <form class="mt-4 space-y-4" @submit.prevent="confirmDelete">
            <div>
              <label for="passkey-delete-password" class="input-label">{{
                t('profile.currentPassword')
              }}</label>
              <input
                id="passkey-delete-password"
                v-model="deletePassword"
                type="password"
                autocomplete="current-password"
                class="input"
                :placeholder="t('profile.passkey.passwordPlaceholder')"
                autofocus
              />
            </div>
            <div class="flex justify-end gap-3">
              <button type="button" class="btn btn-secondary" :disabled="busy" @click="closeDeleteDialog">
                {{ t('common.cancel') }}
              </button>
              <button
                type="submit"
                class="btn btn-danger"
                :disabled="busy || deletePassword.length === 0"
              >
                {{ busy ? t('common.processing') : t('common.delete') }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { passkeyAPI, type PasskeyCredentialSummary } from '@/api'
import { Icon } from '@/components/icons'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ enabled: boolean }>()

const { t } = useI18n()
const appStore = useAppStore()
const supported = passkeyAPI.isSupported()
const loading = ref(false)
const busy = ref(false)
const showAddForm = ref(false)
const newName = ref('')
const newPassword = ref('')
const deleteTarget = ref<PasskeyCredentialSummary | null>(null)
const deletePassword = ref('')
const credentials = ref<PasskeyCredentialSummary[]>([])

// apiClient 拦截器把错误规范化为 { code, reason, message }；
// 透出后端消息（如密码错误），否则回退到通用文案。
function extractErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { message?: string }).message
  return typeof message === 'string' && message.length > 0 ? message : fallback
}

async function loadCredentials(): Promise<void> {
  loading.value = true
  try {
    credentials.value = await passkeyAPI.list()
  } catch (error) {
    const code = (error as { code?: string }).code
    if (code !== 'PASSKEY_DISABLED') {
      appStore.showError(t('profile.passkey.loadFailed'))
    }
  } finally {
    loading.value = false
  }
}

async function addPasskey(): Promise<void> {
  if (newPassword.value.length === 0) return
  busy.value = true
  try {
    await passkeyAPI.register(newName.value.trim(), newPassword.value)
    appStore.showSuccess(t('profile.passkey.added'))
    cancelAdd()
    await loadCredentials()
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'NotAllowedError')) {
      appStore.showError(extractErrorMessage(error, t('profile.passkey.addFailed')))
    }
  } finally {
    busy.value = false
  }
}

function cancelAdd(): void {
  showAddForm.value = false
  newName.value = ''
  newPassword.value = ''
}

async function renamePasskey(credential: PasskeyCredentialSummary): Promise<void> {
  const name = window.prompt(t('profile.passkey.renamePrompt'), credential.name)?.trim()
  if (!name || name === credential.name) return
  busy.value = true
  try {
    await passkeyAPI.rename(credential.id, name)
    credential.name = name
    appStore.showSuccess(t('profile.passkey.renamed'))
  } catch {
    appStore.showError(t('profile.passkey.renameFailed'))
  } finally {
    busy.value = false
  }
}

function deletePasskey(credential: PasskeyCredentialSummary): void {
  deleteTarget.value = credential
  deletePassword.value = ''
}

function closeDeleteDialog(): void {
  deleteTarget.value = null
  deletePassword.value = ''
}

async function confirmDelete(): Promise<void> {
  const credential = deleteTarget.value
  if (!credential || deletePassword.value.length === 0) return
  busy.value = true
  try {
    await passkeyAPI.remove(credential.id, deletePassword.value)
    credentials.value = credentials.value.filter((item) => item.id !== credential.id)
    appStore.showSuccess(t('profile.passkey.deleted'))
    closeDeleteDialog()
  } catch (error) {
    // 密码错误等失败保持对话框打开，允许重试
    appStore.showError(extractErrorMessage(error, t('profile.passkey.deleteFailed')))
  } finally {
    busy.value = false
  }
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  }).format(new Date(value))
}

watch(
  () => props.enabled,
  () => {
    void loadCredentials()
  },
  { immediate: true }
)
</script>
