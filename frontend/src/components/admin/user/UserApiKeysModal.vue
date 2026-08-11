<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div class="min-w-0 flex-1"><p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p><p class="text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p></div>
        <button
          type="button"
          class="btn btn-primary btn-sm shrink-0"
          :disabled="creating"
          @click="toggleCreateForm"
        >
          {{ showCreateForm ? t('admin.users.cancelCreate') : t('admin.users.createApiKey') }}
        </button>
      </div>

      <!-- Newly created key notice (shown once) -->
      <div
        v-if="createdKey"
        class="rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-primary-900/20"
      >
        <p class="mb-2 text-sm font-medium text-primary-800 dark:text-primary-200">{{ createdKey.name }}</p>
        <div class="flex items-center gap-2">
          <code class="min-w-0 flex-1 truncate rounded-lg bg-white px-3 py-2 font-mono text-sm text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ createdKey.key }}</code>
          <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="copyCreatedKey">{{ t('admin.users.copyKey') }}</button>
        </div>
        <p class="mt-2 text-xs text-primary-700 dark:text-primary-300">{{ t('admin.users.createdKeyNotice') }}</p>
      </div>

      <!-- Create form -->
      <form
        v-if="showCreateForm"
        class="space-y-3 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700/50"
        @submit.prevent="submitCreate"
      >
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyName') }} <span class="text-red-500">*</span></label>
          <input v-model.trim="createForm.name" type="text" class="input w-full" :placeholder="t('admin.users.apiKeyNamePlaceholder')" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyGroup') }}</label>
          <select v-model="createForm.groupId" class="input w-full">
            <option :value="null">{{ t('admin.users.apiKeyGroupNone') }}</option>
            <option v-for="group in availableGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
          </select>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyQuota') }}</label>
            <input v-model.number="createForm.quota" type="number" min="0" step="any" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyExpiresInDays') }}</label>
            <input v-model.number="createForm.expiresInDays" type="number" min="1" step="1" class="input w-full" />
          </div>
        </div>
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyRateLimit5h') }}</label>
            <input v-model.number="createForm.rateLimit5h" type="number" min="0" step="any" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyRateLimit1d') }}</label>
            <input v-model.number="createForm.rateLimit1d" type="number" min="0" step="any" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyRateLimit7d') }}</label>
            <input v-model.number="createForm.rateLimit7d" type="number" min="0" step="any" class="input w-full" />
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.users.apiKeyCustomKey') }}</label>
          <input v-model.trim="createForm.customKey" type="text" class="input w-full font-mono" :placeholder="t('admin.users.apiKeyCustomKeyPlaceholder')" />
        </div>
        <div class="flex justify-end">
          <button type="submit" class="btn btn-primary btn-sm" :disabled="creating || !createForm.name">
            <svg v-if="creating" class="mr-1 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg>
            {{ t('admin.users.submitCreate') }}
          </button>
        </div>
      </form>

      <div v-if="loading" class="flex justify-center py-8"><svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg></div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center"><p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p></div>
      <div v-else ref="scrollContainerRef" class="max-h-96 space-y-3 overflow-y-auto" @scroll="closeGroupSelector">
        <div v-for="key in apiKeys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex items-center gap-2"><span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span><span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">{{ key.status }}</span></div>
              <p class="truncate font-mono text-sm text-gray-500">{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}</p>
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
            <div class="flex items-center gap-1">
              <span>{{ t('admin.users.group') }}:</span>
              <button
                :ref="(el) => setGroupButtonRef(key.id, el)"
                @click="openGroupSelector(key)"
                class="-mx-1 -my-0.5 flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :disabled="updatingKeyIds.has(key.id)"
              >
                <GroupBadge
                  v-if="key.group_id && key.group"
                  :name="key.group.name"
                  :platform="key.group.platform"
                  :subscription-type="key.group.subscription_type"
                  :rate-multiplier="key.group.rate_multiplier"
                  :peak-rate-enabled="key.group.peak_rate_enabled"
                  :peak-start="key.group.peak_start"
                  :peak-end="key.group.peak_end"
                  :peak-rate-multiplier="key.group.peak_rate_multiplier"
                />
                <span v-else class="text-gray-400 italic">{{ t('admin.users.none') }}</span>
                <svg v-if="updatingKeyIds.has(key.id)" class="h-3 w-3 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                <svg v-else class="h-3 w-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" /></svg>
              </button>
            </div>
            <div class="flex items-center gap-1"><span>{{ t('admin.users.columns.created') }}: {{ formatDateTime(key.created_at) }}</span></div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>

  <!-- Group Selector Dropdown -->
  <Teleport to="body">
    <div
      v-if="groupSelectorKeyId !== null && dropdownPosition"
      ref="dropdownRef"
      class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-64 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
      :style="{ top: dropdownPosition.top + 'px', left: dropdownPosition.left + 'px' }"
    >
      <div class="max-h-64 overflow-y-auto p-1.5">
        <!-- Unbind option -->
        <button
          @click="changeGroup(selectedKeyForGroup!, null)"
          :class="[
            'flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors',
            !selectedKeyForGroup?.group_id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <span class="text-gray-500 italic">{{ t('admin.users.none') }}</span>
          <svg
            v-if="!selectedKeyForGroup?.group_id"
            class="ml-auto h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"
          ><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
        </button>
        <!-- Group options -->
        <button
          v-for="group in allGroups"
          :key="group.id"
          @click="changeGroup(selectedKeyForGroup!, group.id)"
          :class="[
            'flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
            selectedKeyForGroup?.group_id === group.id
              ? 'bg-primary-50 dark:bg-primary-900/20'
              : 'hover:bg-gray-100 dark:hover:bg-dark-700'
          ]"
        >
          <GroupOptionItem
            :name="group.name"
            :platform="group.platform"
            :subscription-type="group.subscription_type"
            :rate-multiplier="group.rate_multiplier"
            :peak-rate-enabled="group.peak_rate_enabled"
            :peak-start="group.peak_start"
            :peak-end="group.peak_end"
            :peak-rate-multiplier="group.peak_rate_multiplier"
            :description="group.description"
            :selected="selectedKeyForGroup?.group_id === group.id"
          />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import type { AdminUser, AdminGroup, ApiKey, CreateApiKeyRequest } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close'])
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const availableGroups = ref<AdminGroup[]>([])
const loading = ref(false)

// Create-on-behalf-of state
const showCreateForm = ref(false)
const creating = ref(false)
const createdKey = ref<ApiKey | null>(null)
const createForm = reactive<{
  name: string
  groupId: number | null
  customKey: string
  quota: number | null
  expiresInDays: number | null
  rateLimit5h: number | null
  rateLimit1d: number | null
  rateLimit7d: number | null
}>({
  name: '',
  groupId: null,
  customKey: '',
  quota: null,
  expiresInDays: null,
  rateLimit5h: null,
  rateLimit1d: null,
  rateLimit7d: null,
})

const resetCreateForm = () => {
  createForm.name = ''
  createForm.groupId = null
  createForm.customKey = ''
  createForm.quota = null
  createForm.expiresInDays = null
  createForm.rateLimit5h = null
  createForm.rateLimit1d = null
  createForm.rateLimit7d = null
}

const toggleCreateForm = () => {
  showCreateForm.value = !showCreateForm.value
  if (!showCreateForm.value) resetCreateForm()
}

const copyCreatedKey = () => {
  if (createdKey.value) copyToClipboard(createdKey.value.key, t('keys.copied'))
}

// Reads a numeric form field. `v-model.number` yields '' for an emptied input
// (not null), so treat empty/null as "unset". A non-empty value must be a
// finite, non-negative number, otherwise it is rejected (rather than silently
// dropped by a `>= 0` guard).
const readNonNegative = (raw: number | null): { value: number | null; invalid: boolean } => {
  if (raw === null || (raw as unknown) === '') return { value: null, invalid: false }
  if (typeof raw !== 'number' || !Number.isFinite(raw) || raw < 0) return { value: null, invalid: true }
  return { value: raw, invalid: false }
}

const submitCreate = async () => {
  if (!props.user) return
  const name = createForm.name.trim()
  if (!name) {
    appStore.showError(t('admin.users.apiKeyNameRequired'))
    return
  }
  const payload: CreateApiKeyRequest = { name }
  if (createForm.groupId != null) payload.group_id = createForm.groupId
  if (createForm.customKey.trim()) payload.custom_key = createForm.customKey.trim()

  const quota = readNonNegative(createForm.quota)
  const rl5h = readNonNegative(createForm.rateLimit5h)
  const rl1d = readNonNegative(createForm.rateLimit1d)
  const rl7d = readNonNegative(createForm.rateLimit7d)
  // expires_in_days must be a positive integer when provided.
  const rawExpires = createForm.expiresInDays
  const expiresUnset = rawExpires === null || (rawExpires as unknown) === ''
  const expiresInvalid =
    !expiresUnset &&
    (typeof rawExpires !== 'number' || !Number.isInteger(rawExpires) || rawExpires <= 0)

  if (quota.invalid || rl5h.invalid || rl1d.invalid || rl7d.invalid || expiresInvalid) {
    appStore.showError(t('admin.users.apiKeyLimitInvalid'))
    return
  }
  if (quota.value != null) payload.quota = quota.value
  if (!expiresUnset) payload.expires_in_days = rawExpires as number
  if (rl5h.value != null) payload.rate_limit_5h = rl5h.value
  if (rl1d.value != null) payload.rate_limit_1d = rl1d.value
  if (rl7d.value != null) payload.rate_limit_7d = rl7d.value

  creating.value = true
  try {
    const created = await adminAPI.users.createUserApiKey(props.user.id, payload)
    createdKey.value = created
    showCreateForm.value = false
    resetCreateForm()
    appStore.showSuccess(t('admin.users.createKeySuccess'))
    // Reload rather than unshift(created): the create response does not hydrate
    // the Group object, so an inserted grouped key would render as "None" until
    // a refresh. createdKey (with plaintext) is shown separately above the list.
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.createKeyFailed'))
  } finally {
    creating.value = false
  }
}
const updatingKeyIds = ref(new Set<number>())
const groupSelectorKeyId = ref<number | null>(null)
const dropdownPosition = ref<{ top: number; left: number } | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const scrollContainerRef = ref<HTMLElement | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())

const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

watch(() => props.show, (v) => {
  if (v && props.user) {
    // Reset create state on each open so a previous key is never re-shown.
    showCreateForm.value = false
    createdKey.value = null
    resetCreateForm()
    load()
    loadGroups()
  } else {
    closeGroupSelector()
  }
})

const load = async () => {
  if (!props.user) return
  loading.value = true
  groupButtonRefs.value.clear()
  try {
    const res = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = res.items || []
  } catch (error) {
    console.error('Failed to load API keys:', error)
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  if (!props.user) return
  try {
    // allGroups drives the change-group dropdown for EXISTING keys, whose
    // server path (AdminUpdateAPIKeyGroupID) auto-grants access — so it must
    // list every group. availableGroups drives the CREATE form, which goes
    // through APIKeyService.Create (no auto-grant) and must only offer groups
    // the target user can actually bind.
    const [all, available] = await Promise.all([
      adminAPI.groups.getAll(),
      adminAPI.users.getUserAvailableGroups(props.user.id),
    ])
    allGroups.value = all
    availableGroups.value = available
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const DROPDOWN_HEIGHT = 272 // max-h-64 = 16rem = 256px + padding
const DROPDOWN_GAP = 4

const openGroupSelector = (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    closeGroupSelector()
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const spaceBelow = window.innerHeight - rect.bottom
      const openUpward = spaceBelow < DROPDOWN_HEIGHT && rect.top > spaceBelow
      dropdownPosition.value = {
        top: openUpward ? rect.top - DROPDOWN_HEIGHT - DROPDOWN_GAP : rect.bottom + DROPDOWN_GAP,
        left: rect.left
      }
    }
    groupSelectorKeyId.value = key.id
  }
}

const closeGroupSelector = () => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  closeGroupSelector()
  if (key.group_id === newGroupId || (!key.group_id && newGroupId === null)) return

  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroup(key.id, newGroupId)
    // Update local data
    const idx = apiKeys.value.findIndex((k) => k.id === key.id)
    if (idx !== -1) {
      apiKeys.value[idx] = result.api_key
    }
    if (result.auto_granted_group_access && result.granted_group_name) {
      appStore.showSuccess(t('admin.users.groupChangedWithGrant', { group: result.granted_group_name }))
    } else {
      appStore.showSuccess(t('admin.users.groupChangedSuccess'))
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupChangeFailed'))
  } finally {
    updatingKeyIds.value.delete(key.id)
  }
}

const handleKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && groupSelectorKeyId.value !== null) {
    event.stopPropagation()
    closeGroupSelector()
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (dropdownRef.value && !dropdownRef.value.contains(target)) {
    // Check if the click is on one of the group trigger buttons
    for (const el of groupButtonRefs.value.values()) {
      if (el.contains(target)) return
    }
    closeGroupSelector()
  }
}

const handleClose = () => {
  closeGroupSelector()
  // Drop the plaintext key from component state on close (not just next open)
  // to minimize how long it lingers in memory.
  createdKey.value = null
  showCreateForm.value = false
  resetCreateForm()
  emit('close')
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeyDown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeyDown, true)
})
</script>
