<template>
  <div class="space-y-5">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.groupScope') }}</h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.groupScopeHint') }}</p>
      </div>
      <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="ctx.configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
          @click="ctx.configForm.all_groups = true"
        >
          {{ t('admin.riskControl.allGroups') }}
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="!ctx.configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
          @click="ctx.configForm.all_groups = false"
        >
          {{ t('admin.riskControl.selectedGroups') }}
        </button>
      </div>
    </div>

    <div v-if="!ctx.configForm.all_groups" class="space-y-4">
      <div class="relative">
        <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
        <input v-model.trim="ctx.groupSearch.value" type="search" class="input pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
      </div>
      <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
        <button
          v-for="group in ctx.filteredGroups.value"
          :key="group.id"
          type="button"
          class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
          :class="ctx.isGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
          @click="ctx.toggleGroup(group.id)"
        >
          <span class="min-w-0">
            <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
            <span class="mt-1 inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ group.platform }}</span>
          </span>
          <span
            class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
            :class="ctx.isGroupSelected(group.id) ? 'border-primary-500 bg-primary-500 text-white' : 'border-gray-300 text-transparent dark:border-dark-500'"
          >
            <Icon name="check" size="xs" :stroke-width="2" />
          </span>
        </button>
        <p v-if="ctx.filteredGroups.value.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.noGroups') }}</p>
      </div>
    </div>

    <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
      <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.modelFilter') }}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.modelFilterHint') }}</p>
        </div>
        <span class="inline-flex w-fit rounded-md bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          {{ ctx.modelFilterSummary.value }}
        </span>
      </div>

      <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
        <button
          v-for="option in ctx.modelFilterOptions.value"
          :key="option.value"
          type="button"
          class="rounded-lg border p-3 text-left transition-colors"
          :class="ctx.configForm.model_filter_type === option.value
            ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
            : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
          @click="ctx.setModelFilterType(option.value)"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-sm font-semibold">{{ option.label }}</span>
            <span
              class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
              :class="ctx.configForm.model_filter_type === option.value
                ? 'border-primary-500 bg-primary-500 text-white'
                : 'border-gray-300 text-transparent dark:border-dark-500'"
            >
              <Icon name="check" size="xs" :stroke-width="2" />
            </span>
          </div>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
        </button>
      </div>

      <div v-if="ctx.configForm.model_filter_type !== 'all'" class="space-y-2">
        <label class="input-label">{{ t('admin.riskControl.modelFilterModels') }}</label>
        <ModelWhitelistSelector v-model="ctx.configForm.model_filter_models" />
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.modelFilterModelCount', { count: ctx.modelFilterModelCount.value }) }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import ModelWhitelistSelector from '../components/ModelWhitelistSelector.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
