<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { list as listUsers } from '@/api/admin/users'
import type { AdminUser } from '@/types'

const { t } = useI18n()

const props = defineProps<{
  enabled: boolean
  base: number | null
  buffer: number | null // null = infinite yellow zone
  // New v2 props
  allowSwitch: boolean
  userBase: number | null
  userBuffer: number | null
  perUserLimit: number | null
  pinnedUsers: number[]
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:base': [value: number | null]
  'update:buffer': [value: number | null]
  'update:allowSwitch': [value: boolean]
  'update:userBase': [value: number | null]
  'update:userBuffer': [value: number | null]
  'update:perUserLimit': [value: number | null]
  'update:pinnedUsers': [value: number[]]
}>()

const localEnabled = ref(props.enabled)

watch(() => props.enabled, (val) => {
  localEnabled.value = val
})

watch(localEnabled, (val) => {
  emit('update:enabled', val)
  if (!val) {
    emit('update:base', null)
    emit('update:buffer', null)
    emit('update:userBase', null)
    emit('update:userBuffer', null)
    emit('update:perUserLimit', null)
    emit('update:pinnedUsers', [])
  }
})

// ====== Allow Switch ======
const localAllowSwitch = ref(props.allowSwitch)
watch(() => props.allowSwitch, (val) => { localAllowSwitch.value = val })
watch(localAllowSwitch, (val) => { emit('update:allowSwitch', val) })

// ====== User Affinity ======
const userLimitEnabled = ref(props.userBase != null && props.userBase > 0)
watch(() => props.userBase, (val) => { userLimitEnabled.value = val != null && val > 0 })

const toggleUserLimit = () => {
  userLimitEnabled.value = !userLimitEnabled.value
  if (userLimitEnabled.value) {
    emit('update:userBase', 5)
  } else {
    emit('update:userBase', null)
    emit('update:userBuffer', null)
    emit('update:perUserLimit', null)
  }
}

const onUserBaseInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:userBase', Number.isNaN(raw) ? null : Math.max(1, Math.floor(raw)))
}

const userBufferIsInfinite = ref(props.userBuffer === null || props.userBuffer === undefined)
watch(() => props.userBuffer, (val) => { userBufferIsInfinite.value = val === null || val === undefined })

const toggleUserBufferInfinite = () => {
  userBufferIsInfinite.value = !userBufferIsInfinite.value
  emit('update:userBuffer', userBufferIsInfinite.value ? null : 3)
}

const onUserBufferInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:userBuffer', Number.isNaN(raw) ? null : Math.max(0, Math.floor(raw)))
}

const userZonePreview = computed(() => buildZonePreview(props.userBase, props.userBuffer))

// ====== Client Affinity (existing) ======
const baseLimitEnabled = ref(props.base != null && props.base > 0)
watch(() => props.base, (val) => { baseLimitEnabled.value = val != null && val > 0 })

const toggleBaseLimit = () => {
  baseLimitEnabled.value = !baseLimitEnabled.value
  if (baseLimitEnabled.value) {
    emit('update:base', 5)
  } else {
    emit('update:base', null)
    emit('update:buffer', null)
  }
}

const onBaseInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:base', Number.isNaN(raw) ? null : Math.max(1, Math.floor(raw)))
}

const bufferIsInfinite = ref(props.buffer === null || props.buffer === undefined)
watch(() => props.buffer, (val) => { bufferIsInfinite.value = val === null || val === undefined })

const toggleBufferInfinite = () => {
  bufferIsInfinite.value = !bufferIsInfinite.value
  emit('update:buffer', bufferIsInfinite.value ? null : 3)
}

const onBufferInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:buffer', Number.isNaN(raw) ? null : Math.max(0, Math.floor(raw)))
}

const clientZonePreview = computed(() => buildZonePreview(props.base, props.buffer))

// ====== Per-user Client Limit ======
const perUserEnabled = ref(props.perUserLimit != null && props.perUserLimit > 0)
watch(() => props.perUserLimit, (val) => { perUserEnabled.value = val != null && val > 0 })

const togglePerUserLimit = () => {
  perUserEnabled.value = !perUserEnabled.value
  emit('update:perUserLimit', perUserEnabled.value ? 3 : null)
}

const onPerUserLimitInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:perUserLimit', Number.isNaN(raw) ? null : Math.max(1, Math.floor(raw)))
}

// ====== Pinned Users ======
const searchQuery = ref('')
const searchResults = ref<AdminUser[]>([])
const searching = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const pinnedUserDisplay = computed(() => {
  return props.pinnedUsers.map(id => ({ id }))
})

const searchUsers = () => {
  if (searchTimer) clearTimeout(searchTimer)
  if (!searchQuery.value.trim()) {
    searchResults.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    searching.value = true
    try {
      const resp = await listUsers(1, 10, { search: searchQuery.value.trim() })
      searchResults.value = resp.items.filter(
        u => !props.pinnedUsers.includes(u.id)
      )
    } catch {
      searchResults.value = []
    } finally {
      searching.value = false
    }
  }, 300)
}

const addPinnedUser = (user: AdminUser) => {
  if (!props.pinnedUsers.includes(user.id)) {
    emit('update:pinnedUsers', [...props.pinnedUsers, user.id])
  }
  searchQuery.value = ''
  searchResults.value = []
}

const removePinnedUser = (userId: number) => {
  emit('update:pinnedUsers', props.pinnedUsers.filter(id => id !== userId))
}

// ====== Shared helper ======
function buildZonePreview(base: number | null, buffer: number | null) {
  const b = base ?? 0
  if (b <= 0) return null
  if (buffer === null || buffer === undefined) {
    return { green: `1~${b}`, yellow: `${b + 1}+`, red: null }
  }
  if (buffer === 0) {
    return { green: `1~${b}`, yellow: null, red: `${b + 1}+` }
  }
  const yellowMax = b + buffer
  return { green: `1~${b}`, yellow: `${b + 1}~${yellowMax}`, red: `${yellowMax + 1}+` }
}
</script>

<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
    <div class="mb-4">
      <label class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.affinityConfigTitle') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.affinityConfigHint') }}
      </p>
    </div>

    <!-- Section 1: Basic Config -->
    <div class="flex items-center justify-between" :class="{ 'mb-3': localEnabled }">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.affinityToggle') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.affinityToggleHint') }}
        </p>
      </div>
      <button
        type="button"
        @click="localEnabled = !localEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          localEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            localEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="localEnabled" class="space-y-4">
      <!-- Allow Switch toggle -->
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.affinityAllowSwitch') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.affinityAllowSwitchHint') }}
          </p>
        </div>
        <button
          type="button"
          @click="localAllowSwitch = !localAllowSwitch"
          :class="[
            'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
            localAllowSwitch ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              localAllowSwitch ? 'translate-x-4' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <p v-if="!localAllowSwitch" class="text-xs text-amber-600 dark:text-amber-400">
        {{ t('admin.accounts.affinityAllowSwitchWarning') }}
      </p>

      <!-- Section 2: User Affinity -->
      <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
        <div class="flex items-center justify-between mb-2">
          <div>
            <label class="input-label mb-0 text-sm font-medium">{{ t('admin.accounts.affinityUserSection') }}</label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.affinityUserSectionHint') }}
            </p>
          </div>
          <button
            type="button"
            @click="toggleUserLimit"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
              userLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                userLimitEnabled ? 'translate-x-4' : 'translate-x-0'
              ]"
            />
          </button>
        </div>

        <div v-if="userLimitEnabled" class="space-y-2">
          <!-- User green zone -->
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.affinityUserBase') }}</label>
            <input
              :value="userBase"
              @input="onUserBaseInput"
              type="number"
              min="1"
              step="1"
              class="input"
              :placeholder="t('admin.accounts.affinityBasePlaceholder')"
            />
            <p class="input-hint">{{ t('admin.accounts.affinityUserBaseHint') }}</p>
          </div>

          <!-- User yellow zone -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="input-label mb-0">{{ t('admin.accounts.affinityUserBuffer') }}</label>
              <label class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer">
                <input
                  type="checkbox"
                  :checked="userBufferIsInfinite"
                  @change="toggleUserBufferInfinite"
                  class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600"
                />
                {{ t('admin.accounts.affinityBufferInfinite') }}
              </label>
            </div>
            <input
              v-if="!userBufferIsInfinite"
              :value="userBuffer"
              @input="onUserBufferInput"
              type="number"
              min="0"
              step="1"
              class="input"
              :placeholder="t('admin.accounts.affinityBufferPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.accounts.affinityUserBufferHint') }}</p>
          </div>

          <!-- User zone preview -->
          <div v-if="userZonePreview" class="flex items-center gap-2 text-xs">
            <span class="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
              {{ userZonePreview.green }}
            </span>
            <span v-if="userZonePreview.yellow" class="inline-flex items-center gap-1 rounded-full bg-yellow-100 px-2 py-0.5 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">
              {{ userZonePreview.yellow }}
            </span>
            <span v-if="userZonePreview.red" class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-red-700 dark:bg-red-900/30 dark:text-red-400">
              {{ userZonePreview.red }}
            </span>
          </div>

          <div class="border-t border-gray-100 pt-3 dark:border-dark-700">
            <div class="flex items-center justify-between mb-2">
              <div>
                <label class="input-label mb-0 text-sm font-medium">{{ t('admin.accounts.affinityPerUserLimit') }}</label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.affinityPerUserLimitHint') }}
                </p>
              </div>
              <button
                type="button"
                @click="togglePerUserLimit"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  perUserEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    perUserEnabled ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
            <div v-if="perUserEnabled">
              <label class="input-label mb-0">{{ t('admin.accounts.affinityPerUserMax') }}</label>
              <input
                :value="perUserLimit"
                @input="onPerUserLimitInput"
                type="number"
                min="1"
                step="1"
                class="input"
              />
            </div>
          </div>

          <div class="border-t border-gray-100 pt-3 dark:border-dark-700">
            <div class="mb-2">
              <label class="input-label mb-0 text-sm font-medium">{{ t('admin.accounts.affinityPinnedUsers') }}</label>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.affinityPinnedUsersHint') }}
              </p>
            </div>

            <div class="relative">
              <input
                v-model="searchQuery"
                @input="searchUsers"
                type="text"
                class="input"
                :placeholder="t('admin.accounts.affinityPinnedUsersSearch')"
              />
              <div
                v-if="searchResults.length > 0"
                class="absolute z-10 mt-1 w-full rounded-md border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800 max-h-40 overflow-y-auto"
              >
                <div
                  v-for="user in searchResults"
                  :key="user.id"
                  @click="addPinnedUser(user)"
                  class="cursor-pointer px-3 py-1.5 text-xs hover:bg-gray-50 dark:hover:bg-dark-700 flex items-center justify-between"
                >
                  <span class="text-gray-700 dark:text-gray-300 truncate">{{ user.email || user.username }}</span>
                  <span class="text-gray-400 dark:text-gray-500 ml-2 shrink-0">#{{ user.id }}</span>
                </div>
              </div>
              <div v-if="searching" class="absolute right-2 top-1/2 -translate-y-1/2">
                <svg class="h-4 w-4 animate-spin text-gray-400" viewBox="0 0 24 24" fill="none">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
              </div>
            </div>

            <div v-if="pinnedUserDisplay.length > 0" class="mt-2 space-y-1">
              <div
                v-for="pu in pinnedUserDisplay"
                :key="pu.id"
                class="flex items-center justify-between rounded bg-gray-50 px-2 py-1 dark:bg-dark-700"
              >
                <span class="font-mono text-xs text-gray-700 dark:text-gray-300">User #{{ pu.id }}</span>
                <button
                  type="button"
                  @click="removePinnedUser(pu.id)"
                  class="text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400"
                >
                  <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>
            <p v-else class="mt-2 text-xs text-gray-400 dark:text-gray-500">
              {{ t('admin.accounts.affinityPinnedUsersEmpty') }}
            </p>
          </div>
        </div>
      </div>

      <!-- Section 3: Client Affinity -->
      <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
        <div class="mb-2">
          <label class="input-label mb-0 text-sm font-medium">{{ t('admin.accounts.affinityClientSection') }}</label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.affinityClientSectionHint') }}
          </p>
        </div>

        <div class="space-y-2">
          <!-- Client green zone toggle + input -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="input-label mb-0">{{ t('admin.accounts.affinityBase') }}</label>
              <button
                type="button"
                @click="toggleBaseLimit"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  baseLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    baseLimitEnabled ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
            <input
              v-if="baseLimitEnabled"
              :value="base"
              @input="onBaseInput"
              type="number"
              min="1"
              step="1"
              class="input"
              :placeholder="t('admin.accounts.affinityBasePlaceholder')"
            />
            <p class="input-hint">{{ baseLimitEnabled ? t('admin.accounts.affinityBaseHint') : t('admin.accounts.affinityBaseOffHint') }}</p>
          </div>

          <!-- Client buffer (yellow zone) -->
          <div v-if="baseLimitEnabled">
            <div class="flex items-center justify-between mb-1">
              <label class="input-label mb-0">{{ t('admin.accounts.affinityBuffer') }}</label>
              <label class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer">
                <input
                  type="checkbox"
                  :checked="bufferIsInfinite"
                  @change="toggleBufferInfinite"
                  class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600"
                />
                {{ t('admin.accounts.affinityBufferInfinite') }}
              </label>
            </div>
            <input
              v-if="!bufferIsInfinite"
              :value="buffer"
              @input="onBufferInput"
              type="number"
              min="0"
              step="1"
              class="input"
              :placeholder="t('admin.accounts.affinityBufferPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.accounts.affinityBufferHint') }}</p>
          </div>

          <!-- Client zone preview -->
          <div v-if="clientZonePreview" class="flex items-center gap-2 text-xs">
            <span class="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
              {{ clientZonePreview.green }}
            </span>
            <span v-if="clientZonePreview.yellow" class="inline-flex items-center gap-1 rounded-full bg-yellow-100 px-2 py-0.5 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">
              {{ clientZonePreview.yellow }}
            </span>
            <span v-if="clientZonePreview.red" class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-red-700 dark:bg-red-900/30 dark:text-red-400">
              {{ clientZonePreview.red }}
            </span>
          </div>
        </div>
      </div>

    </div>
  </div>
</template>
