<template>
  <BaseDialog
    :show="show"
    :title="t('developerKeys.title')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-5">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('developerKeys.description') }}
      </p>

      <form class="flex flex-col gap-3 sm:flex-row sm:items-end" @submit.prevent="createKey">
        <div class="min-w-0 flex-1">
          <label for="developer-key-name" class="input-label">
            {{ t('developerKeys.name') }}
          </label>
          <input
            id="developer-key-name"
            v-model="newName"
            class="input"
            maxlength="100"
            :placeholder="t('developerKeys.namePlaceholder')"
            :disabled="creating"
            autocomplete="off"
          />
        </div>
        <button
          type="submit"
          class="btn btn-primary shrink-0"
          :disabled="creating || newName.trim().length === 0"
          data-testid="developer-key-create"
        >
          <Icon :name="creating ? 'refresh' : 'plus'" size="sm" :class="creating ? 'animate-spin' : ''" />
          {{ creating ? t('developerKeys.creating') : t('developerKeys.create') }}
        </button>
      </form>

      <div
        v-if="createdSecret"
        class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/30"
        data-testid="developer-key-secret"
      >
        <div class="flex items-start gap-3">
          <Icon name="key" size="md" class="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400" />
          <div class="min-w-0 flex-1">
            <p class="font-medium text-amber-900 dark:text-amber-100">
              {{ t('developerKeys.secretTitle') }}
            </p>
            <p class="mt-1 text-xs text-amber-800 dark:text-amber-200">
              {{ t('developerKeys.secretWarning') }}
            </p>
            <div class="mt-3 flex items-stretch gap-2">
              <code
                class="min-w-0 flex-1 select-all overflow-x-auto rounded-md border border-amber-200 bg-white px-3 py-2 font-mono text-xs text-gray-900 dark:border-amber-800 dark:bg-dark-900 dark:text-gray-100"
              >{{ createdSecret }}</code>
              <button
                type="button"
                class="btn btn-secondary btn-icon shrink-0"
                :aria-label="t('developerKeys.copy')"
                :title="t('developerKeys.copy')"
                data-testid="developer-key-copy"
                @click="copySecret"
              >
                <Icon :name="copied ? 'check' : 'copy'" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <div
        v-if="errorMessage"
        class="flex items-start justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
        role="alert"
        data-testid="developer-key-error"
      >
        <span>{{ errorMessage }}</span>
        <button
          v-if="loadFailed"
          type="button"
          class="shrink-0 font-medium underline underline-offset-2"
          @click="loadKeys"
        >
          {{ t('developerKeys.retry') }}
        </button>
      </div>

      <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
        <h4 class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('developerKeys.listTitle') }}
        </h4>

        <div v-if="loading" class="flex justify-center py-8" data-testid="developer-key-loading">
          <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
        </div>

        <div
          v-else-if="keys.length === 0"
          class="mt-3 rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
          data-testid="developer-key-empty"
        >
          {{ t('developerKeys.empty') }}
        </div>

        <div v-else class="mt-2 divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="key in keys"
            :key="key.id"
            class="flex items-center justify-between gap-3 py-3"
            data-testid="developer-key-item"
          >
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                {{ key.name }}
              </p>
              <p class="mt-0.5 break-all font-mono text-xs text-gray-600 dark:text-gray-300">
                {{ key.key_prefix }}...
              </p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('developerKeys.createdAt', { date: formatDateTimeToMinute(key.created_at) }) }}
                <template v-if="key.last_used_at">
                  · {{ t('developerKeys.lastUsed', { date: formatDateTimeToMinute(key.last_used_at) }) }}
                </template>
                <template v-else>
                  · {{ t('developerKeys.neverUsed') }}
                </template>
              </p>
            </div>
            <button
              type="button"
              class="btn btn-ghost btn-icon shrink-0 text-red-600 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30"
              :disabled="deletingId !== null"
              :aria-label="t('developerKeys.delete')"
              :title="t('developerKeys.delete')"
              data-testid="developer-key-delete"
              @click="deleteTarget = key"
            >
              <Icon
                :name="deletingId === key.id ? 'refresh' : 'trash'"
                size="sm"
                :class="deletingId === key.id ? 'animate-spin' : ''"
              />
            </button>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>

  <ConfirmDialog
    :show="deleteTarget !== null"
    :title="t('developerKeys.deleteTitle')"
    :message="t('developerKeys.deleteConfirm', { name: deleteTarget?.name ?? '' })"
    :confirm-text="t('common.delete')"
    danger
    @confirm="confirmDelete"
    @cancel="deleteTarget = null"
  />
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { developerKeysAPI, type DeveloperKey } from '@/api'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (event: 'close'): void }>()

const { t } = useI18n()
const appStore = useAppStore()
const { copied, copyToClipboard } = useClipboard()

const keys = ref<DeveloperKey[]>([])
const loading = ref(false)
const loadFailed = ref(false)
const creating = ref(false)
const deletingId = ref<number | null>(null)
const newName = ref('')
const createdSecret = ref('')
const deleteTarget = ref<DeveloperKey | null>(null)
const errorMessage = ref('')

async function loadKeys(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  errorMessage.value = ''
  try {
    keys.value = await developerKeysAPI.list()
  } catch (error) {
    loadFailed.value = true
    errorMessage.value = extractApiErrorMessage(error, t('developerKeys.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function createKey(): Promise<void> {
  const name = newName.value.trim()
  if (!name || creating.value) return

  creating.value = true
  loadFailed.value = false
  errorMessage.value = ''
  try {
    const result = await developerKeysAPI.create(name)
    keys.value = [result.key, ...keys.value.filter((key) => key.id !== result.key.id)]
    createdSecret.value = result.secret
    newName.value = ''
    appStore.showSuccess(t('developerKeys.createSuccess'))
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('developerKeys.createFailed'))
  } finally {
    creating.value = false
  }
}

async function copySecret(): Promise<void> {
  await copyToClipboard(createdSecret.value, t('developerKeys.copied'))
}

async function confirmDelete(): Promise<void> {
  const target = deleteTarget.value
  if (!target || deletingId.value !== null) return

  deleteTarget.value = null
  deletingId.value = target.id
  loadFailed.value = false
  errorMessage.value = ''
  try {
    await developerKeysAPI.remove(target.id)
    keys.value = keys.value.filter((key) => key.id !== target.id)
    appStore.showSuccess(t('developerKeys.deleteSuccess'))
  } catch (error) {
    deleteTarget.value = target
    errorMessage.value = extractApiErrorMessage(error, t('developerKeys.deleteFailed'))
  } finally {
    deletingId.value = null
  }
}

function resetTransientState(): void {
  newName.value = ''
  createdSecret.value = ''
  deleteTarget.value = null
  errorMessage.value = ''
  loadFailed.value = false
}

function handleClose(): void {
  resetTransientState()
  emit('close')
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      resetTransientState()
      void loadKeys()
    } else {
      resetTransientState()
    }
  },
  { immediate: true }
)
</script>
