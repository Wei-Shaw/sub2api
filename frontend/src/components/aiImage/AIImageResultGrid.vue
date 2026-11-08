<!--
  结果网格：段（本轮 / 历史）→ 行（每一轮）→ 卡片。

  为什么抽成组件：主人要求「每个功能页里显示这个功能自己的结果，最下面再汇总全部」，
  同一套卡片要在 6 个地方出现（单张生图 / 爆款复刻 / 套图 + 底部汇总）。
  复制 4 遍的话，以后改卡片样式必然漏掉几处。

  组件只管画和抛事件，不改数据：重试 / 复制地址 / 下载 / 加水印下载 / 删除
  都由外面（AIImageView）处理。
-->
<template>
  <div>
    <!-- 空状态。各功能页传自己的文案，所以做成 props。 -->
    <div
      v-if="!sections.length"
      class="flex min-h-[12rem] flex-col items-center justify-center gap-2 text-center"
      data-testid="result-empty"
    >
      <Icon name="sparkles" size="xl" class="text-gray-300 dark:text-dark-600" />
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyTitle }}</p>
      <p v-if="emptyHint" class="text-xs text-gray-400 dark:text-gray-500">{{ emptyHint }}</p>
    </div>

    <div v-else class="flex flex-col gap-5">
      <section v-for="section in sections" :key="section.key" class="flex flex-col gap-3">
        <h3
          v-if="section.title"
          class="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500"
          :data-testid="`result-group-${section.key}`"
        >
          {{ section.title }}
        </h3>
        <div
          v-for="row in section.rows"
          :key="`${section.key}-${row.key}`"
          class="flex flex-wrap items-stretch gap-4"
          :data-testid="`result-round-${section.key}-${row.key}`"
        >
          <div
            v-for="record in row.records"
            :key="record.id"
            class="flex max-w-[26rem] grow basis-[20rem] flex-col overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900"
          >
            <!-- 状态条 -->
            <div class="flex items-center justify-between gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
              <span class="inline-flex items-center gap-1.5 text-xs font-medium" :class="statusClass(record.status)">
                <Icon
                  :name="statusIcon(record.status)"
                  size="sm"
                  :class="record.status === 'generating' ? 'animate-spin' : ''"
                />
                {{ statusLabel(record.status) }}
              </span>
              <span class="text-xs text-gray-400 dark:text-gray-500">{{ formatTime(record.createdAt) }}</span>
            </div>

            <!-- 图片 / 失败原因 -->
            <div class="flex min-h-[16rem] items-center justify-center p-2">
              <!-- 一张卡片里有多张图时也横向排：主人明确要求不要竖着排。
                   外面这层是卡片，本来就窄，所以允许换行，避免挤成一条。 -->
              <div v-if="record.images.length" class="flex w-full flex-wrap justify-center gap-2">
                <img
                  v-for="(url, index) in record.images"
                  :key="index"
                  :src="url"
                  :alt="record.prompt"
                  class="max-h-[34rem] min-w-0 flex-1 rounded object-contain"
                  :data-testid="`result-image-${index}`"
                />
              </div>
              <div v-else-if="record.status === 'failed'" class="px-2 text-center">
                <Icon name="xCircle" size="lg" class="mx-auto text-red-400" />
                <p class="mt-2 text-xs font-medium text-red-600 dark:text-red-400">
                  {{ t('aiImage.result.failedReason') }}
                </p>
                <p class="mt-1 break-words text-xs text-gray-600 dark:text-gray-300">{{ record.error }}</p>
              </div>
              <div v-else-if="record.status === 'completed'" class="px-3 text-center">
                <Icon name="clock" size="lg" class="mx-auto text-gray-300 dark:text-dark-600" />
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('aiImage.result.imageNotRetained') }}
                </p>
              </div>
              <div v-else class="text-center">
                <Icon name="refresh" size="lg" class="mx-auto animate-spin text-gray-300 dark:text-dark-600" />
                <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('aiImage.filter.generating') }}</p>
              </div>
            </div>

            <!-- 提示词与操作 -->
            <div class="mt-auto border-t border-gray-200 px-3 py-2 dark:border-dark-700">
              <p
                v-if="record.productId"
                class="mb-1 truncate text-xs font-medium text-primary-600 dark:text-primary-400"
                :title="record.productId"
              >
                {{ t('aiImage.product.label') }}：{{ record.productId }}
              </p>
              <!-- 提示词完整显示（原来截成 2 行，看不全就得靠悬停提示）。 -->
              <p
                class="max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-xs leading-relaxed text-gray-600 dark:text-gray-300"
                data-testid="record-prompt"
              >
                {{ record.prompt }}
              </p>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <button
                  v-if="record.status === 'failed'"
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="emit('retry', record)"
                >
                  <Icon name="refresh" size="sm" class="mr-1" />
                  {{ t('aiImage.result.retry') }}
                </button>
                <template v-if="record.images.length">
                  <!-- 图片地址是给客户端用的：客户端拿它下载到 images/<商品编号>/<序号>.jpg -->
                  <button
                    v-if="isPersistableImageUrl(record.images[0])"
                    type="button"
                    class="btn btn-primary btn-sm"
                    @click="emit('copyUrl', record)"
                  >
                    <Icon name="link" size="sm" class="mr-1" />
                    {{ t('aiImage.product.copyUrl') }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" @click="emit('download', record)">
                    <Icon name="download" size="sm" class="mr-1" />
                    {{ t('aiImage.result.download') }}
                  </button>
                  <!-- 水印只在下载时叠加，成图上只有这一层水印。 -->
                  <button
                    v-if="watermarkEnabled"
                    type="button"
                    class="btn btn-secondary btn-sm"
                    :disabled="watermarking"
                    @click="emit('downloadWatermark', record)"
                  >
                    <Icon name="download" size="sm" class="mr-1" />
                    {{ t('aiImage.watermark.download') }}
                  </button>
                  <a
                    :href="record.images[0]"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="btn btn-secondary btn-sm"
                  >
                    {{ t('aiImage.result.openInNewTab') }}
                  </a>
                </template>
                <button type="button" class="btn btn-secondary btn-sm" @click="emit('remove', record)">
                  <Icon name="trash" size="sm" class="mr-1" />
                  {{ t('aiImage.result.remove') }}
                </button>
              </div>
              <p
                v-if="record.images.length && !isPersistableImageUrl(record.images[0])"
                class="mt-1 text-xs text-amber-600 dark:text-amber-400"
              >
                {{ t('aiImage.product.noUrl') }}
              </p>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { isPersistableImageUrl } from '@/api/aiImage'
import type { AIImageRecord, RecordStatus, ResultSection } from '@/api/aiImageRecord'

defineProps<{
  /** 已经分好段和行的结果；空数组就显示空状态。 */
  sections: ResultSection[]
  /** 空状态主文案（各功能页传自己的）。 */
  emptyTitle: string
  /** 空状态副文案，可以不传。 */
  emptyHint?: string
  /** 水印开关：开着才显示「带水印下载」按钮。 */
  watermarkEnabled?: boolean
  /** 正在加水印（按钮要禁用，避免连点）。 */
  watermarking?: boolean
}>()

const emit = defineEmits<{
  retry: [record: AIImageRecord]
  copyUrl: [record: AIImageRecord]
  download: [record: AIImageRecord]
  downloadWatermark: [record: AIImageRecord]
  remove: [record: AIImageRecord]
}>()

const { t } = useI18n()

function statusLabel(status: RecordStatus): string {
  if (status === 'completed') return t('aiImage.filter.completed')
  if (status === 'failed') return t('aiImage.filter.failed')
  return t('aiImage.filter.generating')
}

function statusIcon(status: RecordStatus): 'checkCircle' | 'xCircle' | 'refresh' {
  if (status === 'completed') return 'checkCircle'
  if (status === 'failed') return 'xCircle'
  return 'refresh'
}

function statusClass(status: RecordStatus): string {
  if (status === 'completed') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'failed') return 'text-red-600 dark:text-red-400'
  return 'text-primary-600 dark:text-primary-400'
}

function formatTime(timestamp: number): string {
  try {
    return new Date(timestamp).toLocaleString()
  } catch {
    return ''
  }
}
</script>
