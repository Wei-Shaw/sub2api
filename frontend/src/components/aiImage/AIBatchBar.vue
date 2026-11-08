<!--
  批量控制条：进度 / 并发数 / 一键生成全部 / 暂停 / 停止。

  为什么抽成组件并**放进功能页里**（2026-10-19 主人要求）：
  原来它是一条常驻在页面最下面的横条，脚本卡片却在功能页里，
  主人原话是「页面的一键生成全部位置和生成的脚本位置一样，
  在本功能的结果上方，脚本的下方」——
  也就是生成按钮要挨着它要生成的那批脚本，而不是隔着一屏。

  同一套控件要出现在「爆款复刻」和「套图」两个功能页里，
  复制两遍的话以后改一处必漏另一处，所以抽出来。

  组件只画和抛事件，状态全在父组件。
-->
<template>
  <div
    class="mt-3 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-900"
    data-testid="batch-bar"
  >
    <div class="flex flex-wrap items-center gap-2 text-xs">
      <span class="font-medium text-gray-700 dark:text-gray-200" data-testid="batch-progress">
        {{ t('aiImage.batch.progress', { done: progress.done, total: progress.total || count }) }}
      </span>
      <span v-if="progress.failed" class="text-red-600 dark:text-red-400">
        {{ t('aiImage.batch.failedCount', { count: progress.failed }) }}
      </span>
      <span v-if="paused" class="font-medium text-amber-600 dark:text-amber-400">
        {{ t('aiImage.batch.paused') }}
      </span>

      <!-- 并发数：和桌面程序一样上限 3，避免被上游限流、也避免一次烧太多额度 -->
      <label class="inline-flex items-center gap-1 text-gray-500 dark:text-gray-400">
        {{ t('aiImage.batch.concurrency') }}
        <select
          class="input w-16 py-1 text-xs"
          :value="concurrency"
          :disabled="running"
          data-testid="batch-concurrency"
          @change="emit('update:concurrency', Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="n in BATCH_CONCURRENCY_OPTIONS" :key="n" :value="n">{{ n }}</option>
        </select>
      </label>

      <button
        type="button"
        class="btn btn-primary btn-sm"
        data-testid="generate-all-script-cards"
        :disabled="running"
        @click="emit('generate-all')"
      >
        <Icon
          :name="running ? 'refresh' : 'sparkles'"
          size="sm"
          class="mr-1"
          :class="running ? 'animate-spin' : ''"
        />
        {{ running ? t('aiImage.batch.running') : t('aiImage.batch.generateAll', { count }) }}
      </button>

      <div class="flex items-center gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          data-testid="batch-pause"
          :disabled="!running"
          @click="emit('pause')"
        >
          {{ paused ? t('aiImage.batch.resume') : t('aiImage.batch.pause') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          data-testid="batch-stop"
          :disabled="!running"
          @click="emit('stop')"
        >
          {{ t('aiImage.batch.stop') }}
        </button>
      </div>
      <span class="text-gray-400 dark:text-gray-500">{{ t('aiImage.batch.stopHint') }}</span>
    </div>

    <!-- 这组卡片是谁生成的：两个功能页都有同样的控件，标一下免得混起来分不清 -->
    <p v-if="source" class="mt-1 text-xs text-gray-400 dark:text-gray-500" data-testid="batch-source">
      {{ source === 'ozon' ? t('aiImage.batch.fromOzon') : t('aiImage.batch.fromHot') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

/** 并发上限 3：和桌面程序一致。 */
const BATCH_CONCURRENCY_OPTIONS = [1, 2, 3] as const

defineProps<{
  /** 这一批脚本共几张（进度条的分母兜底）。 */
  count: number
  running: boolean
  paused: boolean
  progress: { done: number; total: number; failed: number }
  concurrency: number
  /** 这批脚本的出处，只用于显示一行说明。 */
  source: 'ozon' | 'hot' | ''
}>()

const emit = defineEmits<{
  'generate-all': []
  pause: []
  stop: []
  'update:concurrency': [value: number]
}>()

const { t } = useI18n()
</script>
