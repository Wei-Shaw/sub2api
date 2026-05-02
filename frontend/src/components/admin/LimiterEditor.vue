<template>
  <div class="space-y-3">
    <!-- 数组空时占位提示。父组件提交校验后若 errors 命中 path='limiters' 切换为红色边框 + 红字
         （让 user 看到"至少配置一个限流器"是必填错误，不只是占位） -->
    <div
      v-if="modelValue.length === 0"
      :class="[
        'rounded-lg border border-dashed px-4 py-3 text-center text-sm',
        errorFor('limiters')
          ? 'border-red-400 bg-red-50 text-red-600 dark:border-red-500 dark:bg-red-900/10 dark:text-red-400'
          : 'border-gray-300 text-gray-500 dark:border-dark-600 dark:text-gray-400',
      ]"
    >
      <span v-if="errorFor('limiters')">
        {{ t('admin.serviceQuota.errors.' + errorFor('limiters')) }} · {{ t('admin.serviceQuota.limiterEditor.empty') }}
      </span>
      <span v-else>{{ t('admin.serviceQuota.limiterEditor.empty') }}</span>
    </div>
    <div
      v-for="(item, index) in modelValue"
      :key="item.uid ?? index"
      class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
    >
      <div class="grid grid-cols-1 items-end gap-3 sm:grid-cols-[160px_1fr_140px_auto]">
        <label class="form-field">
          <RequiredLabel :label="t('admin.serviceQuota.columns.type')" required />
          <select
            :value="item.limiter_type"
            :class="['input', { 'border-red-400 focus:ring-red-500': !!errorFor(`limiters[${index}].limiter_type`) }]"
            @change="updateField(index, 'limiter_type', ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="opt in availableTypes(item.limiter_type)" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
          <span
            v-if="errorFor(`limiters[${index}].limiter_type`)"
            class="text-xs text-red-500"
          >
            {{ t('admin.serviceQuota.errors.' + errorFor(`limiters[${index}].limiter_type`)) }}
          </span>
        </label>
        <label class="form-field">
          <RequiredLabel :label="t('admin.serviceQuota.columns.limit')" required />
          <NumericInput
            :model-value="item.limit_value ?? null"
            :min="0"
            :step="getLimitStep(item.limiter_type)"
            :integer="!isDailyUsd(item.limiter_type)"
            :has-error="!!errorFor(`limiters[${index}].limit_value`)"
            @update:model-value="updateField(index, 'limit_value', $event as number)"
          />
          <span
            v-if="errorFor(`limiters[${index}].limit_value`)"
            class="text-xs text-red-500"
          >
            {{ t('admin.serviceQuota.errors.' + errorFor(`limiters[${index}].limit_value`)) }}
          </span>
        </label>
        <label v-if="item.limiter_type !== 'concurrency'" class="form-field">
          <RequiredLabel :label="t('admin.serviceQuota.columns.window')" required />
          <select
            :value="item.window_mode"
            :class="['input', { 'border-red-400 focus:ring-red-500': !!errorFor(`limiters[${index}].window_mode`) }]"
            @change="updateField(index, 'window_mode', ($event.target as HTMLSelectElement).value)"
          >
            <option value="fixed">{{ t('admin.serviceQuota.windows.fixed') }}</option>
            <option value="rolling">{{ t('admin.serviceQuota.windows.rolling') }}</option>
          </select>
          <span
            v-if="errorFor(`limiters[${index}].window_mode`)"
            class="text-xs text-red-500"
          >
            {{ t('admin.serviceQuota.errors.' + errorFor(`limiters[${index}].window_mode`)) }}
          </span>
        </label>
        <div v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.windows.none') }}</div>
        <button
          type="button"
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          :title="t('common.delete')"
          @click="remove(index)"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>

      <!-- TPM/TPD 才显示 token components 选择；其他类型隐藏。
           至少 1 项校验：未勾选时显示红字提示，提交侧由父组件统一阻断。 -->
      <div v-if="usesTokenComponents(item.limiter_type)" class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
            {{ t('admin.serviceQuota.tokenComponents.title') }}
          </span>
          <label
            v-for="comp in TOKEN_COMPONENTS_ALL"
            :key="comp"
            class="inline-flex items-center gap-1.5 text-xs text-gray-700 dark:text-gray-200 cursor-pointer"
          >
            <input
              type="checkbox"
              class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="(item.token_components ?? TOKEN_COMPONENTS_DEFAULT).includes(comp)"
              @change="toggleTokenComponent(index, comp, ($event.target as HTMLInputElement).checked)"
            />
            <span>{{ t('admin.serviceQuota.tokenComponents.' + comp) }}</span>
          </label>
        </div>
        <div
          v-if="errorFor(`limiters[${index}].token_components`) || (item.token_components ?? TOKEN_COMPONENTS_DEFAULT).length === 0"
          class="mt-1 text-xs text-red-500"
        >
          {{ t('admin.serviceQuota.tokenComponents.minOneRequired') }}
        </div>
      </div>

      <!-- 仅 RPM 显示"请求到达即计入" toggle；默认 false（仅成功请求计入）。 -->
      <div v-if="usesCountOnArrival(item.limiter_type)" class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-700">
        <label class="inline-flex items-start gap-2 text-xs text-gray-700 dark:text-gray-200 cursor-pointer">
          <input
            type="checkbox"
            class="mt-0.5 h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :checked="item.count_on_arrival === true"
            @change="updateField(index, 'count_on_arrival', ($event.target as HTMLInputElement).checked)"
          />
          <span class="flex flex-col gap-0.5">
            <span class="font-medium">{{ t('admin.serviceQuota.limiters.rpmCountOnArrival.label') }}</span>
            <span class="text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuota.limiters.rpmCountOnArrival.help') }}
            </span>
          </span>
        </label>
      </div>
    </div>
    <button
      type="button"
      class="btn btn-secondary"
      :disabled="!canAdd"
      @click="add"
    >
      <Icon name="plus" size="sm" class="mr-2" />
      {{ t('admin.serviceQuota.limiterEditor.add') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import NumericInput from '@/components/common/NumericInput.vue'
import RequiredLabel from '@/components/common/RequiredLabel.vue'
import {
  type ServiceQuotaLimiterInput,
  type TokenComponent,
  TOKEN_COMPONENTS_ALL,
  TOKEN_COMPONENTS_DEFAULT,
  limiterUsesTokenComponents,
} from '@/api/admin/serviceQuota'
import type { ValidationError } from '@/utils/validateServiceQuota'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    modelValue: ServiceQuotaLimiterInput[]
    /** 父组件传入的校验错误（路径与 validateServiceQuotaRule 输出对齐） */
    errors?: ValidationError[]
  }>(),
  { errors: () => [] },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: ServiceQuotaLimiterInput[]): void
}>()

// errorFor(path) 返回该字段第一条 error.code，没有则空字符串
function errorFor(path: string): string {
  return props.errors.find((e) => e.path === path)?.code || ''
}

// daily_usd 是金额（小数）；其他都是整数（rpm/tpm/tpd/concurrency）
function getLimitStep(limiterType: string): number {
  return isDailyUsd(limiterType) ? 0.01 : 1
}

// daily_usd 单独走小数；其他类型业务上必须整数（避免 IEEE754 浮点累积误差）
function isDailyUsd(limiterType: string): boolean {
  return limiterType === 'daily_usd'
}

const allTypes = computed(() => [
  { value: 'rpm', label: t('admin.serviceQuota.limiters.rpm') },
  { value: 'tpm', label: t('admin.serviceQuota.limiters.tpm') },
  { value: 'tpd', label: t('admin.serviceQuota.limiters.tpd') },
  { value: 'daily_usd', label: t('admin.serviceQuota.limiters.dailyUsd') },
  { value: 'concurrency', label: t('admin.serviceQuota.limiters.concurrency') },
])

const usedTypes = computed(() => new Set(props.modelValue.map((l) => l.limiter_type)))

const canAdd = computed(() => usedTypes.value.size < allTypes.value.length)

function availableTypes(currentType: string) {
  return allTypes.value.filter((opt) => opt.value === currentType || !usedTypes.value.has(opt.value))
}

function usesTokenComponents(limiterType: string): boolean {
  return limiterUsesTokenComponents(limiterType)
}

// count_on_arrival 仅对 RPM 有意义；切到其他 limiter 类型时会被 strip 掉。
function usesCountOnArrival(limiterType: string): boolean {
  return limiterType === 'rpm'
}

function defaultLimitFor(type: string): number {
  switch (type) {
    case 'rpm': return 60
    case 'tpm': return 100000
    case 'tpd': return 1000000
    case 'daily_usd': return 50
    case 'concurrency': return 5
    default: return 1
  }
}

function pickFirstAvailableType(): string {
  const used = usedTypes.value
  const next = allTypes.value.find((opt) => !used.has(opt.value))
  return next?.value || 'rpm'
}

function blankLimiter(limiter_type: string): ServiceQuotaLimiterInput {
  const out: ServiceQuotaLimiterInput = {
    uid: crypto.randomUUID(),
    limiter_type,
    window_mode: 'fixed',
    limit_value: defaultLimitFor(limiter_type),
  }
  if (limiterUsesTokenComponents(limiter_type)) {
    out.token_components = [...TOKEN_COMPONENTS_DEFAULT]
  }
  if (limiter_type === 'rpm') {
    out.count_on_arrival = false
  }
  return out
}

function add() {
  const limiter_type = pickFirstAvailableType()
  const newList: ServiceQuotaLimiterInput[] = [
    ...props.modelValue,
    blankLimiter(limiter_type),
  ]
  emit('update:modelValue', newList)
}

function remove(index: number) {
  const newList = props.modelValue.slice()
  newList.splice(index, 1)
  emit('update:modelValue', newList)
}

function updateField<K extends keyof ServiceQuotaLimiterInput>(index: number, key: K, value: ServiceQuotaLimiterInput[K]) {
  const newList = props.modelValue.slice()
  const next: ServiceQuotaLimiterInput = { ...newList[index], [key]: value }
  if (key === 'limiter_type' && value === 'concurrency') {
    next.window_mode = 'fixed'
  }
  // limiter_type 切换时同步 token_components：
  //   - 切到 TPM/TPD 且没有 token_components → 填默认（input/output/cache_creation）
  //   - 切到非 TPM/TPD → 删字段，避免提交垃圾数据（后端会清洗，前端也清干净）
  if (key === 'limiter_type') {
    // value 在此分支对应 ServiceQuotaLimiterInput['limiter_type']（string）。
    // 用 typeof narrow 处理泛型上下文，避免运行时拿到非字符串导致 helper 误判。
    const newType: string = typeof value === 'string' ? value : ''
    if (limiterUsesTokenComponents(newType)) {
      if (!next.token_components || next.token_components.length === 0) {
        next.token_components = [...TOKEN_COMPONENTS_DEFAULT]
      }
    } else {
      delete next.token_components
    }
    // count_on_arrival 同理：仅 RPM 保留；切到其他类型 strip 掉
    if (newType === 'rpm') {
      if (next.count_on_arrival === undefined) {
        next.count_on_arrival = false
      }
    } else {
      delete next.count_on_arrival
    }
    // 切到整数型时归位非整数老值（如老数据 999999.999997 切到 tpm 时 → 1000000）
    if (!isDailyUsd(newType) && typeof next.limit_value === 'number' && !Number.isInteger(next.limit_value)) {
      next.limit_value = Math.round(next.limit_value)
    }
  }
  newList[index] = next
  emit('update:modelValue', newList)
}

// toggleTokenComponent 单个勾选/取消，按 TOKEN_COMPONENTS_ALL 顺序去重，
// 允许暂时为空（让 UI 显示红字提示），父组件提交侧统一拦截。
function toggleTokenComponent(index: number, comp: TokenComponent, checked: boolean) {
  const newList = props.modelValue.slice()
  const current = new Set(newList[index].token_components ?? TOKEN_COMPONENTS_DEFAULT)
  if (checked) {
    current.add(comp)
  } else {
    current.delete(comp)
  }
  // 按 TOKEN_COMPONENTS_ALL 固定顺序输出，便于 diff 与展示稳定
  const ordered = TOKEN_COMPONENTS_ALL.filter((c) => current.has(c))
  newList[index] = { ...newList[index], token_components: ordered }
  emit('update:modelValue', newList)
}
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}
</style>