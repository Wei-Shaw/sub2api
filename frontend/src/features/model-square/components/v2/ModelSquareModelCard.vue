<template>
  <article
    :id='cardId'
    :data-model-key='model.key'
    class='scroll-mt-8 relative bg-white/80 dark:bg-dark-900/60 border border-gray-200 dark:border-dark-700/60 rounded-3xl shadow-2xl shadow-black/10 dark:shadow-black/20 hover:shadow-indigo-500/10 hover:-translate-y-1 transition-all duration-500 overflow-hidden backdrop-blur-xl group'
  >
    <!-- 平台渐变顶部条 -->
    <div class='h-1.5 w-full' :class='platformGradientClass(model.platform)'></div>

    <header class='px-7 py-5 flex items-start justify-between border-b border-gray-200 dark:border-dark-700/60 bg-gray-50/50 dark:bg-dark-950/20'>
      <div class='min-w-0'>
        <div class='flex items-center gap-3'>
          <div class='h-11 w-11 rounded-2xl flex items-center justify-center text-xl font-black text-white shadow-lg' :class='platformGradientClass(model.platform)'>
            {{ model.name.slice(0, 1).toUpperCase() }}
          </div>
          <div>
            <h2 class='text-3xl font-black tracking-tight text-gray-900 dark:text-white truncate leading-tight'>{{ model.name }}</h2>
            <p class='mt-0.5 text-sm font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest'>{{ platformLabel(model.platform) }} · {{ model.channels.length }} 个渠道</p>
          </div>
        </div>
      </div>
      <button
        type='button'
        class='shrink-0 inline-flex items-center gap-2 rounded-2xl border border-gray-200 dark:border-dark-700/60 bg-white/80 dark:bg-dark-800/60 px-4 py-2.5 text-base font-bold text-gray-700 dark:text-gray-300 hover:border-indigo-500/50 hover:text-indigo-600 dark:hover:text-indigo-300 hover:bg-indigo-500/10 transition-all shadow-sm'
        @click='showConfig = !showConfig'
      >
        <svg class='h-4 w-4 transition-transform duration-300' :class='showConfig ? "rotate-180" : ""' fill='none' stroke='currentColor' viewBox='0 0 24 24'><path stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='M19 9l-7 7-7-7' /></svg>
        <span>{{ showConfig ? '收起配置' : '查看配置' }}</span>
      </button>
    </header>

    <div class='divide-y divide-gray-200 dark:divide-dark-700/60'>
      <ModelSquareChannelPanel
        v-for='channel in model.channels'
        :key='channel.key'
        :channel='channel'
        :platform='model.platform'
        :user-group-rates='userGroupRates'
      />
    </div>

    <Transition
      enter-active-class='transition-all duration-400 ease-out'
      enter-from-class='opacity-0 max-h-0 -translate-y-2'
      enter-to-class='opacity-100 max-h-[1200px] translate-y-0'
      leave-active-class='transition-all duration-300 ease-in'
      leave-from-class='opacity-100 max-h-[1200px] translate-y-0'
      leave-to-class='opacity-0 max-h-0 -translate-y-2'
    >
      <div v-if='showConfig' class='overflow-hidden border-t border-gray-200 dark:border-dark-700/60 bg-gray-50/50 dark:bg-dark-950/40 px-7 py-6'>
        <div class='mb-4 flex items-center gap-2'>
          <span class='text-xs font-black uppercase tracking-[0.2em] text-gray-500 dark:text-gray-400'>模型配置</span>
          <span class='font-mono text-xs font-bold text-indigo-600 dark:text-indigo-300 bg-indigo-500/10 px-2.5 py-1 rounded-lg border border-indigo-500/20'>{{ model.key }}</span>
        </div>
        <div class='grid gap-3'>
          <div v-for='channel in model.channels' :key='channel.key' class='rounded-2xl border border-gray-200 dark:border-dark-700/60 bg-white dark:bg-dark-900/80 p-4 backdrop-blur'>
            <div class='flex items-center justify-between mb-3'>
              <span class='text-base font-bold text-gray-900 dark:text-white'>{{ channel.name }}</span>
              <span class='font-mono text-xs text-gray-500'>{{ channel.key }}</span>
            </div>
            <div class='space-y-2'>
              <div v-for='entry in channel.entries' :key='entryKey(entry)' class='flex items-center justify-between text-sm'>
                <span class='font-medium text-gray-700 dark:text-gray-300'>{{ entry.group.name }}</span>
                <span class='font-mono text-xs text-gray-500'>
                  id={{ entry.group.id }} · 倍率 {{ entry.group.rate_multiplier }}
                  <template v-if='userGroupRates[entry.group.id] != null'>
                    · 专属 <span class='text-amber-500'>{{ userGroupRates[entry.group.id] }}</span>
                  </template>
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </article>
</template>

<script setup lang='ts'>
import { computed, ref } from 'vue'
import type { ModelSquareModel } from '../../types'
import { entryKey } from '../../utils/key'
import { platformGradientClass, platformLabel } from '@/utils/platformColors'
import ModelSquareChannelPanel from './ModelSquareChannelPanel.vue'

interface Props {
  model: ModelSquareModel
  userGroupRates: Record<number, number>
}

const props = defineProps<Props>()
const showConfig = ref(false)
const cardId = computed(() => 'model-' + props.model.key)
</script>
