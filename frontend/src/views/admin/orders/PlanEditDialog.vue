<template>
  <BaseDialog :show="show" :title="plan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="emit('close')">
    <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.planName') REDACTEDREDACTED <span class="text-red-500">*</span></label>
          <input v-model="planForm.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.group') REDACTEDREDACTED <span class="text-red-500">*</span></label>
          <Select v-model="planForm.group_id" :options="groupOptions" :placeholder="t('payment.admin.selectGroup')" class="w-full">
            <template #selected="{ option REDACTED">
              <span v-if="option?.platform" :class="platformTextClass(String(option.platform))">{{ option.label REDACTEDREDACTED</span>
              <span v-else>{{ option?.label || t('payment.admin.selectGroup') REDACTEDREDACTED</span>
            </template>
            <template #option="{ option, selected REDACTED">
              <span class="flex-1 truncate text-left" :class="option.platform ? platformTextClass(String(option.platform)) : ''">{{ option.label REDACTEDREDACTED</span>
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
          <div><span class="text-gray-500">{{ t('payment.admin.dailyLimit') REDACTEDREDACTED:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.daily_limit_usd != null ? '$' + selectedGroupInfo.daily_limit_usd : t('payment.admin.unlimited') REDACTEDREDACTED</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.weeklyLimit') REDACTEDREDACTED:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.weekly_limit_usd != null ? '$' + selectedGroupInfo.weekly_limit_usd : t('payment.admin.unlimited') REDACTEDREDACTED</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.monthlyLimit') REDACTEDREDACTED:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.monthly_limit_usd != null ? '$' + selectedGroupInfo.monthly_limit_usd : t('payment.admin.unlimited') REDACTEDREDACTED</span></div>
        </div>
      </div>

      <div><label class="input-label">{{ t('payment.admin.planDescription') REDACTEDREDACTED <span class="text-red-500">*</span></label><textarea v-model="planForm.description" rows="2" class="input" required></textarea></div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.price') REDACTEDREDACTED <span class="text-red-500">*</span></label>
          <input v-model.number="planForm.price" type="number" step="0.01" min="0.01" class="input" required />
          <p v-if="subscriptionCnyPreview" class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
            {{ t('payment.admin.subscriptionCnyPayPreview', { amount: subscriptionCnyPreview.amount REDACTED) REDACTEDREDACTED
            <span v-if="subscriptionCnyPreview.feeRate > 0">
              {{ t('payment.admin.subscriptionCnyPayPreviewWithFee', { feeRate: subscriptionCnyPreview.feeRate, total: subscriptionCnyPreview.total REDACTED) REDACTEDREDACTED
            </span>
          </p>
        </div>
        <div><label class="input-label">{{ t('payment.admin.originalPrice') REDACTEDREDACTED</label><input v-model.number="planForm.original_price" type="number" step="0.01" min="0" class="input" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.validityDays') REDACTEDREDACTED <span class="text-red-500">*</span></label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.validityUnit') REDACTEDREDACTED <span class="text-red-500">*</span></label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.sortOrder') REDACTEDREDACTED</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
      </div>
      <div>
        <label class="input-label">{{ t('payment.admin.features') REDACTEDREDACTED</label>
        <textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') REDACTEDREDACTED</p>
      </div>
      <div class="flex items-center gap-3">
        <label class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') REDACTEDREDACTED</label>
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
        <button type="button" @click="emit('close')" class="btn btn-secondary">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button type="submit" form="plan-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') REDACTEDREDACTED</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminPaymentAPI REDACTED from '@/api/admin/payment'
import type { AdminPaymentConfig REDACTED from '@/api/admin/payment'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { formatPaymentAmount REDACTED from '@/components/payment/currency'
import type { SubscriptionPlan REDACTED from '@/types/payment'
import type { AdminGroup REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import { platformTextClass REDACTED from '@/utils/platformColors'

const props = defineProps<{
  show: boolean
  plan: SubscriptionPlan | null
  groups: AdminGroup[]
  paymentConfig?: AdminPaymentConfig | null
REDACTED>()

const emit = defineEmits<{
  close: []
  saved: []
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const saving = ref(false)
const planForm = reactive({ name: '', group_id: null as number | null, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', sort_order: 0, for_sale: true REDACTED)
const planFeaturesText = ref('')

const validityUnitOptions = computed(() => [
  { value: 'days', label: t('payment.admin.days') REDACTED,
  { value: 'weeks', label: t('payment.admin.weeks') REDACTED,
  { value: 'months', label: t('payment.admin.months') REDACTED,
])

const groupOptions = computed(() =>
  props.groups
    .filter(g => g.subscription_type === 'subscription')
    .map(g => ({
      value: g.id,
      label: `${g.nameREDACTED — ${g.platformREDACTED (${g.rate_multiplierREDACTEDx)`,
      platform: g.platform,
    REDACTED)),
)

const selectedGroupInfo = computed(() => {
  if (!planForm.group_id) return null
  return props.groups.find(g => g.id === planForm.group_id) || null
REDACTED)

function roundCnyAmount(value: number): number {
  return Math.round(value * 100) / 100
REDACTED

function ceilCnyAmount(value: number): number {
  return Math.ceil(value * 100) / 100
REDACTED

const subscriptionCnyPreview = computed(() => {
  const price = Number(planForm.price) || 0
  const rate = Number(props.paymentConfig?.subscription_usd_to_cny_rate) || 0
  if (price <= 0 || rate <= 0) return null

  const amount = roundCnyAmount(price * rate)
  const feeRate = Number(props.paymentConfig?.recharge_fee_rate) || 0
  const fee = feeRate > 0 ? ceilCnyAmount((amount * feeRate) / 100) : 0
  const total = feeRate > 0 ? roundCnyAmount(amount + fee) : amount

  return {
    amount: formatPaymentAmount(amount, 'CNY'),
    feeRate,
    total: formatPaymentAmount(total, 'CNY'),
  REDACTED
REDACTED)

// Reset form when dialog opens
watch(() => props.show, (visible) => {
  if (!visible) return
  if (props.plan) {
    Object.assign(planForm, { name: props.plan.name, group_id: props.plan.group_id, description: props.plan.description, price: props.plan.price, original_price: props.plan.original_price || 0, validity_days: props.plan.validity_days, validity_unit: props.plan.validity_unit || 'days', sort_order: props.plan.sort_order || 0, for_sale: props.plan.for_sale REDACTED)
    planFeaturesText.value = (props.plan.features || []).join('\n')
  REDACTED else {
    Object.assign(planForm, { name: '', group_id: null, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', sort_order: 0, for_sale: true REDACTED)
    planFeaturesText.value = ''
  REDACTED
REDACTED)

/** Build request payload with snake_case keys matching backend JSON tags */
function buildPlanPayload() {
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  return {
    name: planForm.name,
    group_id: planForm.group_id,
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price || 0,
    validity_days: planForm.validity_days,
    validity_unit: planForm.validity_unit,
    sort_order: planForm.sort_order,
    for_sale: planForm.for_sale,
    features,
  REDACTED
REDACTED

async function handleSavePlan() {
  if (!planForm.group_id) {
    appStore.showError(t('payment.admin.groupRequired'))
    return
  REDACTED
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('payment.admin.priceRequired'))
    return
  REDACTED
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('payment.admin.validityDaysRequired'))
    return
  REDACTED
  saving.value = true
  try {
    const data = buildPlanPayload()
    if (props.plan) { await adminPaymentAPI.updatePlan(props.plan.id, data) REDACTED
    else { await adminPaymentAPI.createPlan(data) REDACTED
    appStore.showSuccess(t('common.saved'))
    emit('close')
    emit('saved')
  REDACTED catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) REDACTED
  finally { saving.value = false REDACTED
REDACTED
</script>
