<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.provision.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.provision.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button class="btn btn-secondary" :disabled="loading" @click="loadAll">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary" @click="openCreateDialog">
            <Icon name="plus" size="sm" class="mr-1.5" />
            {{ t('admin.provision.createPlan') }}
          </button>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[1fr_360px]">
        <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-700/50">
                <tr>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.columns.plan') }}
                  </th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.columns.group') }}
                  </th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.columns.balance') }}
                  </th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.columns.limits') }}
                  </th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.columns.status') }}
                  </th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('common.actions') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-if="loading">
                  <td colspan="6" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('common.loading') }}
                  </td>
                </tr>
                <tr v-else-if="plans.length === 0">
                  <td colspan="6" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.provision.empty') }}
                  </td>
                </tr>
                <tr v-for="plan in plans" :key="plan.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                  <td class="px-4 py-3">
                    <div class="font-medium text-gray-900 dark:text-white">{{ plan.name }}</div>
                    <code class="text-xs text-gray-500 dark:text-gray-400">{{ plan.code }}</code>
                  </td>
                  <td class="px-4 py-3">
                    <div class="text-sm text-gray-900 dark:text-white">
                      {{ plan.group?.name || `#${plan.group_id}` }}
                    </div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">
                      {{ plan.group?.platform || '-' }} · {{ plan.group?.rate_multiplier || 0 }}x
                    </div>
                  </td>
                  <td class="px-4 py-3 text-right text-sm text-gray-900 dark:text-white">
                    {{ formatMoney(plan.balance) }}
                    <div class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.provision.quota') }} {{ formatMoney(plan.quota) }}
                    </div>
                  </td>
                  <td class="px-4 py-3 text-right text-xs text-gray-500 dark:text-gray-400">
                    <div>5h {{ formatMoney(plan.rate_limit_5h) }}</div>
                    <div>1d {{ formatMoney(plan.rate_limit_1d) }}</div>
                    <div>7d {{ formatMoney(plan.rate_limit_7d) }}</div>
                  </td>
                  <td class="px-4 py-3">
                    <span :class="['badge', plan.enabled ? 'badge-success' : 'badge-secondary']">
                      {{ plan.enabled ? t('common.enabled') : t('common.disabled') }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-right">
                    <div class="inline-flex items-center gap-2">
                      <button class="btn btn-sm btn-secondary" :disabled="savingPlan" @click="togglePlan(plan)">
                        <Icon :name="plan.enabled ? 'ban' : 'check'" size="sm" />
                        <span>{{ plan.enabled ? t('admin.provision.disablePlan') : t('admin.provision.enablePlan') }}</span>
                      </button>
                      <button class="btn btn-sm btn-secondary" :title="t('common.edit')" @click="openEditDialog(plan)">
                        <Icon name="edit" size="sm" />
                      </button>
                      <button class="btn btn-sm btn-danger" :title="t('common.delete')" @click="removePlan(plan)">
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.provision.manualTitle') }}
          </h2>
          <div class="mt-4 space-y-4">
            <label class="block">
              <span class="input-label">{{ t('admin.provision.orderId') }}</span>
              <input v-model.trim="provisionForm.order_id" class="input" placeholder="ORDER_001" />
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.provision.plan') }}</span>
              <select v-model="provisionForm.plan_code" class="input">
                <option value="">{{ t('admin.provision.selectPlan') }}</option>
                <option v-for="plan in enabledPlans" :key="plan.id" :value="plan.code">
                  {{ plan.name }} · {{ plan.code }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('admin.provision.customerLabel') }}</span>
              <input v-model.trim="provisionForm.customer_label" class="input" />
            </label>
            <div v-if="selectedProvisionPlan" class="rounded-md bg-gray-50 p-3 text-sm text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ selectedProvisionPlan.group?.name }} · {{ selectedProvisionPlan.group?.rate_multiplier }}x ·
              {{ formatMoney(selectedProvisionPlan.balance) }}
            </div>
            <button class="btn btn-primary w-full" :disabled="provisioning" @click="submitProvision">
              {{ provisioning ? t('admin.provision.provisioning') : t('admin.provision.provisionKey') }}
            </button>
          </div>
        </section>
      </div>
    </div>

    <BaseDialog :show="showPlanDialog" :title="editingPlan ? t('admin.provision.editPlan') : t('admin.provision.createPlan')" @close="showPlanDialog = false">
      <form class="space-y-4" @submit.prevent="savePlan">
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block">
            <span class="input-label">{{ t('admin.provision.code') }}</span>
            <input v-model.trim="planForm.code" class="input" placeholder="codex_04x" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.provision.name') }}</span>
            <input v-model.trim="planForm.name" class="input" />
          </label>
        </div>
        <label class="block">
          <span class="input-label">{{ t('admin.provision.group') }}</span>
          <select v-model.number="planForm.group_id" class="input">
            <option :value="0">{{ t('admin.provision.selectGroup') }}</option>
            <option v-for="group in eligibleGroups" :key="group.id" :value="group.id">
              {{ group.name }} · {{ group.platform }} · {{ group.rate_multiplier }}x · {{ t('admin.provision.exclusive') }}
            </option>
          </select>
        </label>
        <div v-if="selectedPlanGroup" class="rounded-md bg-gray-50 p-3 text-sm text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          {{ selectedPlanGroup.platform }} · {{ selectedPlanGroup.rate_multiplier }}x ·
          {{ selectedPlanGroup.subscription_type }} · {{ selectedPlanGroup.is_exclusive ? t('admin.provision.exclusive') : t('admin.provision.shared') }}
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block">
            <span class="input-label">{{ t('admin.provision.balance') }}</span>
            <input v-model.number="planForm.balance" class="input" type="number" min="0" step="0.000001" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.provision.quota') }}</span>
            <input v-model.number="planForm.quota" class="input" type="number" min="0" step="0.000001" />
          </label>
        </div>
        <div class="grid gap-4 sm:grid-cols-3">
          <label class="block">
            <span class="input-label">5h</span>
            <input v-model.number="planForm.rate_limit_5h" class="input" type="number" min="0" step="0.000001" />
          </label>
          <label class="block">
            <span class="input-label">1d</span>
            <input v-model.number="planForm.rate_limit_1d" class="input" type="number" min="0" step="0.000001" />
          </label>
          <label class="block">
            <span class="input-label">7d</span>
            <input v-model.number="planForm.rate_limit_7d" class="input" type="number" min="0" step="0.000001" />
          </label>
        </div>
        <div class="grid gap-4 sm:grid-cols-3">
          <label class="block">
            <span class="input-label">{{ t('admin.provision.expiresInDays') }}</span>
            <input v-model.number="planForm.expires_in_days" class="input" type="number" min="1" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.provision.concurrency') }}</span>
            <input v-model.number="planForm.concurrency" class="input" type="number" min="1" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.provision.rpmLimit') }}</span>
            <input v-model.number="planForm.rpm_limit" class="input" type="number" min="0" />
          </label>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="planForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('common.enabled') }}
        </label>
        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-secondary" @click="showPlanDialog = false">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="savingPlan">
            {{ savingPlan ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </form>
    </BaseDialog>

    <BaseDialog :show="!!provisionResult" :title="t('admin.provision.resultTitle')" @close="provisionResult = null">
      <div v-if="provisionResult" class="space-y-4">
        <div class="rounded-md bg-gray-50 p-3 dark:bg-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('nav.apiKeys') }}</div>
          <div class="mt-1 break-all font-mono text-sm text-gray-900 dark:text-white">{{ provisionResult.api_key }}</div>
        </div>
        <div class="grid grid-cols-2 gap-3 text-sm text-gray-600 dark:text-gray-300">
          <div>{{ t('admin.provision.orderId') }}: {{ provisionResult.order_id }}</div>
          <div>{{ t('admin.provision.plan') }}: {{ provisionResult.plan_code }}</div>
          <div>{{ t('admin.provision.balance') }}: {{ formatMoney(provisionResult.balance) }}</div>
          <div>{{ t('admin.provision.rate') }}: {{ provisionResult.rate_multiplier }}x</div>
        </div>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="copyKey(provisionResult.api_key)">
            <Icon name="copy" size="sm" class="mr-1.5" />
            {{ t('keys.copyToClipboard') }}
          </button>
          <button class="btn btn-primary" @click="provisionResult = null">
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ProvisionPlan, ProvisionPlanRequest, ProvisionResult } from '@/api/admin/provision'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const plans = ref<ProvisionPlan[]>([])
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const savingPlan = ref(false)
const provisioning = ref(false)
const showPlanDialog = ref(false)
const editingPlan = ref<ProvisionPlan | null>(null)
const provisionResult = ref<ProvisionResult | null>(null)

const planForm = reactive<ProvisionPlanRequest>({
  code: '',
  name: '',
  group_id: 0,
  balance: 0,
  quota: 0,
  expires_in_days: null,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  concurrency: 5,
  rpm_limit: 0,
  enabled: true
})

const provisionForm = reactive({
  order_id: '',
  plan_code: '',
  customer_label: ''
})

const eligibleGroups = computed(() =>
  groups.value.filter((group) => group.status === 'active' && group.subscription_type === 'standard' && group.is_exclusive)
)

const enabledPlans = computed(() => plans.value.filter((plan) => plan.enabled))

const selectedProvisionPlan = computed(() =>
  plans.value.find((plan) => plan.code === provisionForm.plan_code) || null
)

const selectedPlanGroup = computed(() =>
  groups.value.find((group) => group.id === planForm.group_id) || null
)

function formatMoney(value: number): string {
  return Number(value || 0).toFixed(2)
}

function resetPlanForm() {
  Object.assign(planForm, {
    code: '',
    name: '',
    group_id: 0,
    balance: 0,
    quota: 0,
    expires_in_days: null,
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    concurrency: 5,
    rpm_limit: 0,
    enabled: true
  })
}

async function loadAll() {
  loading.value = true
  try {
    const [nextPlans, nextGroups] = await Promise.all([
      adminAPI.provision.listPlans(),
      adminAPI.groups.getAll()
    ])
    plans.value = nextPlans
    groups.value = nextGroups
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.provision.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  editingPlan.value = null
  resetPlanForm()
  showPlanDialog.value = true
}

function openEditDialog(plan: ProvisionPlan) {
  editingPlan.value = plan
  Object.assign(planForm, {
    code: plan.code,
    name: plan.name,
    group_id: plan.group_id,
    balance: plan.balance,
    quota: plan.quota,
    expires_in_days: plan.expires_in_days ?? null,
    rate_limit_5h: plan.rate_limit_5h,
    rate_limit_1d: plan.rate_limit_1d,
    rate_limit_7d: plan.rate_limit_7d,
    concurrency: plan.concurrency,
    rpm_limit: plan.rpm_limit,
    enabled: plan.enabled
  })
  showPlanDialog.value = true
}

function normalizePlanPayload(): ProvisionPlanRequest {
  return {
    ...planForm,
    expires_in_days: planForm.expires_in_days || null,
    quota: planForm.quota || planForm.balance
  }
}

async function savePlan() {
  savingPlan.value = true
  try {
    const payload = normalizePlanPayload()
    if (editingPlan.value) {
      await adminAPI.provision.updatePlan(editingPlan.value.id, payload)
    } else {
      await adminAPI.provision.createPlan(payload)
    }
    appStore.showSuccess(t('admin.provision.saved'))
    showPlanDialog.value = false
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.provision.saveFailed'))
  } finally {
    savingPlan.value = false
  }
}

async function removePlan(plan: ProvisionPlan) {
  if (!window.confirm(t('admin.provision.deleteConfirm', { name: plan.name }))) {
    return
  }
  try {
    await adminAPI.provision.deletePlan(plan.id)
    appStore.showSuccess(t('admin.provision.deleted'))
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.provision.deleteFailed'))
  }
}

async function togglePlan(plan: ProvisionPlan) {
  savingPlan.value = true
  try {
    await adminAPI.provision.updatePlan(plan.id, {
      code: plan.code,
      name: plan.name,
      group_id: plan.group_id,
      balance: plan.balance,
      quota: plan.quota,
      expires_in_days: plan.expires_in_days ?? null,
      rate_limit_5h: plan.rate_limit_5h,
      rate_limit_1d: plan.rate_limit_1d,
      rate_limit_7d: plan.rate_limit_7d,
      concurrency: plan.concurrency,
      rpm_limit: plan.rpm_limit,
      enabled: !plan.enabled
    })
    appStore.showSuccess(t('admin.provision.saved'))
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.provision.saveFailed'))
  } finally {
    savingPlan.value = false
  }
}

async function submitProvision() {
  if (!provisionForm.order_id || !provisionForm.plan_code) {
    appStore.showError(t('admin.provision.formIncomplete'))
    return
  }
  provisioning.value = true
  try {
    provisionResult.value = await adminAPI.provision.provisionAPIKey({ ...provisionForm })
    appStore.showSuccess(t('admin.provision.provisioned'))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.provision.provisionFailed'))
  } finally {
    provisioning.value = false
  }
}

function copyKey(key: string) {
  copyToClipboard(key, t('admin.provision.keyCopied'))
}

onMounted(loadAll)
</script>
