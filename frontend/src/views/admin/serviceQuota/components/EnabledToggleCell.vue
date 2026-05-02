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
      // 守卫透传 token_components：仅 TPM/TPD 会有该字段；
      // 不带的话后端 normalizeLimiterTokenComponents 会回填默认值，
      // 导致用户在编辑里取消勾选的 component 通过 toggle 又被恢复。
      ...(l.token_components && l.token_components.length > 0
        ? { token_components: [...l.token_components] }
        : {}),
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
    // 后端在 enabled 翻转时会清空该规则下所有 limiter 计数器（counter reset），
    // toast 里同时提示一下，避免用户疑惑"刚被限流的为什么又能进了"。
    appStore.showSuccess(
      `${t('admin.serviceQuota.toggleSuccess')} · ${t('admin.serviceQuota.counterResetOnToggle')}`
    )
  } catch (err: unknown) {
    props.row.enabled = prev
    appStore.showError(extractApiErrorMessage(err, t('admin.serviceQuota.toggleError')))
  }
}
</script>
