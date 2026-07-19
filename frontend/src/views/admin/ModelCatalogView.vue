<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1600px] space-y-5 px-4 py-5 sm:px-6 lg:px-8">
      <header class="flex flex-col gap-4 rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.modelCatalog.title') }}</h1>
            <span v-if="catalog?.initialized" class="badge badge-success">{{ t('admin.modelCatalog.activeRevision', { revision: catalog.revision }) }}</span>
            <span v-else class="badge badge-warning">{{ t('admin.modelCatalog.notInitialized') }}</span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.description') }}</p>
          <div v-if="catalog?.initialized" class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
            <span>{{ t('admin.modelCatalog.epoch') }}: <strong class="text-gray-700 dark:text-dark-200">{{ catalog.epoch }}</strong></span>
            <span>{{ t('admin.modelCatalog.checksum') }}: <code class="font-mono">{{ shortHash(catalog.checksum) }}</code></span>
            <span>{{ t('admin.modelCatalog.publishedAt') }}: {{ formatDate(catalog.published_at) }}</span>
          </div>
        </div>
        <div class="flex shrink-0 gap-2">
          <button class="btn btn-secondary" :disabled="loading" @click="loadCatalog">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
          <button class="btn btn-primary" :disabled="syncing" @click="handleSync">
            <Icon name="refresh" size="sm" :class="syncing ? 'animate-spin' : ''" />
            {{ syncing ? t('admin.modelCatalog.syncing') : t('admin.modelCatalog.sync') }}
          </button>
        </div>
      </header>

      <section v-if="catalog?.initialized" class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.totalModels') }}</p>
          <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ catalog.models.length }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.enabledModels') }}</p>
          <p class="mt-1 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">{{ enabledCount }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.pricingReadMode') }}</p>
          <p class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ catalog.pricing_read_mode || '-' }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.admissionMode') }}</p>
          <p class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ catalog.admission_mode || '-' }}</p>
        </div>
      </section>

      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 border-b border-gray-200 p-4 dark:border-dark-700 sm:flex-row sm:items-center">
          <input v-model.trim="search" class="input sm:max-w-sm" type="search" :placeholder="t('admin.modelCatalog.searchPlaceholder')" />
          <select v-model="stateFilter" class="input sm:w-44">
            <option value="all">{{ t('admin.modelCatalog.allStates') }}</option>
            <option value="enabled">{{ t('admin.modelCatalog.state.enabled') }}</option>
            <option value="disabled">{{ t('admin.modelCatalog.state.disabled') }}</option>
            <option value="retired">{{ t('admin.modelCatalog.state.retired') }}</option>
          </select>
          <span class="text-xs text-gray-500 dark:text-dark-400 sm:ml-auto">{{ t('admin.modelCatalog.filteredCount', { count: filteredModels.length }) }}</span>
        </div>

        <div v-if="loading" class="flex min-h-64 items-center justify-center text-gray-500">
          <Icon name="refresh" size="lg" class="animate-spin" />
        </div>
        <div v-else-if="!catalog?.initialized" class="px-6 py-16 text-center">
          <p class="font-medium text-gray-900 dark:text-white">{{ t('admin.modelCatalog.emptyTitle') }}</p>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.emptyDescription') }}</p>
        </div>
        <div v-else-if="filteredModels.length === 0" class="px-6 py-16 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.modelCatalog.noResults') }}
        </div>

        <div v-else>
          <div class="hidden overflow-x-auto md:block">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-850 dark:text-dark-400">
                <tr>
                  <th class="px-4 py-3">{{ t('admin.modelCatalog.model') }}</th>
                  <th class="px-4 py-3">{{ t('admin.modelCatalog.stateLabel') }}</th>
                  <th class="px-4 py-3">{{ t('admin.modelCatalog.inputPrice') }}</th>
                  <th class="px-4 py-3">{{ t('admin.modelCatalog.outputPrice') }}</th>
                  <th class="px-4 py-3">{{ t('admin.modelCatalog.source') }}</th>
                  <th class="px-4 py-3 text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="model in pagedModels" :key="model.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-750">
                  <td class="px-4 py-3">
                    <p class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ model.canonical_key }}</p>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ model.provider || '-' }} · {{ model.mode || '-' }}</p>
                  </td>
                  <td class="px-4 py-3">
                    <div class="flex items-center gap-2">
                      <ToggleSwitch
                        :checked="model.operator_state === 'enabled'"
                        :disabled="model.operator_state === 'retired'"
                        :loading="mutatingModelID === model.id"
                        :aria-label="t('admin.modelCatalog.toggleAria', { model: model.canonical_key })"
                        @toggle="openStateDialog(model)"
                      />
                      <span :class="stateBadge(model.operator_state)">{{ t(`admin.modelCatalog.state.${model.operator_state}`) }}</span>
                    </div>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-dark-200">{{ formatPerMillion(model.pricing?.input_cost_per_token) }}</td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-dark-200">{{ formatPerMillion(model.pricing?.output_cost_per_token) }}</td>
                  <td class="px-4 py-3">
                    <p class="text-sm text-gray-700 dark:text-dark-200">{{ model.pricing_source || '-' }}</p>
                    <p class="font-mono text-xs text-gray-400">{{ shortHash(model.source_hash) }}</p>
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button class="btn btn-secondary btn-sm" :disabled="model.operator_state === 'retired'" @click="openPricingDialog(model)">
                      <Icon name="edit" size="sm" />
                      {{ t('admin.modelCatalog.editPricing') }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="divide-y divide-gray-100 dark:divide-dark-700 md:hidden">
            <article v-for="model in pagedModels" :key="model.id" class="space-y-3 p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ model.canonical_key }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ model.provider || '-' }} · {{ model.mode || '-' }}</p>
                </div>
                <ToggleSwitch
                  :checked="model.operator_state === 'enabled'"
                  :disabled="model.operator_state === 'retired'"
                  :loading="mutatingModelID === model.id"
                  :aria-label="t('admin.modelCatalog.toggleAria', { model: model.canonical_key })"
                  @toggle="openStateDialog(model)"
                />
              </div>
              <div class="grid grid-cols-2 gap-3 rounded-xl bg-gray-50 p-3 text-xs dark:bg-dark-850">
                <div><p class="text-gray-500">{{ t('admin.modelCatalog.inputPrice') }}</p><p class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatPerMillion(model.pricing?.input_cost_per_token) }}</p></div>
                <div><p class="text-gray-500">{{ t('admin.modelCatalog.outputPrice') }}</p><p class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatPerMillion(model.pricing?.output_cost_per_token) }}</p></div>
              </div>
              <button class="btn btn-secondary w-full" :disabled="model.operator_state === 'retired'" @click="openPricingDialog(model)">
                <Icon name="edit" size="sm" /> {{ t('admin.modelCatalog.editPricing') }}
              </button>
            </article>
          </div>

          <div class="flex items-center justify-between border-t border-gray-200 px-4 py-3 text-sm dark:border-dark-700">
            <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="page--">{{ t('common.previous') }}</button>
            <span class="text-gray-500 dark:text-dark-400">{{ t('admin.modelCatalog.pageOf', { page, pages: totalPages }) }}</span>
            <button class="btn btn-secondary btn-sm" :disabled="page >= totalPages" @click="page++">{{ t('common.next') }}</button>
          </div>
        </div>
      </section>
    </div>

    <BaseDialog :show="stateDialogOpen" :title="t('admin.modelCatalog.stateDialogTitle')" width="narrow" @close="closeStateDialog">
      <div v-if="selectedModel" class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-dark-300">
          {{ t('admin.modelCatalog.stateDialogDescription', { model: selectedModel.canonical_key, state: t(`admin.modelCatalog.state.${pendingState}`) }) }}
        </p>
        <div>
          <label class="input-label">{{ t('admin.modelCatalog.reason') }}</label>
          <textarea v-model.trim="stateReason" class="input min-h-24" maxlength="500" :placeholder="t('admin.modelCatalog.reasonPlaceholder')" />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="closeStateDialog">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="!stateReason || saving" @click="confirmStateChange">{{ t('common.confirm') }}</button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="pricingDialogOpen" :title="t('admin.modelCatalog.pricingDialogTitle')" width="wide" @close="closePricingDialog">
      <form v-if="selectedModel" id="model-pricing-form" class="space-y-5" @submit.prevent="confirmPricingChange">
        <div class="rounded-xl bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t('admin.modelCatalog.pricingWarning') }}</div>
        <p class="break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ selectedModel.canonical_key }}</p>
        <div class="grid gap-4 sm:grid-cols-2">
          <label v-for="field in pricingFields" :key="field.key" class="block">
            <span class="input-label">{{ t(field.label) }}</span>
            <input v-model.number="pricingForm[field.key]" class="input font-mono" type="number" min="0" step="any" required />
            <span class="mt-1 block text-xs text-gray-500">{{ formatPerMillion(pricingForm[field.key]) }}</span>
          </label>
        </div>
        <div>
          <label class="input-label">{{ t('admin.modelCatalog.reason') }}</label>
          <textarea v-model.trim="pricingReason" class="input min-h-24" maxlength="500" required :placeholder="t('admin.modelCatalog.reasonPlaceholder')" />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="closePricingDialog">{{ t('common.cancel') }}</button>
          <button form="model-pricing-form" class="btn btn-primary" type="submit" :disabled="!pricingReason || saving">{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ToggleSwitch from '@/components/common/ToggleSwitch.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { adminAPI } from '@/api'
import type { CatalogAdminModel, CatalogAdminStatus, CatalogOperatorState } from '@/api/admin/modelCatalog'
import { useAppStore } from '@/stores/app'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'

const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const catalog = ref<CatalogAdminStatus | null>(null)
const loading = ref(false)
const syncing = ref(false)
const saving = ref(false)
const mutatingModelID = ref<number | null>(null)
const search = ref('')
const stateFilter = ref<'all' | CatalogOperatorState>('all')
const page = ref(1)
const pageSize = 50
const selectedModel = ref<CatalogAdminModel | null>(null)
const stateDialogOpen = ref(false)
const pricingDialogOpen = ref(false)
const pendingState = ref<'enabled' | 'disabled'>('disabled')
const stateReason = ref('')
const pricingReason = ref('')
const pricingForm = reactive({
  input_cost_per_token: 0,
  output_cost_per_token: 0,
  cache_creation_input_token_cost: 0,
  cache_read_input_token_cost: 0
})

const pricingFields = [
  { key: 'input_cost_per_token', label: 'admin.modelCatalog.inputPricePerToken' },
  { key: 'output_cost_per_token', label: 'admin.modelCatalog.outputPricePerToken' },
  { key: 'cache_creation_input_token_cost', label: 'admin.modelCatalog.cacheWritePricePerToken' },
  { key: 'cache_read_input_token_cost', label: 'admin.modelCatalog.cacheReadPricePerToken' }
] as const

const enabledCount = computed(() => catalog.value?.models.filter((model) => model.operator_state === 'enabled').length ?? 0)
const filteredModels = computed(() => {
  const term = search.value.toLowerCase()
  return (catalog.value?.models ?? []).filter((model) => {
    const matchesSearch = !term || `${model.canonical_key} ${model.provider} ${model.platform} ${model.mode}`.toLowerCase().includes(term)
    const matchesState = stateFilter.value === 'all' || model.operator_state === stateFilter.value
    return matchesSearch && matchesState
  })
})
const totalPages = computed(() => Math.max(1, Math.ceil(filteredModels.value.length / pageSize)))
const pagedModels = computed(() => filteredModels.value.slice((page.value - 1) * pageSize, page.value * pageSize))
watch([search, stateFilter], () => { page.value = 1 })
watch(totalPages, (pages) => { if (page.value > pages) page.value = pages })

async function loadCatalog() {
  loading.value = true
  try {
    catalog.value = await adminAPI.modelCatalog.getModelCatalog()
  } catch (error) {
    console.error('Failed to load model catalog', error)
    appStore.showError(t('admin.modelCatalog.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function handleSync() {
  syncing.value = true
  const key = adminAPI.modelCatalog.createModelCatalogIdempotencyKey('sync')
  try {
    catalog.value = await stepUp.run(() => adminAPI.modelCatalog.syncModelCatalog(key))
    appStore.showSuccess(t('admin.modelCatalog.syncSuccess'))
  } catch (error) {
    if (!isStepUpCancelled(error)) {
      console.error('Failed to sync model catalog', error)
      appStore.showError(t('admin.modelCatalog.syncFailed'))
    }
  } finally {
    syncing.value = false
  }
}

function openStateDialog(model: CatalogAdminModel) {
  if (model.operator_state === 'retired') return
  selectedModel.value = model
  pendingState.value = model.operator_state === 'enabled' ? 'disabled' : 'enabled'
  stateReason.value = ''
  stateDialogOpen.value = true
}
function closeStateDialog() { stateDialogOpen.value = false; selectedModel.value = null }

async function confirmStateChange() {
  const model = selectedModel.value
  if (!model || !stateReason.value) return
  saving.value = true
  mutatingModelID.value = model.id
  const key = adminAPI.modelCatalog.createModelCatalogIdempotencyKey('state', model.id)
  try {
    const result = await stepUp.run(() => adminAPI.modelCatalog.updateModelCatalogState(model.id, {
      expected_version: model.operator_version,
      state: pendingState.value,
      reason: stateReason.value
    }, key))
    catalog.value = result.snapshot
    closeStateDialog()
    appStore.showSuccess(t('admin.modelCatalog.stateSaved'))
  } catch (error) {
    if (!isStepUpCancelled(error)) {
      console.error('Failed to update catalog state', error)
      appStore.showError(t('admin.modelCatalog.mutationFailed'))
      await loadCatalog()
    }
  } finally {
    saving.value = false
    mutatingModelID.value = null
  }
}

function openPricingDialog(model: CatalogAdminModel) {
  selectedModel.value = model
  pricingForm.input_cost_per_token = Number(model.pricing?.input_cost_per_token ?? 0)
  pricingForm.output_cost_per_token = Number(model.pricing?.output_cost_per_token ?? 0)
  pricingForm.cache_creation_input_token_cost = Number(model.pricing?.cache_creation_input_token_cost ?? 0)
  pricingForm.cache_read_input_token_cost = Number(model.pricing?.cache_read_input_token_cost ?? 0)
  pricingReason.value = ''
  pricingDialogOpen.value = true
}
function closePricingDialog() { pricingDialogOpen.value = false; selectedModel.value = null }

async function confirmPricingChange() {
  const model = selectedModel.value
  const snapshot = catalog.value
  if (!model || !snapshot || !pricingReason.value) return
  saving.value = true
  mutatingModelID.value = model.id
  const key = adminAPI.modelCatalog.createModelCatalogIdempotencyKey('pricing', model.id)
  try {
    const result = await stepUp.run(() => adminAPI.modelCatalog.updateModelCatalogPricing(model.id, {
      expected_epoch: snapshot.epoch,
      expected_revision_id: snapshot.revision_id,
      expected_source_hash: model.source_hash,
      input_cost_per_token: pricingForm.input_cost_per_token,
      output_cost_per_token: pricingForm.output_cost_per_token,
      cache_creation_input_token_cost: pricingForm.cache_creation_input_token_cost,
      cache_read_input_token_cost: pricingForm.cache_read_input_token_cost,
      reason: pricingReason.value
    }, key))
    catalog.value = result.snapshot
    closePricingDialog()
    appStore.showSuccess(t('admin.modelCatalog.pricingSaved'))
  } catch (error) {
    if (!isStepUpCancelled(error)) {
      console.error('Failed to update catalog pricing', error)
      appStore.showError(t('admin.modelCatalog.mutationFailed'))
      await loadCatalog()
    }
  } finally {
    saving.value = false
    mutatingModelID.value = null
  }
}

function shortHash(value: string) { return value ? `${value.slice(0, 10)}…${value.slice(-6)}` : '-' }
function formatDate(value: string) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }
function formatPerMillion(value: number | null | undefined) { return `$${(Number(value || 0) * 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 6 })} / 1M` }
function stateBadge(state: CatalogOperatorState) {
  if (state === 'enabled') return 'badge badge-success'
  if (state === 'disabled') return 'badge badge-warning'
  return 'badge badge-secondary'
}

onMounted(loadCatalog)
</script>
