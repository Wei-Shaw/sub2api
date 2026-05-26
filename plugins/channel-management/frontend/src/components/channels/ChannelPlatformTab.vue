<template>
  <div class="space-y-4">
    <!-- Groups -->
    <div>
      <label class="input-label text-xs">
        {{ t('admin.channels.form.groups', 'Associated Groups') }} <span class="input-required">*</span>
        <span v-if="section.group_ids.length > 0" class="ml-1 font-normal text-gray-400">
          ({{ t('admin.channels.form.selectedCount', { count: section.group_ids.length }, `已选 ${section.group_ids.length} 个`) }})
        </span>
      </label>
      <div class="max-h-40 overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-900">
        <div v-if="groupsLoading" class="py-2 text-center text-xs text-gray-500">
          {{ t('common.loading', 'Loading...') }}
        </div>
        <div v-else-if="platformGroups.length === 0" class="py-2 text-center text-xs text-gray-500">
          {{ t('admin.channels.form.noGroupsAvailable', 'No groups available') }}
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <label
            v-for="group in platformGroups"
            :key="group.id"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
            :class="[
              section.group_ids.includes(group.id) ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700' : '',
              isGroupInOtherChannel(group.id) ? 'opacity-40' : ''
            ]"
          >
            <input
              type="checkbox"
              :checked="section.group_ids.includes(group.id)"
              :disabled="isGroupInOtherChannel(group.id)"
              class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="toggleGroup(group.id)"
            />
            <span :class="['font-medium', platformTextClass(group.platform)]">{{ group.name }}</span>
            <span
              :class="['rounded-full px-1 py-0 text-[10px]', platformTagClass(group.platform)]"
            >{{ group.rate_multiplier }}x</span>
            <span class="text-[10px] text-gray-400">{{ group.account_count || 0 }}</span>
            <span
              v-if="isGroupInOtherChannel(group.id)"
              class="text-[10px] text-gray-400"
            >{{ getGroupInOtherChannelLabel(group.id) }}</span>
          </label>
        </div>
      </div>
    </div>

    <!-- Model Mapping -->
    <div>
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelMapping', 'Model Mapping') }}</label>
        <button type="button" @click="addMappingEntry" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('common.add', 'Add') }}
        </button>
      </div>
      <div
        v-if="Object.keys(section.model_mapping).length === 0"
        class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
      >
        {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="(_, srcModel) in section.model_mapping"
          :key="srcModel"
          class="flex items-center gap-2"
        >
          <input
            :value="srcModel"
            type="text"
            class="input flex-1 text-xs"
            :class="platformTextClass(section.platform)"
            :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
            @change="renameMappingKey(srcModel, ($event.target as HTMLInputElement).value)"
          />
          <span class="text-gray-400 text-xs">→</span>
          <input
            :value="section.model_mapping[srcModel]"
            type="text"
            class="input flex-1 text-xs"
            :class="platformTextClass(section.platform)"
            :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
            @input="setMappingValue(srcModel, ($event.target as HTMLInputElement).value)"
          />
          <button
            type="button"
            @click="removeMappingEntry(srcModel)"
            class="btn-icon-danger p-0.5"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <!-- Model Pricing -->
    <div>
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }}</label>
        <button type="button" @click="addPricingEntry" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('common.add', 'Add') }}
        </button>
      </div>
      <div
        v-if="section.model_pricing.length === 0"
        class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
      >
        {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-2">
        <PricingEntryCard
          v-for="(entry, idx) in section.model_pricing"
          :key="idx"
          :entry="entry"
          :platform="section.platform"
          @update="updatePricingEntry(idx, $event)"
          @remove="removePricingEntry(idx)"
        />
      </div>
    </div>

    <!-- Account Stats Pricing Rules (per-platform; always visible) -->
    <AccountStatsRulesEditor
      :model-value="section.account_stats_pricing_rules"
      :platform="section.platform"
      :group-ids="section.group_ids"
      :all-groups="allGroups"
      @update:model-value="updateRules"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * ChannelPlatformTab — 渠道编辑对话框中"单个平台 tab"的内容 (T9 拆出).
 *
 * 包含 groups picker / model mapping / model pricing / account stats rules
 * 四个区块. 通过 v-model:section 双向同步 PlatformSection state.
 *
 * Props:
 *   - section          (双向): 此平台 form state
 *   - allGroups        : 全部 group, 用于 picker / 名称查找
 *   - groupsLoading    : Loading flag, picker 显示骨架文案
 *   - groupChannelMap  : 已被其它渠道占用的 groupId → channel name 映射
 *
 * 解耦说明: 不感知"当前编辑的 channel id"; isGroupInOtherChannel 由父组件传入
 * 计算好的 map (父组件已排除自身).
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import PricingEntryCard from '../PricingEntryCard.vue'
import AccountStatsRulesEditor from './AccountStatsRulesEditor.vue'
import type { PricingFormEntry } from '../types'
import type {
  AdminGroup,
  FormPricingRule,
  PlatformSection,
} from './types'
import { platformTagClass, platformTextClass } from '@sub2api/plugin-sdk'

const props = defineProps<{
  section: PlatformSection
  allGroups: AdminGroup[]
  groupsLoading: boolean
  /** Map<groupId, channelName>, 父组件已排除当前正在编辑的渠道. */
  groupChannelMap: Map<number, string>
}>()

const emit = defineEmits<{
  (e: 'update:section', value: PlatformSection): void
}>()

const { t } = useI18n()

const platformGroups = computed(() =>
  props.allGroups.filter((g) => g.platform === props.section.platform),
)

function emitSection(next: PlatformSection): void {
  emit('update:section', next)
}

// ── Helpers ──
function isGroupInOtherChannel(groupId: number): boolean {
  return props.groupChannelMap.has(groupId)
}

function getGroupInOtherChannelLabel(groupId: number): string {
  const name = props.groupChannelMap.get(groupId) || ''
  return t('admin.channels.form.inOtherChannel', { name }, `In "${name}"`)
}

// ── Group toggle ──
function toggleGroup(groupId: number): void {
  const next = { ...props.section, group_ids: [...props.section.group_ids] }
  const idx = next.group_ids.indexOf(groupId)
  if (idx >= 0) next.group_ids.splice(idx, 1)
  else next.group_ids.push(groupId)
  emitSection(next)
}

// ── Mapping ──
function addMappingEntry(): void {
  const mapping = { ...props.section.model_mapping }
  let key = ''
  let i = 1
  while (key === '' || key in mapping) {
    key = `model-${i}`
    i++
  }
  mapping[key] = ''
  emitSection({ ...props.section, model_mapping: mapping })
}

function removeMappingEntry(key: string): void {
  const mapping = { ...props.section.model_mapping }
  delete mapping[key]
  emitSection({ ...props.section, model_mapping: mapping })
}

function renameMappingKey(oldKey: string, rawNewKey: string): void {
  const newKey = rawNewKey.trim()
  if (!newKey || newKey === oldKey) return
  const mapping = { ...props.section.model_mapping }
  if (newKey in mapping) return
  mapping[newKey] = mapping[oldKey]
  delete mapping[oldKey]
  emitSection({ ...props.section, model_mapping: mapping })
}

function setMappingValue(key: string, value: string): void {
  const mapping = { ...props.section.model_mapping, [key]: value }
  emitSection({ ...props.section, model_mapping: mapping })
}

// ── Pricing ──
function addPricingEntry(): void {
  const pricing = [...props.section.model_pricing, emptyPricing()]
  emitSection({ ...props.section, model_pricing: pricing })
}

function updatePricingEntry(idx: number, entry: PricingFormEntry): void {
  const pricing = [...props.section.model_pricing]
  pricing.splice(idx, 1, entry)
  emitSection({ ...props.section, model_pricing: pricing })
}

function removePricingEntry(idx: number): void {
  const pricing = [...props.section.model_pricing]
  pricing.splice(idx, 1)
  emitSection({ ...props.section, model_pricing: pricing })
}

function emptyPricing(): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
  }
}

// ── Account stats rules ──
function updateRules(next: FormPricingRule[]): void {
  emitSection({ ...props.section, account_stats_pricing_rules: next })
}
</script>
