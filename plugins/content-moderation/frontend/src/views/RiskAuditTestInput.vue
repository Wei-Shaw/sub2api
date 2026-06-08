<template>
  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30" @paste="ctx.handleModerationImagePaste">
    <div class="mb-3 flex items-center justify-between gap-3">
      <div>
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestInput') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestInputHint') }}</p>
      </div>
      <button
        v-if="ctx.moderationTestPrompt.value || ctx.moderationTestImages.value.length > 0 || ctx.moderationTestResult.value"
        type="button"
        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
        @click="ctx.clearModerationTestInput"
      >
        <Icon name="x" size="xs" />
        {{ t('admin.riskControl.clearAuditTest') }}
      </button>
    </div>
    <textarea
      v-model="ctx.moderationTestPrompt.value"
      class="input min-h-24 resize-y text-sm"
      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
    ></textarea>
    <div
      class="mt-3 rounded-lg border border-dashed border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
      @dragover.prevent
      @drop.prevent="ctx.handleModerationImageDrop"
    >
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-start gap-2">
          <Icon name="upload" size="md" class="mt-0.5 text-gray-400" />
          <div>
            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.riskControl.auditTestImages') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestImagesHint') }}</p>
          </div>
        </div>
        <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
          <Icon name="plus" size="sm" />
          {{ t('admin.riskControl.addAuditTestImage') }}
          <input type="file" accept="image/*" multiple class="sr-only" @change="ctx.handleModerationImageUpload" />
        </label>
      </div>
      <div v-if="ctx.moderationTestImages.value.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <div
          v-for="(image, index) in ctx.moderationTestImages.value"
          :key="image.slice(0, 64) + index"
          class="group relative aspect-square overflow-hidden rounded-lg border border-gray-100 bg-gray-100 dark:border-dark-700 dark:bg-dark-700"
        >
          <img :src="image" alt="" class="h-full w-full object-cover" />
          <button
            type="button"
            class="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"
            @click="ctx.removeModerationTestImage(index)"
          >
            <Icon name="x" size="xs" :stroke-width="2" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
