<template>
  <div class="relative" ref="containerRef">
    <span
      :class="[
        'inline-flex items-center gap-1 rounded-md px-1.5 py-px text-[10px] font-medium leading-tight cursor-pointer',
        badgeClass
      ]"
      @mouseenter="handleMouseEnter"
      @mouseleave="handleMouseLeave"
    >
      <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
      </svg>
      <!-- Dual dimension display -->
      <template v-if="showUserDimension && showClientDimension">
        <span class="font-mono">U{{ userCount }}/{{ userLimitDisplay }}</span>
        <span class="text-gray-400 dark:text-gray-500 mx-px">|</span>
        <span class="font-mono">C{{ clientCount }}/{{ clientLimitDisplay }}</span>
      </template>
      <!-- Single dimension: user only -->
      <template v-else-if="showUserDimension">
        <span class="font-mono">{{ userCount }}</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ userLimitDisplay }}</span>
      </template>
      <!-- Single dimension: client only (original) -->
      <template v-else>
        <span class="font-mono">{{ clientCount }}</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ clientLimitDisplay }}</span>
      </template>
    </span>

    <!-- Popover -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0 scale-95"
        enter-to-class="opacity-100 scale-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100 scale-100"
        leave-to-class="opacity-0 scale-95"
      >
        <div
          v-if="showPopover"
          class="fixed z-50 w-80 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
          :style="popoverStyle"
          @mouseenter="handlePopoverEnter"
          @mouseleave="handlePopoverLeave"
        >
          <!-- Header -->
          <div class="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-dark-700">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.affinityDetailTitle') }}
            </span>
            <span v-if="loading" class="text-xs text-gray-400">
              <svg class="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            </span>
          </div>

          <!-- Summary -->
          <div v-if="details" class="flex items-center gap-3 border-b border-gray-100 px-3 py-1.5 dark:border-dark-700 text-[10px] text-gray-500 dark:text-gray-400">
            <span v-if="showUserDimension">{{ t('admin.accounts.affinityUsers', { count: details.total_users }) }}</span>
            <span>{{ t('admin.accounts.affinityClients', { count: details.total_clients }) }}</span>
          </div>

          <!-- User groups tree -->
          <div class="max-h-60 overflow-y-auto">
            <div v-if="loading && !details" class="px-3 py-4 text-center text-xs text-gray-400">
              {{ t('common.loading') }}...
            </div>
            <div v-else-if="!details || details.users.length === 0" class="px-3 py-4 text-center text-xs text-gray-400">
              {{ t('admin.accounts.affinityNoUsers') }}
            </div>
            <div v-else class="divide-y divide-gray-50 dark:divide-dark-700">
              <div v-for="userGroup in details.users" :key="userGroup.user_id" class="py-1">
                <!-- User row -->
                <div class="flex items-center justify-between px-3 py-1">
                  <div class="flex items-center gap-1.5">
                    <span v-if="userGroup.is_pinned" class="text-[10px]" :title="t('admin.accounts.affinityPinnedLabel')">P</span>
                    <span class="text-xs font-medium text-gray-700 dark:text-gray-300 truncate" :title="userGroup.user_email">
                      {{ userGroup.user_email || `User #${userGroup.user_id}` }}
                    </span>
                  </div>
                  <span class="text-[10px] text-gray-400 dark:text-gray-500 shrink-0">
                    {{ t('admin.accounts.affinityClientCountLabel', { count: userGroup.client_count }) }}
                  </span>
                </div>
                <!-- Client rows under user -->
                <div v-for="(client, idx) in userGroup.clients" :key="idx" class="flex items-center justify-between px-3 pl-7 py-0.5">
                  <span class="font-mono text-[10px] text-gray-500 dark:text-gray-400 truncate mr-2" :title="client.client_id">
                    {{ client.client_id }}
                  </span>
                  <span class="text-[10px] text-gray-400 dark:text-gray-500 whitespace-nowrap shrink-0">
                    {{ formatRelativeTime(client.last_active) }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAffinityDetails } from '@/api/admin/accounts'
import type { AffinityDetailsResponse } from '@/types'

interface Props {
  accountId: number
  clientCount: number
  userCount: number
  base: number    // 0 = not configured
  buffer: number | null // null = infinite yellow
  userBase: number      // 0 = not configured
  userBuffer: number | null
}

const props = withDefaults(defineProps<Props>(), {
  userCount: 0,
  userBase: 0,
  userBuffer: null
})
const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const showPopover = ref(false)
const loading = ref(false)
const details = ref<AffinityDetailsResponse | null>(null)
let loaded = false
let hideTimer: ReturnType<typeof setTimeout> | null = null
let showTimer: ReturnType<typeof setTimeout> | null = null

// Dimension visibility
const showClientDimension = computed(() => props.base > 0 || props.clientCount > 0)
const showUserDimension = computed(() => props.userBase > 0 || props.userCount > 0)

// Client limit display
const clientLimitDisplay = computed(() => {
  if (props.base <= 0) return '\u221E' // infinity
  if (props.buffer === null) return `${props.base}+`
  if (props.buffer === 0) return `${props.base}`
  return `${props.base + props.buffer}`
})

// User limit display
const userLimitDisplay = computed(() => {
  if (props.userBase <= 0) return '\u221E'
  if (props.userBuffer === null) return `${props.userBase}+`
  if (props.userBuffer === 0) return `${props.userBase}`
  return `${props.userBase + props.userBuffer}`
})

// Zone calculation for a dimension
function calcZone(count: number, base: number, buffer: number | null): number {
  if (base <= 0) return 0 // no limit = green
  if (count <= base) return 0 // green
  if (buffer === null) return 1 // infinite yellow
  if (buffer === 0) return 2 // no yellow, red
  if (count <= base + buffer) return 1 // yellow
  return 2 // red
}

const clientZone = computed(() => calcZone(props.clientCount, props.base, props.buffer))
const userZone = computed(() => calcZone(props.userCount, props.userBase, props.userBuffer))
const overallZone = computed(() => Math.max(userZone.value, clientZone.value))

const badgeClass = computed(() => {
  const maxCount = Math.max(props.clientCount, props.userCount)
  if (maxCount <= 0) return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  switch (overallZone.value) {
    case 2: return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    case 1: return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
    default: return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
  }
})

const popoverStyle = computed(() => {
  if (!containerRef.value) return {}
  const rect = containerRef.value.getBoundingClientRect()
  const viewportHeight = window.innerHeight
  const viewportWidth = window.innerWidth

  let top = rect.bottom + 6
  let left = rect.left - 40

  if (top + 280 > viewportHeight) {
    top = Math.max(8, rect.top - 280)
  }
  if (left + 320 > viewportWidth) {
    left = Math.max(8, viewportWidth - 328)
  }
  if (left < 8) left = 8

  return { top: `${top}px`, left: `${left}px` }
})

function clearTimers() {
  if (hideTimer) { clearTimeout(hideTimer); hideTimer = null }
  if (showTimer) { clearTimeout(showTimer); showTimer = null }
}

function handleMouseEnter() {
  clearTimers()
  showTimer = setTimeout(() => {
    showPopover.value = true
    if (!loaded) fetchDetails()
  }, 200)
}

function handleMouseLeave() {
  clearTimers()
  hideTimer = setTimeout(() => { showPopover.value = false }, 150)
}

function handlePopoverEnter() {
  clearTimers()
}

function handlePopoverLeave() {
  clearTimers()
  hideTimer = setTimeout(() => { showPopover.value = false }, 150)
}

async function fetchDetails() {
  loading.value = true
  try {
    details.value = await getAffinityDetails(props.accountId)
    loaded = true
  } catch {
    details.value = null
  } finally {
    loading.value = false
  }
}

function formatRelativeTime(isoStr: string): string {
  const now = Date.now()
  const then = new Date(isoStr).getTime()
  const diffSec = Math.floor((now - then) / 1000)

  if (diffSec < 60) return t('common.justNow', 'just now')
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`
  return `${Math.floor(diffSec / 86400)}d ago`
}
</script>
