<template>
  <BaseDialog
    :show="show"
    :title="editingRule ? t('admin.serviceQuota.editRule') : t('admin.serviceQuota.createRule')"
    width="wide"
    @close="onClose"
  >
    <form id="service-quota-form" class="space-y-6" @submit.prevent="save">
      <section class="grid gap-4 md:grid-cols-2">
        <label class="form-field md:col-span-2">
          <RequiredLabel :label="t('admin.serviceQuota.form.name')" />
          <input v-model="form.name" :placeholder="t('admin.serviceQuota.form.namePlaceholder')" class="input" maxlength="128" />
        </label>
        <label class="form-field">
          <RequiredLabel :label="t('admin.serviceQuota.columns.status')" required />
          <select v-model="form.enabled" class="input">
            <option :value="true">{{ t('common.enabled') }}</option>
            <option :value="false">{{ t('common.disabled') }}</option>
          </select>
        </label>
        <label class="form-field">
          <RequiredLabel :label="t('admin.serviceQuota.form.counterMode')" required />
          <select
            v-model="form.counter_mode"
            :class="['input', { 'border-red-400 focus:ring-red-500': !!errorFor('counter_mode') }]"
          >
            <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ counterModeHint(form.counter_mode) }}</span>
          <span
            v-if="errorFor('counter_mode')"
            class="block text-xs text-red-500"
          >
            {{ t('admin.serviceQuota.errors.' + errorFor('counter_mode')) }}
          </span>
        </label>
        <label class="form-field md:col-span-2 flex items-center gap-2">
          <input v-model="form.is_fallback" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
          <span class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">{{ t('admin.serviceQuota.form.fallback') }}</span>
            <span class="ml-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.fallback.hint') }}</span>
          </span>
        </label>
      </section>

      <section v-if="form.counter_mode === 'user'" class="space-y-2">
        <RequiredLabel :label="t('admin.serviceQuota.form.targetUserIds')" required />
        <UserMultiSelect
          v-model="selectedTargetUsers"
          :placeholder="t('admin.serviceQuota.form.targetUserIdsPlaceholder')"
        />
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.targetUserIdsRequired') }}</span>
        <span
          v-if="errorFor('target_user_ids')"
          class="block text-xs text-red-500"
        >
          {{ t('admin.serviceQuota.errors.' + errorFor('target_user_ids')) }}
        </span>
      </section>

      <CollapsibleSection
        :title="t('admin.serviceQuota.form.limitersTitle')"
        :hint="t('admin.serviceQuota.form.limitersHint')"
        :count="form.limiters.length"
        :collapsed="limitersCollapsed"
        @update:collapsed="limitersCollapsed = $event"
      >
        <LimiterEditor v-model="form.limiters" :errors="validationErrors" />
      </CollapsibleSection>

      <CollapsibleSection
        :title="t('admin.serviceQuota.form.pathsTitle')"
        :hint="t('admin.serviceQuota.form.pathsHint')"
        :count="form.paths.length"
        :collapsed="pathsCollapsed"
        @update:collapsed="pathsCollapsed = $event"
      >
        <PathEditor v-model="form.paths" :errors="validationErrors" />
      </CollapsibleSection>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" @click="onClose">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" type="submit" form="service-quota-form" :disabled="saving">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * 服务限额规则编辑对话框（新建 + 编辑共用）。
 *
 * 设计：
 * - 受控显隐：父级用 v-model:show 控制；子组件不管列表加载，只发 saved 事件让父级 reload
 * - 内部封装 form / selectedTargetUsers / 折叠状态 / 保存中状态，避免父级 ConfigView 膨胀
 * - normalizePayload 在子组件内部完成 strip uid / 校验 / target_user_ids 落库
 */
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import UserMultiSelect from '@/components/common/UserMultiSelect.vue'
import LimiterEditor from '@/components/admin/LimiterEditor.vue'
import PathEditor from '@/components/admin/PathEditor.vue'
import RequiredLabel from '@/components/common/RequiredLabel.vue'
import CollapsibleSection from './CollapsibleSection.vue'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  validateServiceQuotaRule,
  parseServiceQuotaApiErrors,
  type ValidationError,
} from '@/utils/validateServiceQuota'
import type { SimpleUser } from '@/api/admin/usage'
import {
  createServiceQuotaRule,
  updateServiceQuotaRule,
  limiterUsesTokenComponents,
  TOKEN_COMPONENTS_DEFAULT,
  type ServiceQuotaLimiterInput,
  type ServiceQuotaRule,
  type ServiceQuotaRuleInput,
} from '@/api/admin/serviceQuota'

const props = defineProps<{
  show: boolean
  editingRule: ServiceQuotaRule | null
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const counterModeOptions = computed(() => [
  { value: 'user', label: t('admin.serviceQuota.counterModes.user') },
  { value: 'per_user', label: t('admin.serviceQuota.counterModes.perUser') },
  { value: 'shared', label: t('admin.serviceQuota.counterModes.shared') },
])

const form = reactive<ServiceQuotaRuleInput>(blankRule())
const selectedTargetUsers = ref<SimpleUser[]>([])
const limitersCollapsed = ref(false)
const pathsCollapsed = ref(false)
const saving = ref(false)

// validationErrors 兼容前端 validateServiceQuotaRule 输出与后端
// SERVICE_QUOTA_VALIDATION_ERROR.details.fields；都以 {path, code} 形式。
// errorFor(path) 返回该字段第一条 code，子组件按 path 自取。
const validationErrors = ref<ValidationError[]>([])

function errorFor(path: string): string {
  return validationErrors.value.find((e) => e.path === path)?.code || ''
}

// 每次 show 由 false → true 时按 editingRule 重置表单；
// 关闭对话框时不清空，避免渐隐过程中文案先消失
watch(
  () => props.show,
  (next, prev) => {
    if (next && !prev) {
      resetForm(props.editingRule)
      validationErrors.value = []
    }
  }
)

function blankRule(): ServiceQuotaRuleInput {
  return {
    enabled: true,
    name: null,
    counter_mode: 'per_user',
    is_fallback: false,
    target_user_ids: null,
    limiters: [{ uid: crypto.randomUUID(), limiter_type: 'rpm', window_mode: 'fixed', limit_value: 60 }],
    paths: [{ uid: crypto.randomUUID(), platform: null, channel_id: null, group_id: null, account_id: null, model_pattern: null }],
  }
}

function resetForm(rule: ServiceQuotaRule | null) {
  const initial = blankRule()
  if (rule) {
    initial.enabled = rule.enabled
    initial.name = rule.name ?? null
    initial.counter_mode = rule.counter_mode
    initial.is_fallback = rule.is_fallback
    initial.limiters = rule.limiters.map((l) => ({
      uid: crypto.randomUUID(),
      limiter_type: l.limiter_type,
      window_mode: l.window_mode,
      // 整数型 limiter（rpm/tpm/tpd/concurrency）老数据可能是 999999.999997
      // 这种 IEEE754 误差残留：编辑前 round 一次，避免编辑框直接呈现毛刺值；
      // daily_usd 保留原小数（金额需要小数精度）
      limit_value: l.limiter_type === 'daily_usd' ? l.limit_value : Math.round(l.limit_value),
      // TPM/TPD 才有 token_components；后端可能返回 null/undefined，统一展开成数组
      token_components: limiterUsesTokenComponents(l.limiter_type)
        ? (l.token_components ?? TOKEN_COMPONENTS_DEFAULT).slice()
        : undefined,
      // RPM 才有 count_on_arrival；后端默认 false
      count_on_arrival: l.limiter_type === 'rpm' ? l.count_on_arrival === true : undefined,
    }))
    initial.paths = rule.paths.map((p) => ({
      uid: crypto.randomUUID(),
      platform: p.platform ?? null,
      channel_id: p.channel_id ?? null,
      group_id: p.group_id ?? null,
      account_id: p.account_id ?? null,
      model_pattern: p.model_pattern ?? null,
    }))
    initial.target_user_ids = rule.target_user_ids ?? null
  }
  Object.assign(form, initial)
  selectedTargetUsers.value = (rule?.target_users || []).map((u) => ({ id: u.id, email: u.email }))
}

function counterModeHint(value: string): string {
  const map: Record<string, string> = {
    user: t('admin.serviceQuota.counterModeHints.user'),
    per_user: t('admin.serviceQuota.counterModeHints.perUser'),
    shared: t('admin.serviceQuota.counterModeHints.shared'),
  }
  return map[value] || ''
}

function cleanText(value?: string | null): string | null {
  return value && value.trim() ? value.trim() : null
}

function cleanNumber(value?: number | null): number | null {
  return value && value > 0 ? value : null
}

// normalizePayload 仅做"strip uid + 按类型决定可选字段"两件事；
// 校验已经被 validateServiceQuotaRule 接管，这里假设 form 已合规。
function normalizePayload(): ServiceQuotaRuleInput {
  return {
    enabled: form.enabled,
    name: cleanText(form.name),
    counter_mode: form.counter_mode,
    is_fallback: form.is_fallback,
    // 注意：strip 掉 uid（仅前端用于 v-for stable key）
    limiters: form.limiters.map((l) => {
      const rawLimit = Number(l.limit_value)
      // 整数型 limiter 提交前最后一次 round 防御：保证写入 DB 的总是整数，
      // 避免任何上游路径泄漏 IEEE754 误差。daily_usd 保留小数。
      const limitValue = l.limiter_type === 'daily_usd' ? rawLimit : Math.round(rawLimit)
      const out: ServiceQuotaLimiterInput = {
        limiter_type: l.limiter_type,
        window_mode: l.limiter_type === 'concurrency' ? 'fixed' : l.window_mode,
        limit_value: limitValue,
      }
      // 只有 TPM/TPD 才提交 token_components；其他类型不带，让后端自然走默认/忽略路径
      if (limiterUsesTokenComponents(l.limiter_type)) {
        out.token_components = (l.token_components ?? TOKEN_COMPONENTS_DEFAULT).slice()
      }
      // 只有 RPM 才提交 count_on_arrival；缺省 false 与后端一致
      if (l.limiter_type === 'rpm') {
        out.count_on_arrival = l.count_on_arrival === true
      }
      return out
    }),
    paths: form.paths.map((p) => ({
      platform: cleanText(p.platform),
      channel_id: cleanNumber(p.channel_id),
      group_id: cleanNumber(p.group_id),
      account_id: cleanNumber(p.account_id),
      model_pattern: cleanText(p.model_pattern),
    })),
    target_user_ids: form.counter_mode === 'user' ? selectedTargetUsers.value.map((u) => u.id) : null,
  }
}

async function save() {
  // 提交前先把 selectedTargetUsers 同步到 form.target_user_ids，让校验函数能拿到
  const formForValidate: ServiceQuotaRuleInput = {
    ...form,
    target_user_ids:
      form.counter_mode === 'user'
        ? selectedTargetUsers.value.map((u) => u.id)
        : null,
  }
  const result = validateServiceQuotaRule(formForValidate)
  validationErrors.value = result.errors
  if (!result.ok) {
    // 客户端校验未通过：直接显示行内错误，不调 API；toast 给一个总览提示
    appStore.showError(t('admin.serviceQuota.errors.formInvalid'))
    return
  }

  saving.value = true
  try {
    const payload = normalizePayload()
    if (props.editingRule) {
      await updateServiceQuotaRule(props.editingRule.id, payload)
    } else {
      await createServiceQuotaRule(payload)
    }
    appStore.showSuccess(t('admin.serviceQuota.saveSuccess'))
    emit('update:show', false)
    emit('saved')
  } catch (error: unknown) {
    // 后端 SERVICE_QUOTA_VALIDATION_ERROR：把字段错误投影到行内显示
    const apiErrors = parseServiceQuotaApiErrors(error)
    if (apiErrors && apiErrors.length > 0) {
      validationErrors.value = apiErrors
      appStore.showError(t('admin.serviceQuota.errors.formInvalid'))
      return
    }
    // 其他错误：优先按后端 reason 查 common.errors.* 本地化文案
    // （如 INVALID_REQUEST_BODY / INVALID_ID / SERVICE_QUOTA_UNAVAILABLE），
    // miss 则走 saveError 兜底；最后才退化到后端英文 message
    const fallback = t('admin.serviceQuota.saveError')
    appStore.showError(extractI18nErrorMessage(error, t, 'common.errors', fallback))
  } finally {
    saving.value = false
  }
}

function onClose() {
  emit('update:show', false)
}
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}
</style>
