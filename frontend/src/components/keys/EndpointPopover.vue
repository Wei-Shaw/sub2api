<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import type { CustomEndpoint } from '@/types'

const props = defineProps<{
  apiBaseUrl: string
  customEndpoints: CustomEndpoint[]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copiedEndpoint = ref<string | null>(null)

let copiedResetTimer: number | undefined

const allEndpoints = computed(() => {
  const items: Array<{ name: string; endpoint: string; description: string; isDefault: boolean }> = []
  if (props.apiBaseUrl) {
    items.push({
      name: t('keys.endpoints.title'),
      endpoint: props.apiBaseUrl,
      description: '',
      isDefault: true,
    })
  }
  for (const ep of props.customEndpoints) {
    items.push({ ...ep, isDefault: false })
  }
  return items
})

async function copy(url: string) {
  const success = await copyToClipboard(url, t('keys.endpoints.copied'))
  if (!success) return

  copiedEndpoint.value = url
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = window.setTimeout(() => {
    if (copiedEndpoint.value === url) {
      copiedEndpoint.value = null
    }
  }, 1800)
}

function tooltipHint(endpoint: string): string {
  return copiedEndpoint.value === endpoint
    ? t('keys.endpoints.copiedHint')
    : t('keys.endpoints.clickToCopy')
}

function speedTestUrl(endpoint: string): string {
  return `https://www.tcptest.cn/http/${encodeURIComponent(endpoint)}`
}

onBeforeUnmount(() => {
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
})
</script>

<template>
  <div v-if="allEndpoints.length > 0" class="flex flex-wrap gap-2">
    <div
      v-for="(item, index) in allEndpoints"
      :key="index"
      class="flex min-h-[36px] min-w-0 max-w-full flex-wrap items-center gap-1.5 rounded-sm border border-border bg-surface px-2.5 py-1.5 text-meta transition-colors hover:border-accent"
    >
      <span class="font-semibold text-fg">{{ item.name }}</span>
      <span
        v-if="item.isDefault"
        class="badge badge-primary px-1 py-0 leading-tight"
      >{{ t('keys.endpoints.default') }}</span>

      <span class="text-border-strong" aria-hidden="true">|</span>

      <div class="group/endpoint relative flex min-w-0 items-center gap-1.5">
        <div
          class="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 w-max max-w-[min(24rem,calc(100vw-2rem))] -translate-x-1/2 translate-y-1 rounded-sm border border-border-strong bg-surface-raised px-3 py-2.5 text-left opacity-0 shadow-overlay transition-opacity duration-150 group-hover/endpoint:translate-y-0 group-hover/endpoint:opacity-100 group-focus-within/endpoint:translate-y-0 group-focus-within/endpoint:opacity-100"
        >
          <p
            v-if="item.description"
            class="max-w-[24rem] break-words text-meta text-fg"
          >
            {{ item.description }}
          </p>
          <p
            class="flex items-center gap-1.5 text-meta text-accent-strong"
            :class="item.description ? 'mt-1.5' : ''"
          >
            <span class="h-1.5 w-1.5 bg-accent" aria-hidden="true"></span>
            {{ tooltipHint(item.endpoint) }}
          </p>
          <div class="absolute left-1/2 top-full h-3 w-3 -translate-x-1/2 -translate-y-1/2 rotate-45 border-b border-r border-border-strong bg-surface-raised"></div>
        </div>

        <code
          class="min-w-0 cursor-pointer break-all font-mono text-fg-muted decoration-border-strong decoration-dashed underline-offset-2 hover:text-accent-strong hover:underline focus:text-accent-strong focus:underline focus:outline-none"
          role="button"
          tabindex="0"
          @click="copy(item.endpoint)"
          @keydown.enter.prevent="copy(item.endpoint)"
          @keydown.space.prevent="copy(item.endpoint)"
        >{{ item.endpoint }}</code>

        <button
          type="button"
          class="-my-1.5 inline-flex min-h-10 min-w-10 items-center justify-center rounded-sm p-2 transition-colors"
          :class="copiedEndpoint === item.endpoint
            ? 'text-success'
            : 'text-fg-subtle hover:text-accent'"
          :aria-label="tooltipHint(item.endpoint)"
          @click="copy(item.endpoint)"
        >
          <svg v-if="copiedEndpoint === item.endpoint" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          <svg v-else class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        </button>

        <a
          :href="speedTestUrl(item.endpoint)"
          target="_blank"
          rel="noopener noreferrer"
          class="-my-1.5 inline-flex min-h-10 min-w-10 items-center justify-center rounded-sm p-2 text-fg-subtle transition-colors hover:text-accent"
          :title="t('keys.endpoints.speedTest')"
        >
          <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
        </a>
      </div>
    </div>
  </div>
</template>
