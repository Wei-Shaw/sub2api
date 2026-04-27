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
          <span class="input-label">{{ t('admin.serviceQuota.form.name') }}</span>
          <input v-model="form.name" :placeholder="t('admin.serviceQuota.form.namePlaceholder')" class="input" maxlength="128" />
        </label>
        <label class="form-field">
          <span class="input-label">{{ t('admin.serviceQuota.columns.status') }}</span>
          <select v-model="form.enabled" class="input">
            <option :value="true">{{ t('common.enabled') }}</option>
            <option :value="false">{{ t('common.disabled') }}</option>
          </select>
        </label>
        <label class="form-field">
          <span class="input-label">{{ t('admin.serviceQuota.form.counterMode') }}</span>
          <select v-model="form.counter_mode" class="input">
            <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ counterModeHint(form.counter_mode) }}</span>
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
        <span class="input-label">{{ t('admin.serviceQuota.form.targetUserIds') }}</span>
        <UserMultiSelect
          v-model="selectedTargetUsers"
          :placeholder="t('admin.serviceQuota.form.targetUserIdsPlaceholder')"
        />
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.targetUserIdsRequired') }}</span>
      </section>

      <CollapsibleSection
        :title="t('admin.serviceQuota.form.limitersTitle')"
        :hint="t('admin.serviceQuota.form.limitersHint')"
        :count="form.limiters.length"
        :collapsed="limitersCollapsed"
        @update:collapsed="limitersCollapsed = $event"
      >
        <LimiterEditor v-model="form.limiters" />
      </CollapsibleSection>

      <CollapsibleSection
        :title="t('admin.serviceQuota.form.pathsTitle')"
        :hint="t('admin.serviceQuota.form.pathsHint')"
        :count="form.paths.length"
        :collapsed="pathsCollapsed"
        @update:collapsed="pathsCollapsed = $event"
      >
        <PathEditor v-model="form.paths" />
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
import CollapsibleSection from './CollapsibleSection.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
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

// 每次 show 由 false → true 时按 editingRule 重置表单；
// 关闭对话框时不清空，避免渐隐过程中文案先消失
watch(
  () => props.show,
  (next, prev) => {
    if (next && !prev) resetForm(props.editingRule)
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
      limit_value: l.limit_value,
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

function normalizePayload(): ServiceQuotaRuleInput {
  // 限额必须 > 0：阻止意外提交 limit_value=0 导致规则永久拒绝请求
  for (const l of form.limiters) {
    if (!(Number(l.limit_value) > 0)) {
      throw new Error(t('admin.serviceQuota.errors.limitValueMustBePositive'))
    }
    // TPM/TPD 必须至少勾 1 项 token component；提交侧拦截，避免后端 400
    if (limiterUsesTokenComponents(l.limiter_type)) {
      const comps = l.token_components ?? TOKEN_COMPONENTS_DEFAULT
      if (comps.length === 0) {
        throw new Error(t('admin.serviceQuota.tokenComponents.minOneRequired'))
      }
    }
  }
  return {
    enabled: form.enabled,
    name: cleanText(form.name),
    counter_mode: form.counter_mode,
    is_fallback: form.is_fallback,
    // 注意：strip 掉 uid（仅前端用于 v-for stable key）
    limiters: form.limiters.map((l) => {
      const out: ServiceQuotaLimiterInput = {
        limiter_type: l.limiter_type,
        window_mode: l.limiter_type === 'concurrency' ? 'fixed' : l.window_mode,
        limit_value: Number(l.limit_value),
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
    appStore.showError(extractApiErrorMessage(error, t('admin.serviceQuota.saveError')))
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
