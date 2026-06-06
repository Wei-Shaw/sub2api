<template>
  <div class="relative">
    <template v-if="isAdmin">
      <button
        ref="triggerRef"
        @click="toggleDropdown"
        class="flex items-center gap-1.5 rounded-lg bg-gray-100 px-2 py-1 text-xs text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-400 dark:hover:bg-dark-700"
        :title="buttonTitle"
      >
        <span v-if="currentVersion" class="font-medium">v{{ currentVersion }}</span>
        <span
          v-else
          class="h-3 w-12 animate-pulse rounded bg-gray-200 font-medium dark:bg-dark-600"
        ></span>
      </button>

      <transition name="dropdown">
        <div
          v-if="dropdownOpen"
          ref="dropdownRef"
          class="absolute left-0 z-50 mt-2 w-64 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">
              {{ t('version.currentVersion') }}
            </span>
          </div>

          <div class="space-y-4 p-4">
            <div class="text-center">
              <div class="text-2xl font-bold text-gray-900 dark:text-white">
                <span v-if="currentVersion">v{{ currentVersion }}</span>
                <span v-else class="text-gray-400 dark:text-dark-500">--</span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('version.currentVersion') }}
              </p>
            </div>

            <button
              @click="handleRestart"
              :disabled="restarting"
              class="flex w-full items-center justify-center gap-2 rounded-lg bg-green-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-green-600 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <svg
                v-if="restarting"
                class="h-4 w-4 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
              <Icon v-else name="refresh" size="sm" :stroke-width="2" />
              <template v-if="restarting">
                <span>{{ t('version.restarting') }}</span>
                <span v-if="restartCountdown > 0" class="tabular-nums">
                  ({{ restartCountdown }}s)
                </span>
              </template>
              <span v-else>{{ t('version.restartNow') }}</span>
            </button>
          </div>
        </div>
      </transition>
    </template>

    <span v-else-if="currentVersion" class="text-xs text-gray-500 dark:text-dark-400">
      v{{ currentVersion }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores'
import { restartService } from '@/api/admin/system'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const props = defineProps<{
  version?: string
}>()

const authStore = useAuthStore()

const isAdmin = computed(() => authStore.isAdmin)
const currentVersion = computed(() => props.version || '')
const buttonTitle = computed(() =>
  currentVersion.value
    ? `${t('version.currentVersion')}: v${currentVersion.value}`
    : t('version.currentVersion')
)

const dropdownOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)

const restarting = ref(false)
const restartCountdown = ref(0)

function toggleDropdown() {
  if (!isAdmin.value) return
  dropdownOpen.value = !dropdownOpen.value
}

function closeDropdown() {
  dropdownOpen.value = false
}

async function handleRestart() {
  if (restarting.value) return

  restarting.value = true
  restartCountdown.value = 8

  try {
    await restartService()
  } catch {
    console.log('Service restarting...')
  }

  const countdownInterval = setInterval(() => {
    restartCountdown.value--
    if (restartCountdown.value <= 0) {
      clearInterval(countdownInterval)
      checkServiceAndReload()
    }
  }, 1000)
}

async function checkServiceAndReload() {
  const maxRetries = 5
  const retryDelay = 1000

  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch('/health', {
        method: 'GET',
        cache: 'no-cache'
      })
      if (response.ok) {
        window.location.reload()
        return
      }
    } catch {
      // Service not ready yet.
    }

    if (i < maxRetries - 1) {
      await new Promise((resolve) => setTimeout(resolve, retryDelay))
    }
  }

  window.location.reload()
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node
  if (dropdownRef.value?.contains(target) || triggerRef.value?.contains(target)) {
    return
  }
  closeDropdown()
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
