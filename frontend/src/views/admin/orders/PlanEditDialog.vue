<template>
  <BaseDialog :show="show" :title="plan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="emit('close')">
    <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.planName') }} <span class="text-red-500">*</span></label>
          <input v-model="planForm.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.group') }} <span class="text-red-500">*</span></label>
          <Select v-model="planForm.group_id" :options="groupOptions" :placeholder="t('payment.admin.selectGroup')" class="w-full">
            <template #selected="{ option }">
              <span v-if="option?.platform" :class="platformTextClass(String(option.platform))">{{ option.label }}</span>
              <span v-else>{{ option?.label || t('payment.admin.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <span class="flex-1 truncate text-left" :class="option.platform ? platformTextClass(String(option.platform)) : ''">{{ option.label }}</span>
              <span v-if="option.subscriptionType" class="ml-2 shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ option.subscriptionTypeLabel }}</span>
              <Icon v-if="selected" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
            </template>
          </Select>
        </div>
      </div>

      <!-- Group Info Preview -->
      <div v-if="selectedGroupInfo" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-2 flex items-center gap-2">
          <GroupBadge :name="selectedGroupInfo.name" :platform="selectedGroupInfo.platform" :rate-multiplier="selectedGroupInfo.rate_multiplier" />
        </div>
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div><span class="text-gray-500">{{ t('payment.admin.dailyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.daily_limit_usd != null ? '$' + selectedGroupInfo.daily_limit_usd : t('payment.admin.unlimited') }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.weeklyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.weekly_limit_usd != null ? '$' + selectedGroupInfo.weekly_limit_usd : t('payment.admin.unlimited') }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.monthlyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.monthly_limit_usd != null ? '$' + selectedGroupInfo.monthly_limit_usd : t('payment.admin.unlimited') }}</span></div>
        </div>
      </div>

      <div><label class="input-label">{{ t('payment.admin.planDescription') }} <span class="text-red-500">*</span></label><textarea v-model="planForm.description" rows="2" class="input" required></textarea></div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.price') }} <span class="text-red-500">*</span></label><input v-model.number="planForm.price" type="number" step="0.01" min="0.01" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.originalPrice') }}</label><input v-model.number="planForm.original_price" type="number" step="0.01" min="0" class="input" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.validityDays') }} <span class="text-red-500">*</span></label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.validityUnit') }} <span class="text-red-500">*</span></label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.sortOrder') }}</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
      </div>
      <div>
        <label class="input-label">{{ t('payment.admin.features') }}</label>
        <textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') }}</p>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <div class="flex items-center justify-between gap-3">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('payment.admin.externalSubscribe') }}</label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.externalSubscribeHint') }}</p>
          </div>
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              planForm.external_subscribe_enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
            ]"
            @click="planForm.external_subscribe_enabled = !planForm.external_subscribe_enabled"
          >
            <span :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              planForm.external_subscribe_enabled ? 'translate-x-5' : 'translate-x-0'
            ]" />
          </button>
        </div>
        <div class="mt-3">
          <label class="input-label">{{ t('payment.admin.externalSubscribeTargetType') }}</label>
          <div class="grid grid-cols-1 gap-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-800 sm:grid-cols-2">
            <button
              type="button"
              class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
              :class="externalSubscribeTargetType === 'url'
                ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'"
              @click="setExternalSubscribeTargetType('url')"
            >
              {{ t('payment.admin.externalSubscribeTargetUrl') }}
            </button>
            <button
              type="button"
              class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
              :class="externalSubscribeTargetType === 'dialog'
                ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'"
              @click="setExternalSubscribeTargetType('dialog')"
            >
              {{ t('payment.admin.externalSubscribeTargetDialog') }}
            </button>
          </div>
        </div>
        <div v-if="externalSubscribeTargetType === 'url'" class="mt-3">
          <label class="input-label">{{ t('payment.admin.externalSubscribeUrl') }}</label>
          <input v-model.trim="planForm.external_subscribe_url" type="url" class="input" :placeholder="t('payment.admin.externalSubscribeUrlPlaceholder')" />
        </div>
        <div v-else class="mt-3">
          <label class="input-label">{{ t('payment.admin.externalSubscribeDialogText') }}</label>
          <textarea
            v-model.trim="planForm.external_subscribe_dialog_text"
            rows="4"
            class="input"
            :placeholder="t('payment.admin.externalSubscribeDialogTextPlaceholder')"
          ></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.externalSubscribeDialogTextHint') }}</p>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <label class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label>
        <button
          type="button"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            planForm.for_sale ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
          ]"
          @click="planForm.for_sale = !planForm.for_sale"
        >
          <span :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            planForm.for_sale ? 'translate-x-5' : 'translate-x-0'
          ]" />
        </button>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="plan-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { SubscriptionPlan } from '@/types/payment'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import { platformTextClass } from '@/utils/platformColors'

const props = defineProps<{
  show: boolean
  plan: SubscriptionPlan | null
  groups: AdminGroup[]
  initialExternalSubscribeEnabled?: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: [plan: SubscriptionPlan]
}>()

const { t } = useI18n()
const appStore = useAppStore()

const saving = ref(false)
const planForm = reactive({ name: '', group_id: null as number | null, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', sort_order: 0, external_subscribe_enabled: false, external_subscribe_url: '', external_subscribe_dialog_text: '', for_sale: true })
const planFeaturesText = ref('')
const externalSubscribeTargetType = ref<'url' | 'dialog'>('url')

const validityUnitOptions = computed(() => [
  { value: 'days', label: t('payment.admin.days') },
  { value: 'weeks', label: t('payment.admin.weeks') },
  { value: 'months', label: t('payment.admin.months') },
])

const groupOptions = computed(() =>
  props.groups
    .map(g => ({
      value: g.id,
      label: `${g.name} — ${g.platform} (${g.rate_multiplier}x)`,
      platform: g.platform,
      subscriptionType: g.subscription_type,
      subscriptionTypeLabel: g.subscription_type === 'subscription'
        ? t('admin.groups.subscription.subscription')
        : t('admin.groups.subscription.standard'),
    })),
)

const selectedGroupInfo = computed(() => {
  if (!planForm.group_id) return null
  return props.groups.find(g => g.id === planForm.group_id) || null
})

function inferExternalSubscribeTargetType(plan: SubscriptionPlan | null): 'url' | 'dialog' {
  if (plan?.external_subscribe_dialog_text?.trim()) return 'dialog'
  return 'url'
}

function setExternalSubscribeTargetType(type: 'url' | 'dialog') {
  externalSubscribeTargetType.value = type
  if (type === 'url') {
    planForm.external_subscribe_dialog_text = ''
    return
  }
  planForm.external_subscribe_url = ''
}

// Reset form when dialog opens
watch(() => props.show, (visible) => {
  if (!visible) return
  if (props.plan) {
    Object.assign(planForm, { name: props.plan.name, group_id: props.plan.group_id, description: props.plan.description, price: props.plan.price, original_price: props.plan.original_price || 0, validity_days: props.plan.validity_days, validity_unit: props.plan.validity_unit || 'days', sort_order: props.plan.sort_order || 0, external_subscribe_enabled: props.initialExternalSubscribeEnabled ?? props.plan.external_subscribe_enabled === true, external_subscribe_url: props.plan.external_subscribe_url || '', external_subscribe_dialog_text: props.plan.external_subscribe_dialog_text || '', for_sale: props.plan.for_sale })
    externalSubscribeTargetType.value = inferExternalSubscribeTargetType(props.plan)
    planFeaturesText.value = (props.plan.features || []).join('\n')
  } else {
    Object.assign(planForm, { name: '', group_id: null, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', sort_order: 0, external_subscribe_enabled: false, external_subscribe_url: '', external_subscribe_dialog_text: '', for_sale: true })
    externalSubscribeTargetType.value = 'url'
    planFeaturesText.value = ''
  }
})

/** Build request payload with snake_case keys matching backend JSON tags */
function normalizeExternalSubscribeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  if (/^[a-z][a-z\d+\-.]*:\/\//i.test(trimmed)) return trimmed
  return `https://${trimmed}`
}

function buildPlanPayload() {
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  const useDialogTarget = externalSubscribeTargetType.value === 'dialog'
  const externalSubscribeUrl = !useDialogTarget
    ? normalizeExternalSubscribeUrl(planForm.external_subscribe_url)
    : ''
  const externalSubscribeDialogText = useDialogTarget
    ? planForm.external_subscribe_dialog_text.trim()
    : ''
  return {
    name: planForm.name,
    group_id: planForm.group_id,
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price || 0,
    validity_days: planForm.validity_days,
    validity_unit: planForm.validity_unit,
    sort_order: planForm.sort_order,
    external_subscribe_enabled: planForm.external_subscribe_enabled,
    external_subscribe_url: externalSubscribeUrl,
    external_subscribe_dialog_text: externalSubscribeDialogText,
    for_sale: planForm.for_sale,
    features,
  }
}

async function handleSavePlan() {
  if (!planForm.group_id) {
    appStore.showError(t('payment.admin.groupRequired'))
    return
  }
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('payment.admin.priceRequired'))
    return
  }
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('payment.admin.validityDaysRequired'))
    return
  }
  if (
    planForm.external_subscribe_enabled &&
    externalSubscribeTargetType.value === 'url' &&
    !normalizeExternalSubscribeUrl(planForm.external_subscribe_url)
  ) {
    appStore.showError(t('payment.admin.externalSubscribeUrlRequired'))
    return
  }
  if (
    planForm.external_subscribe_enabled &&
    externalSubscribeTargetType.value === 'dialog' &&
    !planForm.external_subscribe_dialog_text.trim()
  ) {
    appStore.showError(t('payment.admin.externalSubscribeDialogTextRequired'))
    return
  }
  saving.value = true
  try {
    const data = buildPlanPayload()
    const response = props.plan
      ? await adminPaymentAPI.updatePlan(props.plan.id, data)
      : await adminPaymentAPI.createPlan(data)
    appStore.showSuccess(t('common.saved'))
    emit('saved', response.data)
    emit('close')
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { saving.value = false }
}
</script>
