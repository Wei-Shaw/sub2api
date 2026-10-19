<!--
  脚本卡片列表：一次执行产出的每一张脚本，可以展开改提示词。

  为什么抽成组件：主人要求「爆款复刻、套图这些功能里，本功能结果图的上方
  要有这次执行生成的每一个脚本」。同一套卡片现在要出现在两个面板里，
  复制两遍的话以后改一处必漏另一处。

  组件只负责画和抛事件，不改数据：
  - 编辑框用 `:value` + `@input` 抛出去，由父组件写回卡片
    （不在这里直接改 props，避免「谁改的」查不清）；
  - 展开哪一张由父组件用 `openKey` 控制，所以**同一时间只会展开一张**。
-->
<template>
  <div>
    <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('aiImage.script.built', { count: cards.length }) }}
      </span>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-if="cards.some((card) => card.edited)"
          type="button"
          class="btn btn-secondary btn-sm"
          data-testid="script-reset-edits"
          @click="emit('reset-edits')"
        >
          {{ t('aiImage.script.resetEdits') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" @click="emit('apply-all')">
          {{ t('aiImage.script.applyAll') }}
        </button>
      </div>
    </div>

    <!--
      ⚠️ `items-start` 不能删。
      默认 grid 的子项会被拉成同一行里最高的那个高度：
      一张卡片展开编辑区后，同一行其它卡片会被撑成同样高，
      看上去就像「没点的卡片也冒出一个空框，而且里面没内容」。
      （主人 2026-10-17 报的就是这个现象。）
    -->
    <div class="grid grid-cols-1 items-start gap-2 sm:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="card in cards"
        :key="card.key"
        class="rounded-lg border bg-gray-50 dark:bg-dark-900"
        :class="card.edited ? 'border-amber-300 dark:border-amber-500/40' : 'border-gray-200 dark:border-dark-600'"
        :data-testid="`script-card-${card.key}`"
      >
        <button
          type="button"
          class="flex w-full items-center justify-between gap-2 px-2 py-2 text-left text-xs"
          :title="card.prompt"
          :data-testid="`script-card-toggle-${card.key}`"
          @click="emit('toggle', card.key)"
        >
          <span class="min-w-0">
            <span class="block truncate font-medium text-gray-800 dark:text-gray-100">{{ card.title }}</span>
            <span class="mt-0.5 block text-gray-400 dark:text-gray-500">
              <template v-if="card.edited">{{ t('aiImage.script.editedTag') }}</template>
              <template v-else>{{ t('aiImage.script.fillOne') }}</template>
            </span>
          </span>
          <Icon
            :name="openKey === card.key ? 'chevronUp' : 'chevronDown'"
            size="sm"
            class="flex-shrink-0 text-gray-400"
          />
        </button>

        <!-- 编辑区：改的就是这一张真正会发出去的内容 -->
        <div v-if="openKey === card.key" class="border-t border-gray-200 px-2 py-2 dark:border-dark-600">
          <textarea
            :value="card.prompt"
            rows="8"
            class="input w-full resize-y text-xs leading-relaxed"
            :data-testid="`script-card-text-${card.key}`"
            @input="emit('edit', card, ($event.target as HTMLTextAreaElement).value)"
          ></textarea>
          <div class="mt-2 flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :data-testid="`script-card-save-${card.key}`"
              @click="emit('save', card)"
            >
              {{ t('aiImage.script.saveEdit') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :data-testid="`script-card-generate-${card.key}`"
              @click="emit('generate', card)"
            >
              {{ t('aiImage.script.generateThis') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :data-testid="`script-card-fill-${card.key}`"
              @click="emit('fill', card)"
            >
              {{ t('aiImage.script.fillOne') }}
            </button>
            <button v-if="card.edited" type="button" class="btn btn-secondary btn-sm" @click="emit('restore', card)">
              {{ t('aiImage.script.restoreCard') }}
            </button>
          </div>
          <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('aiImage.script.editHint') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { OzonScriptCard } from '@/api/aiImagePrompts'

defineProps<{
  /** 这一批脚本卡片（父组件持有，这里只读）。 */
  cards: OzonScriptCard[]
  /** 当前展开的是哪一张；同一时间只会有一张。 */
  openKey: string
}>()

const emit = defineEmits<{
  toggle: [key: string]
  edit: [card: OzonScriptCard, value: string]
  save: [card: OzonScriptCard]
  generate: [card: OzonScriptCard]
  fill: [card: OzonScriptCard]
  restore: [card: OzonScriptCard]
  'reset-edits': []
  'apply-all': []
}>()

const { t } = useI18n()
</script>
