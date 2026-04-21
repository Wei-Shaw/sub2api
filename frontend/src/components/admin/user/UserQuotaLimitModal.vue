<template>
  <BaseDialog :show="show" :title="t('userQuota.modalTitle')" width="wide" @close="$emit('close')">
    <div v-if="user" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100"><span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span></div>
        <div class="flex-1">
          <p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('userQuota.modalDescription') }}</p>
        </div>
      </div>
      <div v-if="loading" class="flex justify-center py-10">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>
      <template v-else>
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('userQuota.userLevelSwitch') }}</label>
          <div class="flex flex-wrap gap-3">
            <label v-for="opt in overrideOptions" :key="String(opt.value)" class="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600" :class="{ 'border-primary-500 bg-primary-50 dark:bg-primary-900/20': overrideValue === opt.value }">
              <input type="radio" :value="opt.value" v-model="overrideValue" class="text-primary-600" />
              <span>{{ opt.label }}</span>
            </label>
          </div>
        </div>
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('userQuota.dailyLimitLabel') }}</label>
          <div class="relative">
            <div class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">$</div>
            <input v-model="dailyLimitInput" type="number" step="0.01" min="0" class="input pl-8" :placeholder="t('userQuota.dailyLimitPlaceholder')" />
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('userQuota.dailyLimitHint') }}</p>
        </div>
        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('userQuota.rulesLabel') }}</label>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="eligibleGroupsForNewRule.length === 0" @click="addRuleDraft">{{ t('userQuota.addRule') }}</button>
          </div>
          <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">{{ t('userQuota.rulesHint') }}</p>
          <div v-if="ruleDrafts.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('userQuota.noRules') }}</div>
          <div v-else class="space-y-3">
            <div v-for="(draft, idx) in ruleDrafts" :key="idx" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
              <div class="grid grid-cols-1 gap-3 md:grid-cols-[1fr_160px_auto]">
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('userQuota.ruleGroups') }}</label>
                  <div class="flex flex-wrap gap-1.5">
                    <label v-for="g in allEligibleGroups" :key="g.id" class="inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs" :class="draft.group_ids.includes(g.id) ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-600'">
                      <input type="checkbox" :checked="draft.group_ids.includes(g.id)" :disabled="!draft.group_ids.includes(g.id) && isGroupUsedInOtherDraft(g.id, idx)" @change="toggleRuleGroup(idx, g.id)" />
                      <span>{{ g.name }}</span>
                    </label>
                  </div>
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('userQuota.ruleLimit') }}</label>
                  <input v-model.number="draft.daily_limit_usd" type="number" step="0.01" min="0" class="input" />
                </div>
                <div class="flex items-end"><button type="button" class="btn btn-secondary text-red-600 dark:text-red-400" @click="removeRuleDraft(idx)">{{ t('userQuota.deleteRule') }}</button></div>
              </div>
              <div v-if="quotaData && draft.id" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('userQuota.todayUsageRule', { ruleId: draft.id }) }}: ${{ formatLimitUsd(ruleUsageFor(draft.id)) }}</div>
            </div>
          </div>
        </div>
        <div v-if="quotaData" class="rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-700">
          <div class="mb-1 flex items-center justify-between"><span class="text-gray-600 dark:text-gray-400">{{ t('userQuota.todayUsageLabel') }}</span><span class="font-medium text-gray-900 dark:text-white">${{ formatLimitUsd(quotaData.today_usage.total_used_usd) }}</span></div>
          <div class="mb-1 flex items-center justify-between"><span class="text-gray-600 dark:text-gray-400">{{ t('userQuota.resetAtLabel') }}</span><span class="text-gray-900 dark:text-white">{{ formatDateTime(quotaData.today_usage.reset_at) }}</span></div>
          <div class="flex items-center justify-between">
            <span class="text-gray-600 dark:text-gray-400">{{ t('userQuota.effectiveLimitLabel') }}</span>
            <span class="text-gray-900 dark:text-white">
              <template v-if="!quotaData.resolved.enabled">{{ t('userQuota.effectiveDisabled') }}</template>
              <template v-else-if="quotaData.resolved.daily_limit === null">{{ t('userQuota.unlimited') }}</template>
              <template v-else>${{ formatLimitUsd(quotaData.resolved.daily_limit) }}</template>
            </span>
          </div>
        </div>
      </template>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="$emit('close')">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting || loading" @click="handleSave">{{ submitting ? t('common.saving') : t('common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { userQuotaAPI } from '@/api/admin/quota'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime, formatLimitUsd } from '@/utils/format'
import {
  QUOTA_OVERRIDE_ENABLED, QUOTA_OVERRIDE_DISABLED, QUOTA_OVERRIDE_FOLLOW_GLOBAL,
  QUOTA_PERIOD_DAILY,
  QUOTA_ERR_RULE_OVERLAP, QUOTA_ERR_RULE_SUBSCRIPTION,
  QUOTA_ERR_RULE_GROUP_NOT_FOUND, QUOTA_ERR_RULE_NOT_FOUND,
} from '@/constants/quota'
import type { AdminUser, AdminGroup } from '@/types'
import type { UserQuotaView, UpdateUserQuotaRequest, ReplaceRuleInput } from '@/types/quota'

interface RuleDraft { id?: number; group_ids: number[]; daily_limit_usd: number }
type OverrideValue = boolean | null

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'success'): void }>()
const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const quotaData = ref<UserQuotaView | null>(null)
const overrideValue = ref<OverrideValue>(QUOTA_OVERRIDE_FOLLOW_GLOBAL)
const dailyLimitInput = ref<number | null>(null)
const ruleDrafts = ref<RuleDraft[]>([])
const allGroups = ref<AdminGroup[]>([])

const overrideOptions = computed(() => [
  { value: QUOTA_OVERRIDE_FOLLOW_GLOBAL as OverrideValue, label: t('userQuota.overrideFollow') },
  { value: QUOTA_OVERRIDE_ENABLED as OverrideValue, label: t('userQuota.overrideOn') },
  { value: QUOTA_OVERRIDE_DISABLED as OverrideValue, label: t('userQuota.overrideOff') },
])

const allEligibleGroups = computed<AdminGroup[]>(() =>
  allGroups.value.filter((g) => g.status === 'active' && g.subscription_type === 'standard')
)

const eligibleGroupsForNewRule = computed(() => {
  const used = new Set<number>()
  for (const d of ruleDrafts.value) for (const id of d.group_ids) used.add(id)
  return allEligibleGroups.value.filter((g) => !used.has(g.id))
})

// Admin CRUD 场景（规则增删改、配额配置更新）不会触发 USAGE_QUOTA_EXCEEDED
// 该错误码仅由网关扣费超限路径抛出，应由聊天/测试等 gateway 请求组件处理
const errorI18nMap = computed<Record<string, string>>(() => ({
  [QUOTA_ERR_RULE_OVERLAP]: t('userQuota.errors.QUOTA_RULE_GROUPS_OVERLAP'),
  [QUOTA_ERR_RULE_SUBSCRIPTION]: t('userQuota.errors.QUOTA_RULE_GROUP_SUBSCRIPTION'),
  [QUOTA_ERR_RULE_GROUP_NOT_FOUND]: t('userQuota.errors.QUOTA_RULE_GROUP_NOT_FOUND'),
  [QUOTA_ERR_RULE_NOT_FOUND]: t('userQuota.errors.QUOTA_RULE_NOT_FOUND'),
}))

watch(() => props.show, (v) => { if (v && props.user) void load() })

async function load(): Promise<void> {
  if (!props.user) return
  loading.value = true
  try {
    const [view, groupsRes] = await Promise.all([
      userQuotaAPI.getUserQuota(props.user.id),
      adminAPI.groups.list(1, 1000),
    ])
    quotaData.value = view
    allGroups.value = groupsRes.items
    overrideValue.value = view.user_override.usage_limit_enabled
    dailyLimitInput.value = view.user_override.daily_usage_limit_usd
    ruleDrafts.value = view.resolved.rules.map((r) => ({
      id: r.id, group_ids: [...r.group_ids], daily_limit_usd: r.daily_limit_usd,
    }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), errorI18nMap.value))
  } finally {
    loading.value = false
  }
}

function addRuleDraft(): void {
  const candidate = eligibleGroupsForNewRule.value[0]
  if (!candidate) return
  ruleDrafts.value.push({ group_ids: [candidate.id], daily_limit_usd: 0 })
}

function removeRuleDraft(idx: number): void { ruleDrafts.value.splice(idx, 1) }

function isGroupUsedInOtherDraft(groupId: number, currentIdx: number): boolean {
  return ruleDrafts.value.some((d, i) => i !== currentIdx && d.group_ids.includes(groupId))
}

function toggleRuleGroup(idx: number, groupId: number): void {
  const draft = ruleDrafts.value[idx]
  if (!draft) return
  const pos = draft.group_ids.indexOf(groupId)
  if (pos >= 0) { draft.group_ids.splice(pos, 1); return }
  if (!isGroupUsedInOtherDraft(groupId, idx)) draft.group_ids.push(groupId)
}

function ruleUsageFor(ruleId: number | undefined): number {
  if (!ruleId || !quotaData.value) return 0
  return quotaData.value.today_usage.rules_used[String(ruleId)] ?? 0
}

function validateDrafts(): string | null {
  const seen = new Set<number>()
  for (const d of ruleDrafts.value) {
    if (d.group_ids.length === 0) return t('userQuota.validationGroupsRequired')
    if (!(d.daily_limit_usd > 0)) return t('userQuota.validationLimitPositive')
    for (const gid of d.group_ids) {
      if (seen.has(gid)) return t('userQuota.validationGroupsOverlap')
      seen.add(gid)
    }
  }
  return null
}

async function handleSave(): Promise<void> {
  if (!props.user) return
  const err = validateDrafts()
  if (err) { appStore.showError(err); return }
  submitting.value = true
  try {
    // 1. 提交用户级总开关与总限额（不含规则）
    const body: UpdateUserQuotaRequest = {
      usage_limit_enabled: overrideValue.value,
      daily_usage_limit_usd:
        dailyLimitInput.value === null || dailyLimitInput.value === undefined || Number(dailyLimitInput.value) <= 0
          ? null : Number(dailyLimitInput.value),
    }
    await userQuotaAPI.updateUserQuota(props.user.id, body)

    // 2. 单事务幂等全量替换所有规则（后端会删除未出现的历史规则、新建/更新其他）
    const rules: ReplaceRuleInput[] = ruleDrafts.value.map((draft) => ({
      group_ids: [...draft.group_ids].sort((a, b) => a - b),
      daily_limit_usd: draft.daily_limit_usd,
      period: QUOTA_PERIOD_DAILY,
    }))
    await userQuotaAPI.replaceUserQuotaRules(props.user.id, rules)

    appStore.showSuccess(t('common.success'))
    emit('success'); emit('close')
  } catch (e: unknown) {
    appStore.showError(extractApiErrorMessage(e, t('common.error'), errorI18nMap.value))
  } finally {
    submitting.value = false
  }
}
</script>
