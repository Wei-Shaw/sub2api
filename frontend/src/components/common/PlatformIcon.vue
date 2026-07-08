<template>
  <span
    v-if="iconEntry?.renderAs === 'mask'"
    :class="['platform-icon-mask', sizeClass]"
    :style="maskStyle"
    aria-hidden="true"
  />
  <img
    v-else-if="iconEntry"
    :src="iconEntry.src"
    alt=""
    :class="['platform-icon-img', sizeClass]"
    aria-hidden="true"
  />
  <svg v-else :class="sizeClass" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
    <path
      d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { GroupPlatform } from '@/types'
import anthropicIcon from '@/assets/platform-icons/anthropic.svg'
import openaiIcon from '@/assets/platform-icons/openai.svg'
import geminiIcon from '@/assets/platform-icons/gemini.svg'
import antigravityIcon from '@/assets/platform-icons/antigravity.png'
import grokIcon from '@/assets/platform-icons/grok.svg'
import compositeIcon from '@/assets/platform-icons/composite.svg'
import kiroIcon from '@/assets/platform-icons/kiro.svg'
import deepseekIcon from '@/assets/platform-icons/deepseek.svg'
import glmIcon from '@/assets/platform-icons/glm.svg'
import zaiIcon from '@/assets/platform-icons/zai.svg'
import minimaxIcon from '@/assets/platform-icons/minimax.svg'
import opencodeIcon from '@/assets/platform-icons/opencode.svg'

interface Props {
  platform?: GroupPlatform
  size?: 'xs' | 'sm' | 'md' | 'lg'
}

interface PlatformIconEntry {
  src: string
  renderAs: 'mask' | 'image'
}

const props = withDefaults(defineProps<Props>(), {
  size: 'sm'
})

const platformIcons = {
  anthropic: { src: anthropicIcon, renderAs: 'mask' },
  openai: { src: openaiIcon, renderAs: 'mask' },
  gemini: { src: geminiIcon, renderAs: 'image' },
  antigravity: { src: antigravityIcon, renderAs: 'image' },
  grok: { src: grokIcon, renderAs: 'mask' },
  composite: { src: compositeIcon, renderAs: 'mask' },
  kiro: { src: kiroIcon, renderAs: 'image' },
  deepseek: { src: deepseekIcon, renderAs: 'mask' },
  glm: { src: glmIcon, renderAs: 'mask' },
  zai: { src: zaiIcon, renderAs: 'image' },
  minimax: { src: minimaxIcon, renderAs: 'mask' },
  opencode: { src: opencodeIcon, renderAs: 'mask' }
} satisfies Record<GroupPlatform, PlatformIconEntry>

const iconEntry = computed(() => (props.platform ? platformIcons[props.platform] : undefined))

const maskStyle = computed<Record<string, string>>(() => {
  if (!iconEntry.value || iconEntry.value.renderAs !== 'mask') return {} as Record<string, string>
  return {
    '--platform-icon-url': `url("${iconEntry.value.src}")`
  }
})

const sizeClass = computed(() => {
  const sizes = {
    xs: 'w-3 h-3',
    sm: 'w-3.5 h-3.5',
    md: 'w-4 h-4',
    lg: 'w-5 h-5'
  }
  return sizes[props.size] + ' flex-shrink-0'
})
</script>

<style scoped>
.platform-icon-mask {
  display: inline-block;
  background-color: currentColor;
  mask: var(--platform-icon-url) center / contain no-repeat;
  -webkit-mask: var(--platform-icon-url) center / contain no-repeat;
}

.platform-icon-img {
  display: inline-block;
  object-fit: contain;
}
</style>