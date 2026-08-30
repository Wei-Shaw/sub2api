<template>
  <img
    v-if="safeLogoUrl && !imageFailed"
    :src="safeLogoUrl"
    :alt="alt"
    :class="sizeClass"
    class="shrink-0 object-contain"
    @error="imageFailed = true"
  />
  <PlatformIcon v-else :platform="fallbackPlatform" :size="fallbackIconSize" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const props = withDefaults(
  defineProps<{
    provider: string
    logoKey?: string | null
    logoUrl?: string | null
    alt?: string
    size?: 'sm' | 'md' | 'lg' | 'xl'
  }>(),
  {
    logoKey: '',
    logoUrl: '',
    alt: '',
    size: 'md'
  }
)

const imageFailed = ref(false)

const safeLogoUrl = computed(() => sanitizeProviderLogoUrl(props.logoUrl))
const fallbackPlatform = computed(() => {
  const key = (props.logoKey || props.provider || 'composite').trim().toLowerCase()
  return key === 'moonshot' ? 'kimi' : key
})
const fallbackIconSize = computed<'sm' | 'md' | 'lg'>(() => {
  if (props.size === 'sm') return 'sm'
  if (props.size === 'lg' || props.size === 'xl') return 'lg'
  return 'md'
})
const sizeClass = computed(() => {
  if (props.size === 'sm') return 'h-4 w-4'
  if (props.size === 'lg') return 'h-8 w-8'
  if (props.size === 'xl') return 'h-10 w-10'
  return 'h-5 w-5'
})

watch(() => props.logoUrl, () => {
  imageFailed.value = false
})

function sanitizeProviderLogoUrl(value: string | null | undefined): string {
  const trimmed = value?.trim() ?? ''
  if (!trimmed || trimmed.startsWith('//')) return ''

  try {
    const windowOrigin = typeof window === 'undefined' ? '' : window.location.origin
    const base = windowOrigin && windowOrigin !== 'null' ? windowOrigin : 'https://sub2api.invalid'
    const parsed = new URL(trimmed, `${base}/`)
    const isExplicitAbsolute = /^[a-z][a-z\d+.-]*:/i.test(trimmed)
    if (isExplicitAbsolute) return parsed.protocol === 'https:' ? parsed.href : ''
    return parsed.origin === base ? `${parsed.pathname}${parsed.search}${parsed.hash}` : ''
  } catch {
    return ''
  }
}
</script>
