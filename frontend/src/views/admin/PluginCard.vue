<template>
  <article
    :class="[
      'group relative flex flex-col overflow-hidden rounded-xl border bg-white shadow-card transition-all hover:shadow-card-hover dark:bg-dark-800/60',
      plugin.uninstalled_at
        ? 'border-gray-300 opacity-75 dark:border-dark-600'
        : 'border-gray-200 dark:border-dark-700'
    ]"
  >
    <span
      v-if="plugin.builtin"
      class="absolute inset-y-0 left-0 w-1 bg-primary-500"
      aria-hidden="true"
    />

    <div class="flex flex-1 flex-col p-4">
      <!-- Top: icon + display_name + builtin badge -->
      <div class="flex items-start gap-3">
        <span
          :class="[
            'flex h-10 w-10 shrink-0 items-center justify-center rounded-lg [&_svg]:h-6 [&_svg]:w-6',
            plugin.uninstalled_at
              ? 'bg-gray-200/60 text-gray-400 grayscale dark:bg-dark-700/60 dark:text-gray-500'
              : 'bg-primary-500/10 text-primary-600 dark:text-primary-300'
          ]"
        >
          <span
            v-if="plugin.icon_svg"
            v-html="sanitizeSvg(plugin.icon_svg)"
            aria-hidden="true"
          />
          <Icon v-else name="cube" size="md" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="flex items-start justify-between gap-2">
            <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
              {{ plugin.display_name || plugin.name }}
            </h2>
            <span
              v-if="plugin.builtin"
              class="inline-flex shrink-0 items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
            >
              {{ t('admin.plugins.builtinTag') }}
            </span>
          </div>
          <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
            {{ plugin.name }}<span v-if="plugin.version"> · v{{ plugin.version }}</span>
          </p>
        </div>
      </div>

      <!-- Status row -->
      <div class="mt-3 flex items-center gap-2 text-xs">
        <template v-if="plugin.uninstalled_at">
          <span class="inline-block h-2 w-2 shrink-0 rounded-full bg-gray-400 dark:bg-gray-500" aria-hidden="true" />
          <span class="font-medium text-gray-600 dark:text-gray-300">
            {{ t('admin.plugins.softUninstalled') }}
          </span>
        </template>
        <template v-else>
          <span
            :class="['inline-block h-2 w-2 shrink-0 rounded-full', dotClass(plugin.state, plugin.enabled)]"
            aria-hidden="true"
          />
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ stateLabel(plugin, t) }}</span>
          <span
            v-if="plugin.enabled && plugin.started_at && !isZeroTime(plugin.started_at)"
            class="ml-auto truncate text-gray-500 dark:text-gray-400"
          >
            {{ t('admin.plugins.uptimePrefix') }} {{ formatUptime(plugin.started_at, nowMs) }}
          </span>
        </template>
      </div>

      <!-- Description -->
      <p class="mt-3 line-clamp-2 min-h-[2.5rem] text-xs text-gray-600 dark:text-gray-300">
        <span v-if="plugin.description">{{ plugin.description }}</span>
        <span v-else class="text-gray-400 dark:text-gray-500">—</span>
      </p>

      <!-- Last error -->
      <div
        v-if="plugin.last_error"
        class="mt-3 flex items-start gap-1.5 rounded-md border border-red-200 bg-red-50 px-2 py-1.5 text-[11px] text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300"
      >
        <Icon name="exclamationCircle" size="xs" class="mt-0.5 shrink-0" />
        <span class="line-clamp-2 break-all">{{ plugin.last_error }}</span>
      </div>

      <!-- Actions -->
      <div class="mt-4 flex flex-wrap items-center gap-1.5 pt-2">
        <button
          class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          @click="$emit('detail', plugin.name)"
        >
          <Icon name="infoCircle" size="xs" />
          {{ t('admin.plugins.detailButton') }}
        </button>

        <template v-if="plugin.uninstalled_at">
          <button
            class="inline-flex items-center gap-1 rounded-md border border-emerald-300 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 hover:bg-emerald-100 disabled:opacity-60 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
            :disabled="isBusy"
            @click="$emit('restore', plugin.name)"
          >
            <Icon name="refresh" size="xs" />
            {{ t('admin.plugins.restoreButton') }}
          </button>
          <button
            class="inline-flex items-center gap-1 rounded-md border border-red-300 bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100 disabled:opacity-60 dark:border-red-700 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/40"
            :disabled="isBusy"
            @click="$emit('purge', plugin)"
          >
            <Icon name="trash" size="xs" />
            {{ t('admin.plugins.purgeButton') }}
          </button>
        </template>

        <template v-else>
          <button
            v-if="plugin.has_settings === true"
            class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            @click="$emit('settings', plugin.name)"
          >
            <Icon name="cog" size="xs" />
            {{ t('admin.plugins.settingsButton') }}
          </button>
          <button
            v-if="plugin.enabled"
            class="inline-flex items-center gap-1 rounded-md border border-amber-300 bg-amber-50 px-2 py-1 text-xs font-medium text-amber-700 hover:bg-amber-100 disabled:opacity-60 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-300 dark:hover:bg-amber-900/40"
            :disabled="isBusy"
            @click="$emit('disable', plugin.name)"
          >
            {{ t('admin.plugins.disable') }}
          </button>
          <button
            v-else
            class="inline-flex items-center gap-1 rounded-md border border-emerald-300 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 hover:bg-emerald-100 disabled:opacity-60 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/40"
            :disabled="isBusy"
            @click="$emit('enable', plugin.name)"
          >
            {{ t('admin.plugins.enable') }}
          </button>
          <button
            class="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            :disabled="!plugin.enabled || isBusy"
            @click="$emit('restart', plugin.name)"
          >
            <Icon name="refresh" size="xs" :class="isBusy ? 'animate-spin' : ''" />
            {{ t('admin.plugins.restart') }}
          </button>
          <button
            v-if="!plugin.builtin"
            class="inline-flex items-center gap-1 rounded-md border border-red-300 bg-white px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50 disabled:opacity-60 dark:border-red-700 dark:bg-dark-800 dark:text-red-300 dark:hover:bg-red-900/20"
            :disabled="isBusy"
            @click="$emit('uninstall', plugin)"
          >
            <Icon name="trash" size="xs" />
            {{ t('admin.plugins.uninstallButton') }}
          </button>
          <button
            v-if="!plugin.builtin && !plugin.enabled"
            class="inline-flex items-center gap-1 rounded-md border border-red-300 bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100 disabled:opacity-60 dark:border-red-700 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/40"
            :disabled="isBusy"
            @click="$emit('remove-files', plugin)"
          >
            <Icon name="trash" size="xs" />
            {{ t('admin.plugins.removeFilesButton') }}
          </button>
        </template>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { sanitizeSvg } from '@/utils/sanitize'
import {
  type PluginInfo,
  dotClass,
  stateLabel,
  formatUptime,
  isZeroTime,
} from './pluginHelpers'

defineProps<{
  plugin: PluginInfo
  isBusy: boolean
  nowMs: number
}>()

defineEmits<{
  detail: [name: string]
  settings: [name: string]
  enable: [name: string]
  disable: [name: string]
  restart: [name: string]
  uninstall: [plugin: PluginInfo]
  restore: [name: string]
  purge: [plugin: PluginInfo]
  'remove-files': [plugin: PluginInfo]
}>()

const { t } = useI18n()
</script>
