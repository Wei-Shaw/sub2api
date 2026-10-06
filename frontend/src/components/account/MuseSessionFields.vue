<template>
  <div class="space-y-4">
    <p class="input-hint">{{ t('admin.accounts.muse.setupNote') }}</p>
    <div>
      <label class="input-label" for="muse-owner">{{ t('admin.accounts.muse.owner') }}</label>
      <Select
        id="muse-owner"
        :model-value="ownerId || null"
        :options="userOptions"
        :aria-label="t('admin.accounts.muse.owner')"
        aria-describedby="muse-owner-note"
        :placeholder="t('admin.accounts.muse.chooseUser')"
        :search-placeholder="t('admin.accounts.muse.searchUsers')"
        :empty-text="t('admin.accounts.muse.noUsers')"
        :loading="usersLoading"
        :error="usersError || ownerLookupError"
        searchable remote clearable
        @search="loadUsers"
        @update:model-value="emit('update:ownerId', Number($event) || 0)"
      >
        <template #selected>{{ selectedUserLabel }}</template>
      </Select>
      <p id="muse-owner-note" class="input-hint">{{ t('admin.accounts.muse.ownerNote') }}</p>
      <p v-if="hasMoreUsers" class="input-hint">{{ t('admin.accounts.muse.moreUsers') }}</p>
      <div v-if="usersError || ownerLookupError" role="alert" class="mt-2 flex flex-wrap items-center gap-2 text-sm text-red-600 dark:text-red-400">
        <span>{{ t('admin.accounts.muse.usersError') }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="retryUsers">{{ t('admin.accounts.retry') }}</button>
      </div>
    </div>
    <div class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="muse-session-import">
      <div>
        <p class="font-medium">{{ t('admin.accounts.muse.session') }}</p>
        <p class="input-hint">{{ t('admin.accounts.muse.fileIntro') }}</p>
      </div>
      <div
        class="rounded-lg border border-dashed border-gray-300 bg-gray-50 p-4 text-center dark:border-dark-500 dark:bg-dark-700"
        @dragover.prevent
        @drop.prevent="dropFile"
      >
        <label class="btn btn-primary cursor-pointer focus-within:ring-2 focus-within:ring-primary-500 focus-within:ring-offset-2" :class="{ 'pointer-events-none opacity-60': importing }">
          {{ t(replacement ? 'admin.accounts.muse.replaceFile' : 'admin.accounts.muse.importFile') }}
          <input type="file" accept=".json,application/json" class="sr-only" :disabled="importing" :aria-label="t(replacement ? 'admin.accounts.muse.replaceFile' : 'admin.accounts.muse.importFile')" @change="chooseFile" />
        </label>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.muse.dropFile') }}</p>
      </div>
      <p v-if="fileError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ fileError }}</p>
      <p v-if="sessionReady" role="status" class="text-sm text-green-700 dark:text-green-400">
        {{ t(fileName ? 'admin.accounts.muse.fileReady' : 'admin.accounts.muse.sessionReady') }} <span v-if="fileName" class="break-all">{{ fileName }}</span>
      </p>
      <p v-else-if="replacement" class="input-hint">{{ t('admin.accounts.muse.savedSession') }}</p>
      <details class="text-sm" data-testid="muse-export-guide">
        <summary class="cursor-pointer font-medium text-primary-600 dark:text-primary-400">{{ t('admin.accounts.muse.exportTitle') }}</summary>
        <div class="mt-3 space-y-3">
          <p class="input-hint">{{ t('admin.accounts.muse.exportDesktop') }}</p>
          <!-- Increment the download version when rebuilding the exporter ZIP. -->
          <a href="/muse-session-exporter.zip?v=1" download class="btn btn-secondary btn-sm inline-flex">
            {{ t('admin.accounts.muse.downloadExporter') }}
          </a>
          <ol class="list-decimal space-y-2 pl-5 text-gray-600 dark:text-gray-300">
            <li>{{ t('admin.accounts.muse.exportUnzip') }}</li>
            <li>{{ t('admin.accounts.muse.exportInstall') }} <code class="text-xs">chrome://extensions</code> / <code class="text-xs">edge://extensions</code></li>
            <li>{{ t('admin.accounts.muse.exportLogin') }} <a href="https://muse.ai/" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline dark:text-primary-400">muse.ai</a></li>
            <li>{{ t('admin.accounts.muse.exportDownload') }}</li>
            <li>{{ t('admin.accounts.muse.exportImport') }}</li>
          </ol>
        </div>
      </details>
      <details class="text-sm" data-testid="muse-manual-session">
        <summary class="cursor-pointer text-gray-600 dark:text-gray-300">{{ t('admin.accounts.muse.pasteInstead') }}</summary>
        <label class="input-label mt-3" for="muse-session">{{ t('admin.accounts.muse.pasteLabel') }}</label>
        <textarea id="muse-session" :value="sessionJson" class="input font-mono" rows="4" autocomplete="off"
          :placeholder="replacement ? t('admin.accounts.muse.keepSession') : t('admin.accounts.muse.sessionPlaceholder')"
          @input="pasteSession(($event.target as HTMLTextAreaElement).value)" />
      </details>
      <p class="input-hint">{{ t('admin.accounts.muse.exportPrivate') }}</p>
      <p class="input-hint">{{ t('admin.accounts.muse.afterSave') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'
import type { AdminUser } from '@/types'
import { parseMuseSessionDocument } from './museSession'

const props = defineProps<{ ownerId: number; sessionJson: string; replacement?: boolean; accountId?: number }>()
const emit = defineEmits<{ 'update:ownerId': [value: number]; 'update:sessionJson': [value: string] }>()
const { t } = useI18n()
type OwnerUser = Pick<AdminUser, 'id' | 'email' | 'username' | 'status'>
const users = ref<OwnerUser[]>([])
const knownUsers = ref<Record<number, OwnerUser>>({})
const usersLoading = ref(false)
const usersError = ref(false)
const failedOwnerId = ref(0)
const hasMoreUsers = ref(false)
let userSearch = ''
let searchSequence = 0
let ownerSequence = 0
let searchController: AbortController | null = null

const ownerLookupError = computed(() => failedOwnerId.value === props.ownerId && props.ownerId > 0 && !knownUsers.value[props.ownerId])
function userLabel(user: OwnerUser) {
  return user.username ? user.username + ' — ' + user.email : user.email
}
const userOptions = computed(() => users.value.map(user => ({ value: user.id, label: userLabel(user), disabled: user.status === 'disabled' })))
const selectedUserLabel = computed(() => {
  const owner = knownUsers.value[props.ownerId]
  if (owner) return userLabel(owner)
  if (props.ownerId > 0) return t(ownerLookupError.value ? 'admin.accounts.muse.assignedUserUnavailable' : 'common.loading')
  return t('admin.accounts.muse.chooseUser')
})

async function loadUsers(query = '') {
  userSearch = query
  const sequence = ++searchSequence
  searchController?.abort()
  searchController = new AbortController()
  users.value = []
  hasMoreUsers.value = false
  usersLoading.value = true
  usersError.value = false
  try {
    const result = await adminAPI.users.list(1, 30, { search: query.trim(), include_subscriptions: false }, { signal: searchController.signal })
    if (sequence !== searchSequence) return
    users.value = result.items.map(({ id, email, username, status }) => ({ id, email, username, status }))
    hasMoreUsers.value = result.total > result.items.length
    const selected = knownUsers.value[props.ownerId]
    knownUsers.value = selected ? { [selected.id]: selected } : {}
    for (const user of users.value) knownUsers.value[user.id] = user
  } catch {
    if (sequence !== searchSequence) return
    users.value = []
    hasMoreUsers.value = false
    usersError.value = true
  } finally {
    if (sequence === searchSequence) usersLoading.value = false
  }
}

async function hydrateOwner(id: number) {
  const sequence = ++ownerSequence
  failedOwnerId.value = 0
  if (!Number.isSafeInteger(id) || id <= 0 || knownUsers.value[id]) return
  try {
    const user = await adminAPI.users.getById(id)
    if (sequence === ownerSequence && props.ownerId === id) {
      knownUsers.value[id] = { id: user.id, email: user.email, username: user.username, status: user.status }
    }
  } catch {
    if (sequence === ownerSequence && props.ownerId === id) failedOwnerId.value = id
  }
}

function retryUsers() {
  void loadUsers(userSearch)
  if (ownerLookupError.value) void hydrateOwner(props.ownerId)
}

watch(() => props.ownerId, hydrateOwner, { immediate: true })
onMounted(() => { void loadUsers() })
onUnmounted(() => { searchSequence++; ownerSequence++; importSequence++; searchController?.abort() })

const fileError = ref('')
const fileName = ref('')
const importing = ref(false)
let importSequence = 0
const sessionReady = computed(() => {
  try { parseMuseSessionDocument(props.sessionJson); return true } catch { return false }
})
watch(() => props.sessionJson, raw => { if (!raw.trim()) fileName.value = '' })
watch(() => props.accountId, () => { importSequence++; importing.value = false; fileError.value = ''; fileName.value = '' }, { flush: 'sync' })

async function importFile(file: File) {
  if (importing.value) return
  const sequence = ++importSequence
  const accountId = props.accountId
  importing.value = true
  fileError.value = ''
  try {
    if (file.size > 64 * 1024) throw new Error('too large')
    const raw = await file.text()
    if (sequence !== importSequence || props.accountId !== accountId) return
    parseMuseSessionDocument(raw)
    fileName.value = file.name
    emit('update:sessionJson', raw)
  } catch {
    if (sequence === importSequence) fileError.value = t('admin.accounts.muse.importFileError')
  } finally {
    if (sequence === importSequence) importing.value = false
  }
}

async function chooseFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) await importFile(file)
  input.value = ''
}

function dropFile(event: DragEvent) {
  const files = event.dataTransfer?.files
  if (files?.length !== 1) {
    fileError.value = t('admin.accounts.muse.importFileError')
    return
  }
  void importFile(files[0])
}

function pasteSession(raw: string) {
  fileName.value = ''
  fileError.value = ''
  emit('update:sessionJson', raw)
}
</script>
