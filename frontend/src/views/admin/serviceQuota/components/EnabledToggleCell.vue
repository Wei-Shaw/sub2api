<template>
  <div class="flex items-center gap-2">
    <Toggle :modelValue="localEnabled" @update:modelValue="onToggle" />
    <span class="text-xs text-gray-500 dark:text-gray-400">
      {{ localEnabled ? t('common.enabled') : t('common.disabled') }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  updateServiceQuotaRule,
  type ServiceQuotaRule,
  type ServiceQuotaRuleInput,
} from '@/api/admin/serviceQuota'

const props = defineProps<{ row: ServiceQuotaRule }>()
const emit = defineEmits<{ (e: 'updated'): void }>()
const { t } = useI18n()
const appStore = useAppStore()

// 本地镜像 props.row.enabled，避免直接 mutate prop（vue/no-mutating-props）。
// 父组件收到 emit('updated') 后会 reload 列表，覆盖 localEnabled。
const localEnabled = ref(props.row.enabled)
watch(() => props.row.enabled, (v) => { localEnabled.value = v })

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
      // 同理透传 count_on_arrival：仅 RPM 有；不带的话翻转 enabled 时
      // 会把用户配置的 "请求到达即计入" 复位为后端默认值。
      ...(l.limiter_type === 'rpm' && typeof l.count_on_arrival === 'boolean'
        ? { count_on_arrival: l.count_on_arrival }
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
  const prev = localEnabled.value
  localEnabled.value = next
  try {
    await updateServiceQuotaRule(props.row.id, buildPayload(props.row, next))
    emit('updated')
    // 后端在 enabled 翻转时会清空该规则下所有 limiter 计数器（counter reset），
    // toast 里同时提示一下，避免用户疑惑"刚被限流的为什么又能进了"。
    appStore.showSuccess(
      `${t('admin.serviceQuota.toggleSuccess')} · ${t('admin.serviceQuota.counterResetOnToggle')}`
    )
  } catch (err: unknown) {
    localEnabled.value = prev
    appStore.showError(
      extractI18nErrorMessage(err, t, 'common.errors', t('admin.serviceQuota.toggleError')),
    )
  }
}
</script>
