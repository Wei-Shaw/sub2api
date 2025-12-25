<template>
  <div class="inline-flex items-center rounded-md overflow-hidden text-xs font-medium">
    <!-- Platform part -->
    <span
      :class="[
        'inline-flex items-center gap-1 px-2 py-1',
        platformClass
      ]"
    >
      <PlatformIcon :platform="platform" size="xs" />
      <span>{{ platformLabel }}</span>
    </span>
    <!-- Type part -->
    <span
      :class="[
        'inline-flex items-center gap-1 px-1.5 py-1',
        typeClass
      ]"
    >
      <!-- OAuth icon -->
      <svg v-if="type === 'oauth'" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
      </svg>
      <!-- Setup Token icon -->
      <svg v-else-if="type === 'setup-token'" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
      </svg>
      <!-- API Key icon -->
      <svg v-else class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
      </svg>
      <span>{{ typeLabel }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { AccountPlatform, AccountType } from '@/types'
import PlatformIcon from './PlatformIcon.vue'

interface Props {
  platform: AccountPlatform
  type: AccountType
}

const props = defineProps<Props>()

const platformLabel = computed(() => {
  return props.platform === 'anthropic' ? 'Anthropic' : 'OpenAI'
})

const typeLabel = computed(() => {
  switch (props.type) {
    case 'oauth': return 'OAuth'
    case 'setup-token': return 'Token'
    case 'apikey': return 'Key'
    default: return props.type
  }
})

const platformClass = computed(() => {
  if (props.platform === 'anthropic') {
    return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
  }
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})

const typeClass = computed(() => {
  if (props.platform === 'anthropic') {
    return 'bg-orange-100 text-orange-600 dark:bg-orange-900/30 dark:text-orange-400'
  }
  return 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400'
})
</script>
