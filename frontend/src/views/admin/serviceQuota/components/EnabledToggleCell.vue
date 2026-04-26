<template>
  <div class="flex items-center gap-2">
    <Toggle :modelValue="row.enabled" @update:modelValue="onToggle" />
    <span class="text-xs text-gray-500 dark:text-gray-400">
      {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  updateServiceQuotaRule,
  type ServiceQuotaRule,
  type ServiceQuotaRuleInput,
} from '@/api/admin/serviceQuota'

const props = defineProps<{ row: ServiceQuotaRule }>()
const { t } = useI18n()
const appStore = useAppStore()

// 把规则现有字段重组为 update 接口需要的 Input 形状（仅前端 uid 字段会被 strip）
function buildPayload(row: ServiceQuotaRule, enabled: boolean): ServiceQuotaRuleInput {
  return {
    enabled,
    name: row.name ?? null,
    counter_mode: row.counter_mode,
    is_fallback: row.is_fallback,
    limiters: row.limiters.map((l) => ({
      limiter_type: l.limiter_type,
      window_mode: l.window_mode,
      limit_value: l.limit_value,
    })),
    paths: row.paths.map((p) => ({
      platform: p.platform ?? null,
      channel_id: p.channel_id ?? null,
      group_id: p.group_id ?? null,
      account_id: p.account_id ?? null,
      model_pattern: p.model_pattern ?? null,
    })),
    target_user_ids: row.target_user_ids ?? null,
  }
}

async function onToggle(next: boolean) {
  const prev = props.row.enabled
  // 乐观更新
  props.row.enabled = next
  try {
    const updated = await updateServiceQuotaRule(props.row.id, buildPayload(props.row, next))
    Object.assign(props.row, updated)
    appStore.showSuccess(t('admin.serviceQuota.toggleSuccess'))
  } catch (err: unknown) {
    props.row.enabled = prev
    appStore.showError(extractApiErrorMessage(err, t('admin.serviceQuota.toggleError')))
  }
}
</script>
