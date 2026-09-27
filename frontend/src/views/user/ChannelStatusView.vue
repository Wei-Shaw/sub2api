<template>
  <ChannelStatusV1View v-if="isV1" />
  <ChannelStatusV2View v-else />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { isChannelMonitorV1Mode } from '@/utils/featureFlags'

// 只加载当前模式的视图（V2 会带上 chart.js 等较大的依赖）
const ChannelStatusV1View = defineAsyncComponent(() => import('./ChannelStatusV1View.vue'))
const ChannelStatusV2View = defineAsyncComponent(() => import('./ChannelStatusV2View.vue'))

const isV1 = computed(() => isChannelMonitorV1Mode())
</script>
