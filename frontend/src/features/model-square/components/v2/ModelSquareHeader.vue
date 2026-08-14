<template>
  <div class='mb-8 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between'>
    <div class='max-w-2xl'>
      <div class='mb-3 inline-flex items-center gap-2 rounded-full border border-indigo-500/20 bg-indigo-500/5 px-3 py-1 text-[10px] font-black uppercase tracking-[0.2em] text-indigo-300 dark:border-indigo-400/20 dark:bg-indigo-400/10 dark:text-indigo-200'>
        <span class='h-1.5 w-1.5 rounded-full bg-indigo-400 animate-pulse'></span>
        Model Square
      </div>
      <h1 class='text-4xl font-black tracking-tight bg-gradient-to-r from-gray-900 via-gray-800 to-gray-600 bg-clip-text text-transparent dark:from-white dark:via-gray-100 dark:to-gray-400'>
        模型广场
      </h1>
      <p class='mt-2 text-base text-gray-500 dark:text-dark-400 leading-relaxed'>汇聚所有可用模型、渠道与分组定价，一键对比并快速选择最优接入方案。</p>
    </div>
    <div class='flex items-center gap-3'>
      <div class='relative group flex-1 lg:flex-none'>
        <div class='absolute inset-0 rounded-2xl bg-gradient-to-r from-indigo-500/20 to-purple-500/20 opacity-0 group-focus-within:opacity-100 transition-opacity duration-300 blur-xl'></div>
        <div class='relative flex items-center'>
          <Icon name='search' size='sm' class='absolute left-4 top-1/2 -translate-y-1/2 text-gray-500 group-focus-within:text-indigo-400 transition-colors' />
          <input
            :value='search'
            type='text'
            aria-label='搜索模型、渠道、平台或分组'
            class='w-full lg:w-96 h-12 bg-white/80 dark:bg-dark-900/60 border border-gray-200 dark:border-dark-700/60 rounded-2xl py-2 pl-11 pr-4 text-sm font-medium text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 outline-none focus:border-indigo-500/50 focus:bg-white dark:focus:bg-dark-800/80 transition-all shadow-lg shadow-black/5 dark:shadow-black/20 backdrop-blur'
            placeholder='搜索模型、渠道、平台或分组...'
            @input='$emit("update:search", ($event.target as HTMLInputElement).value)'
          />
        </div>
      </div>
      <button
        type='button'
        class='flex h-12 w-12 items-center justify-center rounded-2xl bg-white/80 dark:bg-dark-900/60 border border-gray-200 dark:border-dark-700/60 hover:bg-white dark:hover:bg-dark-800/80 transition-all shadow-lg shadow-black/5 dark:shadow-black/20 active:scale-95 backdrop-blur'
        :aria-label='isDark ? "切换到亮色模式" : "切换到暗色模式"'
        @click='$emit("toggle-dark")'
      >
        <Icon v-if='isDark' name='sun' size='sm' class='text-amber-500' />
        <Icon v-else name='moon' size='sm' class='text-indigo-500' />
      </button>
      <button
        type='button'
        class='flex h-12 w-12 items-center justify-center rounded-2xl bg-white/80 dark:bg-dark-900/60 border border-gray-200 dark:border-dark-700/60 hover:bg-white dark:hover:bg-dark-800/80 transition-all shadow-lg shadow-black/5 dark:shadow-black/20 active:scale-95 backdrop-blur'
        :disabled='loading'
        aria-label='刷新'
        @click='$emit("refresh")'
      >
        <Icon name='refresh' size='sm' :class='iconClass' />
      </button>
    </div>
  </div>
</template>

<script setup lang='ts'>
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  search: string
  loading: boolean
  isDark: boolean
}

const props = defineProps<Props>()

defineEmits<{
  'update:search': [value: string]
  refresh: []
  'toggle-dark': []
}>()

const iconClass = computed(() => (props.loading ? 'animate-spin' : ''))
</script>
