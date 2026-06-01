<template>
  <div class="space-y-2">
    <div class="flex justify-end">
      <button class="btn btn-primary btn-sm" @click="emit('create')">
        <Icon name="plus" size="sm" class="mr-1" />
        {{ t('admin.channelMonitor.template.createButton') }}
      </button>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-400">
      {{ t('common.loading') }}
    </div>

    <div
      v-else-if="templates.length === 0"
      class="py-8 text-center text-sm text-gray-400"
    >
      {{ t('admin.channelMonitor.template.emptyState') }}
    </div>

    <div
      v-for="tpl in templates"
      v-else
      :key="tpl.id"
      class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="font-medium text-gray-900 dark:text-white">{{ tpl.name }}</span>
            <span
              class="inline-flex items-center rounded-md px-1.5 py-0.5 text-xs"
              :class="modeBadgeClass(tpl.body_override_mode)"
            >
              {{ modeLabel(tpl.body_override_mode) }}
            </span>
            <span
              v-if="tpl.associated_monitors > 0"
              class="text-xs text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.channelMonitor.template.associatedCount', { n: tpl.associated_monitors }) }}
            </span>
          </div>
          <p v-if="tpl.description" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ tpl.description }}
          </p>
          <p class="mt-1 text-xs text-gray-400">
            {{ t('admin.channelMonitor.template.headersSummary', { n: Object.keys(tpl.extra_headers || {}).length }) }}
          </p>
        </div>
        <div class="flex flex-shrink-0 gap-2">
          <button
            class="btn btn-secondary btn-sm"
            :disabled="tpl.associated_monitors === 0"
            :title="t('admin.channelMonitor.template.applyTooltip')"
            @click="emit('apply', tpl)"
          >
            <Icon name="refresh" size="sm" class="mr-1" />
            {{ t('admin.channelMonitor.template.applyButton') }}
          </button>
          <button class="btn btn-secondary btn-sm" @click="emit('edit', tpl)">
            {{ t('common.edit') }}
          </button>
          <button class="btn btn-secondary btn-sm text-semantic-danger" @click="emit('delete', tpl)">
            {{ t('common.delete') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import type { BodyOverrideMode } from '../../../api/admin/channelMonitor'
import type { ChannelMonitorTemplate } from '../../../api/admin/channelMonitorTemplate'

const { t } = useI18n()

defineProps<{
  templates: ChannelMonitorTemplate[]
  loading: boolean
}>()

const emit = defineEmits<{
  create: []
  edit: [tpl: ChannelMonitorTemplate]
  delete: [tpl: ChannelMonitorTemplate]
  apply: [tpl: ChannelMonitorTemplate]
}>()

function modeBadgeClass(mode: BodyOverrideMode): string {
  switch (mode) {
    case 'merge':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'replace':
      return 'bg-purple-100 text-purple-700 dark:bg-purple-500/15 dark:text-purple-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

function modeLabel(mode: BodyOverrideMode): string {
  return t(`admin.channelMonitor.advanced.bodyMode${mode.charAt(0).toUpperCase()}${mode.slice(1)}`)
}
</script>
