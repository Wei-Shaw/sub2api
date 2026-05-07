<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.filterTabs.title')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <div class="rounded-lg bg-slate-50 px-4 py-3 text-sm text-slate-600 dark:bg-dark-800 dark:text-slate-300">
        {{ t('admin.accounts.filterTabs.description') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.filterTabs.defaultTab') }}</label>
        <Select v-model="localDefaultTabId" :options="defaultTabOptions" class="w-full" />
      </div>

      <div class="space-y-3">
        <div
          v-for="(tab, index) in localTabs"
          :key="tab.id"
          class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 space-y-3">
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.label') }}</label>
                <input v-model="tab.label" type="text" class="input" :placeholder="t('admin.accounts.filterTabs.labelPlaceholder')" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.realGroup') }}</label>
                <Select v-model="tab.group" :options="groupOptions" class="w-full" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.displayGroup') }}</label>
                <input v-model="tab.display_group" type="text" class="input" :placeholder="t('admin.accounts.filterTabs.displayGroupPlaceholder')" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.keywordSearch') }}</label>
                <input v-model="tab.search" type="text" class="input" :placeholder="t('admin.accounts.filterTabs.keywordSearchPlaceholder')" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.namePrefix') }}</label>
                <input v-model="tab.name_prefix" type="text" class="input" :placeholder="t('admin.accounts.filterTabs.namePrefixPlaceholder')" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.accounts.filterTabs.searchRegex') }}</label>
                <input v-model="tab.search_regex" type="text" class="input font-mono" :placeholder="t('admin.accounts.filterTabs.searchRegexPlaceholder')" />
              </div>
              <p v-if="validationErrors[index]" class="text-xs text-red-500">
                {{ validationErrors[index] }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm text-red-600" @click="removeTab(index)">
              {{ t('common.delete') }}
            </button>
          </div>
        </div>
      </div>

      <button type="button" class="btn btn-secondary" @click="addTab">
        {{ t('admin.accounts.filterTabs.addTab') }}
      </button>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="$emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="!canSave" @click="save">
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SelectOption } from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { AdminGroup } from '@/types'

export interface AccountFilterTabConfig {
  id: string
  label: string
  group?: string
  display_group?: string
  search?: string
  name_prefix?: string
  search_regex?: string
}

interface BuiltinTabOption {
  id: string
  label: string
}

const props = defineProps<{
  show: boolean
  tabs: AccountFilterTabConfig[]
  defaultTabId: string
  builtinTabs: BuiltinTabOption[]
  groups: AdminGroup[]
}>()

const emit = defineEmits<{
  close: []
  save: [payload: { tabs: AccountFilterTabConfig[]; defaultTabId: string }]
}>()

const { t } = useI18n()

const localTabs = ref<AccountFilterTabConfig[]>([])
const localDefaultTabId = ref('all')

const createTabId = () => `custom-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`

const cloneTabs = (tabs: AccountFilterTabConfig[]) => tabs.map(tab => ({
  id: tab.id,
  label: tab.label || '',
  group: tab.group || '',
  display_group: tab.display_group || '',
  search: tab.search || '',
  name_prefix: tab.name_prefix || '',
  search_regex: tab.search_regex || ''
}))

watch(
  () => props.show,
  (show) => {
    if (!show) return
    localTabs.value = cloneTabs(props.tabs)
    localDefaultTabId.value = props.defaultTabId || 'all'
  },
  { immediate: true }
)

const groupOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.accounts.filterTabs.noRealGroup') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...props.groups.map(group => ({ value: String(group.id), label: group.name }))
])

const defaultTabOptions = computed<SelectOption[]>(() => [
  ...props.builtinTabs.map(tab => ({ value: tab.id, label: tab.label })),
  ...localTabs.value.map(tab => ({
    value: tab.id,
    label: tab.label || t('admin.accounts.filterTabs.unnamedTab')
  }))
])

const validationErrors = computed(() => localTabs.value.map((tab) => {
  if (!tab.label?.trim()) {
    return t('admin.accounts.filterTabs.validationLabel')
  }
  if (!tab.group?.trim() && !tab.display_group?.trim() && !tab.search?.trim() && !tab.name_prefix?.trim() && !tab.search_regex?.trim()) {
    return t('admin.accounts.filterTabs.validationCondition')
  }
  if (tab.search_regex?.trim()) {
    try {
      new RegExp(tab.search_regex, 'i')
    } catch {
      return t('admin.accounts.filterTabs.validationRegex')
    }
  }
  return ''
}))

const canSave = computed(() => validationErrors.value.every(error => !error))

const addTab = () => {
  localTabs.value.push({
    id: createTabId(),
    label: '',
    group: '',
    display_group: '',
    search: '',
    name_prefix: '',
    search_regex: ''
  })
}

const removeTab = (index: number) => {
  const [removed] = localTabs.value.splice(index, 1)
  if (removed && localDefaultTabId.value === removed.id) {
    localDefaultTabId.value = 'all'
  }
}

const save = () => {
  if (!canSave.value) return
  emit('save', {
    tabs: cloneTabs(localTabs.value),
    defaultTabId: localDefaultTabId.value || 'all'
  })
}
</script>
