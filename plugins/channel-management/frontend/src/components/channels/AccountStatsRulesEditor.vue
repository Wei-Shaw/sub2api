<template>
  <div class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-700 space-y-3">
    <div class="flex items-center justify-between">
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.channels.form.accountStatsPricingRules') }}
      </h4>
      <button
        type="button"
        @click="addRule"
        class="rounded-lg border border-primary-300 px-3 py-1 text-xs font-medium text-primary-600 hover:bg-primary-50 dark:border-primary-600 dark:text-primary-400 dark:hover:bg-primary-900/20"
      >
        + {{ t('admin.channels.form.addRule') }}
      </button>
    </div>

    <p
      v-if="modelValue.length === 0"
      class="text-xs italic text-gray-400 dark:text-gray-500"
    >
      {{ t('admin.channels.form.noRulesConfigured') }}
    </p>

    <div
      v-for="(rule, ruleIndex) in modelValue"
      :key="ruleIndex"
      class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
    >
      <div class="flex items-center justify-between">
        <input
          :value="rule.name"
          :placeholder="t('admin.channels.form.ruleName')"
          class="bg-transparent text-sm font-medium text-gray-700 placeholder-gray-400 outline-none dark:text-gray-300"
          @input="updateRuleName(ruleIndex, ($event.target as HTMLInputElement).value)"
        />
        <button
          type="button"
          @click="removeRule(ruleIndex)"
          class="text-semantic-danger text-xs"
        >
          {{ t('common.delete', 'Delete') }}
        </button>
      </div>

      <div>
        <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleGroups') }}</label>
        <div class="mt-1 flex flex-wrap gap-1">
          <label
            v-for="gid in groupIds"
            :key="gid"
            class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors"
            :class="rule.group_ids.includes(gid)
              ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
              : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
          >
            <input
              type="checkbox"
              :checked="rule.group_ids.includes(gid)"
              class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="toggleRuleGroup(ruleIndex, gid)"
            />
            <span>{{ getGroupNameById(gid) }}</span>
          </label>
        </div>
        <p v-if="groupIds.length === 0" class="mt-1 text-xs text-gray-400">
          {{ t('admin.channels.form.noGroupsInChannel') }}
        </p>
      </div>

      <div>
        <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleAccounts') }}</label>
        <input
          :value="rule.account_ids.join(', ')"
          @change="updateRuleAccountIds(ruleIndex, ($event.target as HTMLInputElement).value)"
          :placeholder="t('admin.channels.form.ruleAccountsPlaceholder')"
          class="input mt-1 text-sm"
        />
      </div>

      <div>
        <div class="mb-1 flex items-center justify-between">
          <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleModelPricing') }}</label>
          <button
            type="button"
            @click="addRulePricing(ruleIndex)"
            class="text-xs text-primary-600 hover:text-primary-700"
          >
            + {{ t('common.add', 'Add') }}
          </button>
        </div>
        <div v-if="rule.pricing.length === 0" class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500">
          {{ t('admin.channels.form.noPricingRules') }}
        </div>
        <div v-else class="space-y-2">
          <PricingEntryCard
            v-for="(entry, pIdx) in rule.pricing"
            :key="pIdx"
            :entry="entry"
            :platform="platform"
            @update="updateRulePricing(ruleIndex, pIdx, $event)"
            @remove="removeRulePricing(ruleIndex, pIdx)"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * AccountStatsRulesEditor — 账号统计自定义定价规则编辑器 (T9 拆出).
 *
 * 仅做"规则列表"的展示与增删改, 不做 form ↔ API 转换 (这部分在
 * useChannelFormSerializer 里). 通过 v-model 双向同步 rules 数组,
 * 与父组件 (ChannelPlatformTab) 解耦.
 */
import { useI18n } from 'vue-i18n'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'
import type { AdminGroup, FormPricingRule, GroupPlatform } from './types'

const props = defineProps<{
  /** 当前平台的 rules 数组 (form state). */
  modelValue: FormPricingRule[]
  /** 平台 (用于子卡片配色). */
  platform: GroupPlatform
  /** 此平台 section 已选的 groupIds, 作为 rule.group_ids 候选集. */
  groupIds: number[]
  /** 全部 group, 用于 id → name 查找. */
  allGroups: AdminGroup[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: FormPricingRule[]): void
}>()

const { t } = useI18n()

function emitRules(next: FormPricingRule[]): void {
  emit('update:modelValue', next)
}

function addRule(): void {
  const next = props.modelValue.slice()
  next.push({ name: '', group_ids: [], account_ids: [], pricing: [] })
  emitRules(next)
}

function removeRule(idx: number): void {
  const next = props.modelValue.slice()
  next.splice(idx, 1)
  emitRules(next)
}

function updateRuleName(idx: number, value: string): void {
  const next = props.modelValue.slice()
  next[idx] = { ...next[idx], name: value }
  emitRules(next)
}

function toggleRuleGroup(idx: number, gid: number): void {
  const next = props.modelValue.slice()
  const rule = { ...next[idx], group_ids: [...next[idx].group_ids] }
  const pos = rule.group_ids.indexOf(gid)
  if (pos >= 0) {
    rule.group_ids.splice(pos, 1)
  } else {
    rule.group_ids.push(gid)
  }
  next[idx] = rule
  emitRules(next)
}

function updateRuleAccountIds(idx: number, raw: string): void {
  const ids = raw
    .split(',')
    .map((s) => parseInt(s.trim(), 10))
    .filter((n) => !isNaN(n) && n > 0)
  const next = props.modelValue.slice()
  next[idx] = { ...next[idx], account_ids: ids }
  emitRules(next)
}

function addRulePricing(ruleIdx: number): void {
  const next = props.modelValue.slice()
  const rule = { ...next[ruleIdx], pricing: [...next[ruleIdx].pricing] }
  rule.pricing.push(emptyPricing())
  next[ruleIdx] = rule
  emitRules(next)
}

function removeRulePricing(ruleIdx: number, pricingIdx: number): void {
  const next = props.modelValue.slice()
  const rule = { ...next[ruleIdx], pricing: [...next[ruleIdx].pricing] }
  rule.pricing.splice(pricingIdx, 1)
  next[ruleIdx] = rule
  next[ruleIdx] = rule
  emitRules(next)
}

function updateRulePricing(
  ruleIdx: number,
  pricingIdx: number,
  entry: PricingFormEntry,
): void {
  const next = props.modelValue.slice()
  const rule = { ...next[ruleIdx], pricing: [...next[ruleIdx].pricing] }
  rule.pricing.splice(pricingIdx, 1, entry)
  next[ruleIdx] = rule
  emitRules(next)
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

function getGroupNameById(groupId: number): string {
  const group = props.allGroups.find((g) => g.id === groupId)
  return group ? group.name : `#${groupId}`
}
</script>
