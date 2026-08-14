<template>
  <div class="rounded border border-line bg-surface">
    <div class="flex items-start justify-between gap-4 border-b border-line px-4 py-3">
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-ink">
          {{ t('profile.passkey.title') }}
        </h2>
        <p class="mt-0.5 text-xs text-ink-tertiary">
          {{ t('profile.passkey.description') }}
        </p>
      </div>
      <Button
        v-if="enabled && supported && !showAddForm"
        tone="accent"
        variant="solid"
        size="md"
        class="shrink-0"
        :disabled="busy"
        @click="showAddForm = true"
      >
        {{ t('profile.passkey.add') }}
      </Button>
    </div>

    <div class="px-4 py-4">
      <p v-if="!enabled" class="mb-4 text-sm text-ink-tertiary">
        {{ t('profile.passkey.featureDisabled') }}
      </p>
      <p v-if="enabled && !supported" class="mb-4 text-sm text-warn">
        {{ t('profile.passkey.unsupported') }}
      </p>
      <div>
        <form
          v-if="enabled && supported && showAddForm"
          class="mb-4 rounded border border-line bg-surface-sunken p-3"
          @submit.prevent="addPasskey"
        >
          <div class="grid gap-3 sm:grid-cols-2">
            <FormField id="passkey-name" :label="t('profile.passkey.name')">
              <template #default="{ describedBy }">
                <input
                  id="passkey-name"
                  v-model="newName"
                  class="input"
                  maxlength="100"
                  :aria-describedby="describedBy"
                  :placeholder="t('profile.passkey.namePlaceholder')"
                  autofocus
                />
              </template>
            </FormField>
            <FormField id="passkey-add-password" :label="t('profile.currentPassword')">
              <template #default="{ describedBy }">
                <input
                  id="passkey-add-password"
                  v-model="newPassword"
                  type="password"
                  autocomplete="current-password"
                  class="input"
                  :aria-describedby="describedBy"
                  :placeholder="t('profile.passkey.passwordPlaceholder')"
                />
              </template>
            </FormField>
          </div>
          <div class="flex justify-end gap-2">
            <Button size="md" :disabled="busy" @click="cancelAdd">
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              tone="accent"
              variant="solid"
              size="md"
              :loading="busy"
              :disabled="newPassword.length === 0"
            >
              {{ t('profile.passkey.continue') }}
            </Button>
          </div>
        </form>

        <div v-if="loading" class="space-y-2 py-2">
          <div class="skeleton h-3 w-40"></div>
          <div class="skeleton h-3 w-64"></div>
        </div>

        <p
          v-else-if="credentials.length === 0"
          class="border border-line bg-surface-sunken px-4 py-8 text-center text-xs text-ink-tertiary"
        >
          {{ t('profile.passkey.empty') }}
        </p>

        <div v-else class="divide-y divide-line-subtle">
          <div
            v-for="credential in credentials"
            :key="credential.id"
            class="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Icon name="key" size="xs" class="shrink-0 text-ink-tertiary" />
                <p class="truncate text-sm font-medium text-ink">
                  {{ credential.name }}
                </p>
                <!-- A squared, bordered badge — the pill is gone, and so is the tint-only signal. -->
                <Badge v-if="credential.backup" caps>
                  {{ t('profile.passkey.synced') }}
                </Badge>
              </div>
              <p class="mt-1 text-xs text-ink-tertiary">
                {{ t('profile.passkey.createdAt', { date: formatDate(credential.created_at) }) }}
                <template v-if="credential.last_used_at">
                  · {{ t('profile.passkey.lastUsed', { date: formatDate(credential.last_used_at) }) }}
                </template>
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <Button :disabled="busy" @click="renamePasskey(credential)">
                {{ t('common.edit') }}
              </Button>
              <Button
                variant="ghost"
                tone="danger"
                :disabled="busy"
                @click="deletePasskey(credential)"
              >
                {{ t('common.delete') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 删除确认：吊销凭据需验证当前密码，防止被窃会话静默移除 Passkey -->
    <div v-if="deleteTarget" class="modal-overlay" @click.self="closeDeleteDialog">
      <div class="modal-content max-w-md" role="dialog" aria-modal="true">
        <div class="modal-header">
          <h3 class="modal-title">{{ t('profile.passkey.deleteTitle') }}</h3>
        </div>
        <form @submit.prevent="confirmDelete">
          <div class="modal-body space-y-3">
            <p class="text-sm text-ink-secondary">
              {{ t('profile.passkey.deleteConfirm', { name: deleteTarget.name }) }}
            </p>
            <FormField id="passkey-delete-password" :label="t('profile.currentPassword')">
              <template #default="{ describedBy }">
                <input
                  id="passkey-delete-password"
                  v-model="deletePassword"
                  type="password"
                  autocomplete="current-password"
                  class="input"
                  :aria-describedby="describedBy"
                  :placeholder="t('profile.passkey.passwordPlaceholder')"
                  autofocus
                />
              </template>
            </FormField>
          </div>
          <div class="modal-footer">
            <Button size="md" :disabled="busy" @click="closeDeleteDialog">
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              tone="danger"
              variant="solid"
              size="md"
              :loading="busy"
              :disabled="deletePassword.length === 0"
            >
              {{ t('common.delete') }}
            </Button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { passkeyAPI, type PasskeyCredentialSummary } from '@/api'
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Icon from '@/components/icons/Icon.vue'
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
  if (!props.enabled) {
    credentials.value = []
    return
  }
  loading.value = true
  try {
    credentials.value = await passkeyAPI.list()
  } catch (error) {
    // 字符串错误码在 reason 字段（code 是数字状态码）；
    // 设置变更竞态下后端仍可能返回 PASSKEY_DISABLED，静默处理
    const reason = (error as { reason?: string }).reason
    if (reason !== 'PASSKEY_DISABLED') {
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
    month: '2-digit',
    day: '2-digit'
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
