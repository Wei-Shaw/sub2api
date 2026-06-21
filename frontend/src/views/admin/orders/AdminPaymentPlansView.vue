<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Actions -->
      <div class="flex items-center justify-end gap-2">
        <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
          <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
        </button>
        <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') }}</button>
      </div>

      <!-- Plans Table -->
      <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
        <template #cell-name="{ value, row }">
          <span class="text-sm font-medium" :class="getPlanNameClass(row.group_id)">{{ value }}</span>
        </template>
        <template #cell-group_id="{ value }">
          <span v-if="isGroupMissing(value)" class="text-sm">
            <span class="text-gray-400">#{{ value }}</span>
            <span class="ml-1 badge badge-danger">{{ t('payment.admin.groupMissing') }}</span>
          </span>
          <GroupBadge
            v-else-if="getGroup(value)"
            :name="getGroup(value)!.name"
            :platform="getGroup(value)!.platform"
            :rate-multiplier="getGroup(value)!.rate_multiplier"
          />
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-price="{ value, row }">
          <div class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">${{ (value ?? 0).toFixed(2) }}</span>
            <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">${{ row.original_price.toFixed(2) }}</span>
          </div>
        </template>
        <template #cell-validity_days="{ value, row }">
          <span class="text-sm">{{ value }} {{ t('payment.admin.' + (row.validity_unit || 'days')) }}</span>
        </template>
        <template #cell-for_sale="{ value, row }">
          <button
            type="button"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              value ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
            ]"
            @click="toggleForSale(row)"
          >
            <span :class="[
              'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              value ? 'translate-x-4' : 'translate-x-0'
            ]" />
          </button>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-2">
            <button @click="openPlanEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400">
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t('common.edit') }}</span>
            </button>
            <button @click="confirmDeletePlan(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('common.delete') }}</span>
            </button>
          </div>
        </template>
      </DataTable>

      <div class="card p-4">
        <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">首页商品规格设置</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">按 sub2apipay 原版方式维护商品卡片、详情、图片、价格、slug 与支付类型。</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <a class="btn btn-secondary" href="/payment" target="_blank" rel="noopener noreferrer">打开支付首页</a>
            <button class="btn btn-secondary" :disabled="productsLoading" @click="loadProducts">刷新</button>
            <button class="btn btn-secondary" :disabled="productsLoading" @click="addProduct">新增商品</button>
            <button class="btn btn-primary" :disabled="productsSaving || products.length === 0" @click="saveProducts">保存商品设置</button>
          </div>
        </div>

        <div class="grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
          <aside class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
            <div class="mb-3 flex items-center justify-between">
              <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">商品列表</h4>
              <button class="rounded-xl border border-gray-200 bg-white px-3 py-1 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" @click="jsonOpen = !jsonOpen">
                {{ jsonOpen ? '关闭 JSON' : '打开 JSON' }}
              </button>
            </div>

            <div class="space-y-2">
              <button
                v-for="(product, index) in products"
                :key="`${product.slug || 'new'}-${index}`"
                type="button"
                :class="productListButtonClass(index)"
                @click="selectProduct(index)"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="min-w-0">
                    <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ product.title || '未命名商品' }}</div>
                    <div class="truncate text-xs text-gray-500 dark:text-gray-400">{{ product.slug || '未设置 slug' }}</div>
                  </div>
                  <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[11px]', product.active ? 'bg-emerald-100 text-emerald-700' : 'bg-gray-200 text-gray-500 dark:bg-dark-600 dark:text-gray-400']">
                    {{ product.active ? '启用' : '停用' }}
                  </span>
                </div>
              </button>
            </div>

            <div v-if="jsonOpen" class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-600">
              <textarea v-model="jsonDraft" class="min-h-[280px] w-full rounded-2xl border border-gray-200 bg-white p-3 font-mono text-xs text-gray-900 outline-none dark:border-dark-600 dark:bg-dark-900 dark:text-gray-100" />
              <div class="mt-3 flex gap-3">
                <button class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" @click="applyJsonDraft">应用 JSON</button>
                <button class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" @click="resetJsonDraft">重置 JSON</button>
              </div>
            </div>
          </aside>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-600 dark:bg-dark-800/40">
            <div v-if="selectedProduct" class="space-y-6">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h4 class="text-xl font-semibold text-gray-900 dark:text-white">{{ selectedProduct.title || '未命名商品' }}</h4>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">当前编辑第 {{ selectedProductIndex + 1 }} 个商品</p>
                </div>
                <div class="flex flex-wrap gap-2">
                  <button class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" :disabled="selectedProductIndex <= 0" @click="moveSelectedProduct(-1)">上移</button>
                  <button class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" :disabled="selectedProductIndex >= products.length - 1" @click="moveSelectedProduct(1)">下移</button>
                  <button class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200" @click="duplicateProduct">复制</button>
                  <button class="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 hover:bg-red-100 dark:border-red-900/60 dark:bg-red-900/20 dark:text-red-300" @click="removeSelectedProduct">删除</button>
                </div>
              </div>

              <div class="grid gap-4 md:grid-cols-2">
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">标题
                  <input :value="selectedProduct.title" class="input" @input="updateSelectedProduct({ title: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">Slug
                  <div class="space-y-2">
                    <input :value="selectedProduct.slug" class="input" @input="updateSelectedProduct({ slug: normalizeSlug(inputValue($event)) })" />
                    <button type="button" class="rounded-xl border border-gray-200 px-3 py-2 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700" @click="updateSelectedProduct({ slug: normalizeSlug(selectedProduct.title) })">根据标题生成</button>
                  </div>
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">分类
                  <input :value="selectedProduct.category" class="input" @input="updateSelectedProduct({ category: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">价格 + 币种
                  <div class="grid grid-cols-2 gap-3">
                    <input :value="selectedProduct.priceLabel" class="input" @input="updateSelectedProduct({ priceLabel: inputValue($event) })" />
                    <input :value="selectedProduct.currency || 'CNY'" class="input" @input="updateSelectedProduct({ currency: inputValue($event) })" />
                  </div>
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">商品类型
                  <select :value="selectedProduct.productType" class="input" @change="changeSelectedProductType(inputValue($event))">
                    <option value="topup">充值</option>
                    <option value="subscription">订阅</option>
                  </select>
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">排序
                  <input type="number" :value="selectedProduct.sortOrder" class="input" @input="updateSelectedProduct({ sortOrder: numberValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300 md:col-span-2">主图 URL
                  <input :value="selectedProduct.image" class="input" @input="updateSelectedProduct({ image: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300 md:col-span-2">卡片图 URL
                  <input :value="selectedProduct.cardImage || ''" class="input" @input="updateSelectedProduct({ cardImage: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300 md:col-span-2">标签（逗号分隔）
                  <input :value="selectedProduct.tags.join(', ')" class="input" @input="updateSelectedProduct({ tags: splitTags(inputValue($event)) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">徽标
                  <input :value="selectedProduct.badge || ''" class="input" @input="updateSelectedProduct({ badge: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">按钮文案
                  <input :value="selectedProduct.ctaText || ''" class="input" @input="updateSelectedProduct({ ctaText: inputValue($event) })" />
                </label>
                <div v-if="selectedProduct.productType === 'topup'" class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">充值金额（amount）
                  <div class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-700/60">
                    <div class="text-base font-semibold text-gray-900 dark:text-white">{{ formatCatalogAmount(selectedProduct.amount ?? 0) }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      根据价格 {{ selectedProduct.priceLabel || '0.00' }} 和倍率 {{ selectedProductPricing?.multiplier.toFixed(4) || '1.0000' }} 自动计算
                      <span v-if="selectedProductPricing?.label">（{{ selectedProductPricing.label }}）</span>
                    </div>
                  </div>
                </div>
                <label v-else class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">订阅 planId
                  <input :value="selectedProduct.planId || ''" class="input" @input="updateSelectedProduct({ planId: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">启用状态
                  <span class="flex h-11 items-center gap-3 rounded-2xl border border-gray-200 bg-white px-4 dark:border-dark-600 dark:bg-dark-700">
                    <input type="checkbox" :checked="selectedProduct.active" @change="updateSelectedProduct({ active: checkedValue($event) })" />
                    <span class="text-sm text-gray-700 dark:text-gray-200">{{ selectedProduct.active ? '启用' : '停用' }}</span>
                  </span>
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300 md:col-span-2">摘要
                  <textarea :value="selectedProduct.summary" class="input min-h-[120px] py-3" @input="updateSelectedProduct({ summary: inputValue($event) })" />
                </label>
                <label class="space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300 md:col-span-2">详情正文
                  <textarea :value="selectedProduct.description" class="input min-h-[220px] py-3" @input="updateSelectedProduct({ description: inputValue($event) })" />
                </label>
              </div>

              <div class="rounded-3xl border border-gray-200 bg-gray-50 p-5 dark:border-dark-600 dark:bg-dark-900/40">
                <div class="mb-3 text-sm font-semibold text-gray-800 dark:text-gray-100">实时预览</div>
                <div class="overflow-hidden rounded-3xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
                  <img v-if="selectedProduct.image" :src="selectedProduct.image" :alt="selectedProduct.title" class="h-64 w-full object-cover" />
                  <div v-else class="flex h-64 items-center justify-center text-sm text-gray-500 dark:text-gray-400">暂无图片</div>
                  <div class="p-5">
                    <div class="mb-2 flex flex-wrap gap-2 text-xs">
                      <span class="rounded-full bg-emerald-100 px-3 py-1 text-emerald-700">{{ selectedProduct.category || '未分类' }}</span>
                      <span v-for="tag in selectedProduct.tags" :key="tag" class="rounded-full bg-gray-100 px-3 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ tag }}</span>
                    </div>
                    <div class="text-2xl font-bold text-gray-900 dark:text-white">{{ selectedProduct.title || '未命名商品' }}</div>
                    <div class="mt-2 whitespace-pre-line text-sm leading-7 text-gray-500 dark:text-gray-400">{{ selectedProduct.summary }}</div>
                    <div class="mt-5 flex items-center justify-between">
                      <div class="text-3xl font-black text-gray-900 dark:text-white">
                        {{ selectedProduct.priceLabel || '0.00' }}
                        <span class="ml-2 text-sm font-semibold text-gray-500">{{ selectedProduct.currency || 'CNY' }}</span>
                      </div>
                      <span class="rounded-2xl bg-emerald-500 px-4 py-2 text-sm font-semibold text-white">{{ selectedProduct.ctaText || '立即购买' }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="flex min-h-[420px] items-center justify-center rounded-2xl border border-dashed border-gray-300 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
              还没有商品，点击“新增商品”开始。
            </div>
          </section>
        </div>
      </div>
    </div>

    <!-- Plan Edit Dialog -->
    <PlanEditDialog :show="showPlanDialog" :plan="editingPlan" :groups="groups" @close="showPlanDialog = false" @saved="loadPlans" />

    <ConfirmDialog :show="showDeletePlanDialog" :title="t('payment.admin.deletePlan')" :message="t('payment.admin.deletePlanConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeletePlan" @cancel="showDeletePlanDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import adminAPI from '@/api/admin'
import type { BalancePricingTier, CatalogProduct, SubscriptionPlan } from '@/types/payment'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlanEditDialog from './PlanEditDialog.vue'
import { platformTextClass } from '@/utils/platformColors'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])

async function loadGroups() {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch { /* ignore */ }
}

function getGroup(id: number): AdminGroup | undefined {
  return groups.value.find(g => g.id === id)
}

function isGroupMissing(id: number): boolean {
  return id > 0 && !groups.value.find(g => g.id === id)
}

function getPlanNameClass(groupId: number): string {
  const group = getGroup(groupId)
  return group ? platformTextClass(group.platform) : 'text-gray-900 dark:text-white'
}


// ==================== Plans ====================

const plansLoading = ref(false)
const plans = ref<SubscriptionPlan[]>([])
const showPlanDialog = ref(false)
const showDeletePlanDialog = ref(false)
const editingPlan = ref<SubscriptionPlan | null>(null)
const deletingPlanId = ref<number | null>(null)

const planColumns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('payment.admin.planName') },
  { key: 'group_id', label: t('payment.admin.group') },
  { key: 'price', label: t('payment.admin.price') },
  { key: 'validity_days', label: t('payment.admin.validityDays') },
  { key: 'for_sale', label: t('payment.admin.forSale') },
  { key: 'sort_order', label: t('payment.admin.sortOrder') },
  { key: 'actions', label: t('common.actions') },
])

async function loadPlans() {
  plansLoading.value = true
  try {
    const res = await adminPaymentAPI.getPlans()
    // Backend returns features as newline-separated string; parse to array
    plans.value = (res.data || []).map((p: Omit<SubscriptionPlan, 'features'> & { features: string | string[] }) => ({
      ...p,
      features: typeof p.features === 'string'
        ? p.features.split('\n').map((f: string) => f.trim()).filter(Boolean)
        : (p.features || []),
    }))
  }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { plansLoading.value = false }
}

function openPlanEdit(plan: SubscriptionPlan | null) {
  editingPlan.value = plan
  showPlanDialog.value = true
}


/** Quick toggle for_sale from the list */
async function toggleForSale(plan: SubscriptionPlan) {
  try {
    await adminPaymentAPI.updatePlan(plan.id, { for_sale: !plan.for_sale })
    plan.for_sale = !plan.for_sale
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

function confirmDeletePlan(plan: SubscriptionPlan) { deletingPlanId.value = plan.id; showDeletePlanDialog.value = true }
async function handleDeletePlan() {
  if (!deletingPlanId.value) return
  try { await adminPaymentAPI.deletePlan(deletingPlanId.value); appStore.showSuccess(t('common.deleted')); showDeletePlanDialog.value = false; loadPlans() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

// ==================== Homepage products ====================

const emptyProduct: CatalogProduct = {
  slug: '',
  title: '',
  category: '余额充值',
  summary: '',
  description: '',
  image: '',
  cardImage: '',
  tags: [],
  priceLabel: '',
  currency: 'CNY',
  badge: '',
  active: true,
  sortOrder: 10,
  productType: 'topup',
  amount: 0,
  planId: '',
  ctaText: '立即购买',
}

const productsLoading = ref(false)
const productsSaving = ref(false)
const products = ref<CatalogProduct[]>([])
const selectedProductIndex = ref(0)
const jsonOpen = ref(false)
const jsonDraft = ref('')
const balanceRechargeMultiplier = ref(1)
const balancePricingTiers = ref<BalancePricingTier[]>([])

const selectedProduct = computed(() => products.value[selectedProductIndex.value] || null)
const selectedProductPricing = computed(() => selectedProduct.value ? resolveCatalogTopupPricing(selectedProduct.value) : null)

async function loadPaymentPricingConfig() {
  const res = await adminPaymentAPI.getConfig()
  balanceRechargeMultiplier.value = normalizePositiveNumber(res.data.balance_recharge_multiplier, 1)
  balancePricingTiers.value = normalizeBalancePricingTiers(res.data.balance_pricing_tiers)
}

async function loadProducts() {
  productsLoading.value = true
  try {
    await loadPaymentPricingConfig()
    const res = await adminPaymentAPI.getProducts()
    products.value = (res.data?.products || []).map(normalizeProductForEdit)
    selectedProductIndex.value = products.value.length > 0 ? 0 : -1
    syncProductsJson()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    productsLoading.value = false
  }
}

function normalizeProductForEdit(product: CatalogProduct): CatalogProduct {
  const normalized: CatalogProduct = {
    ...emptyProduct,
    ...product,
    tags: Array.isArray(product.tags) ? product.tags : [],
    productType: product.productType === 'subscription' ? 'subscription' : 'topup',
    active: product.active !== false,
    sortOrder: Number(product.sortOrder || 0),
  }
  return withCalculatedCatalogAmount(normalized)
}

function normalizePositiveNumber(value: unknown, fallback: number): number {
  const numeric = Number(value)
  return Number.isFinite(numeric) && numeric > 0 ? numeric : fallback
}

function normalizeBalancePricingTiers(tiers: BalancePricingTier[] | undefined): BalancePricingTier[] {
  return (tiers || [])
    .filter(tier => tier?.enabled !== false && normalizePositiveNumber(tier.multiplier, 0) > 0 && Number(tier.min) >= 0 && Number(tier.max) >= Number(tier.min))
    .map(tier => ({
      ...tier,
      min: Number(tier.min),
      max: Number(tier.max),
      multiplier: Number(tier.multiplier),
      sortOrder: Number(tier.sortOrder || 0),
    }))
    .sort((a, b) => (a.sortOrder - b.sortOrder) || (a.min - b.min))
}

function parseCatalogPriceLabel(priceLabel: string): number {
  const cleaned = String(priceLabel || '').trim().replace(/[,¥￥$]/g, '')
  const first = cleaned.split(/\s+/)[0]
  const amount = Number(first || 0)
  return Number.isFinite(amount) && amount > 0 ? amount : 0
}

function resolveCatalogTopupPricing(product: CatalogProduct): { payAmount: number; multiplier: number; amount: number; label: string } {
  const payAmount = parseCatalogPriceLabel(product.priceLabel)
  const tier = balancePricingTiers.value.find(item => payAmount >= item.min && payAmount <= item.max)
  const multiplier = normalizePositiveNumber(tier?.multiplier, balanceRechargeMultiplier.value)
  return {
    payAmount,
    multiplier,
    amount: payAmount > 0 ? Math.round((payAmount / multiplier) * 100) / 100 : 0,
    label: tier?.label || '默认倍率',
  }
}

function withCalculatedCatalogAmount(product: CatalogProduct): CatalogProduct {
  if (product.productType !== 'topup') {
    return { ...product, amount: undefined }
  }
  return { ...product, amount: resolveCatalogTopupPricing(product).amount }
}

function formatCatalogAmount(amount: number): string {
  return Number(amount || 0).toFixed(2)
}

function syncProductsJson() {
  jsonDraft.value = JSON.stringify(products.value, null, 2)
}

function resetJsonDraft() {
  syncProductsJson()
}

function normalizeSlug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/-{2,}/g, '-')
}

function inputValue(event: Event): string {
  return (event.target as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement).value
}

function numberValue(event: Event): number {
  return Number(inputValue(event) || 0)
}

function checkedValue(event: Event): boolean {
  return (event.target as HTMLInputElement).checked
}

function splitTags(value: string): string[] {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

function productListButtonClass(index: number): string[] {
  const selected = index === selectedProductIndex.value
  return [
    'w-full rounded-2xl border px-3 py-3 text-left transition',
    selected
      ? 'border-emerald-400 bg-emerald-50 dark:border-emerald-500 dark:bg-emerald-900/20'
      : 'border-gray-200 bg-white hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-700 dark:hover:bg-dark-600',
  ]
}

function selectProduct(index: number) {
  selectedProductIndex.value = index
}

function updateSelectedProduct(patch: Partial<CatalogProduct>) {
  const current = selectedProduct.value
  if (!current) return
  products.value[selectedProductIndex.value] = normalizeProductForEdit({ ...current, ...patch })
  syncProductsJson()
}

function changeSelectedProductType(value: string) {
  const current = selectedProduct.value
  if (!current) return
  const productType = value === 'subscription' ? 'subscription' : 'topup'
  updateSelectedProduct({
    productType,
    amount: productType === 'topup' ? resolveCatalogTopupPricing(current).amount : undefined,
    planId: productType === 'subscription' ? current.planId || '' : '',
  })
}

function addProduct() {
  const nextOrder = products.value.length > 0
    ? Math.max(...products.value.map(product => product.sortOrder || 0)) + 10
    : 10
  products.value.push({
    ...emptyProduct,
    sortOrder: nextOrder,
  })
  selectedProductIndex.value = products.value.length - 1
  syncProductsJson()
}

function duplicateProduct() {
  const current = selectedProduct.value
  if (!current) return
  const copied = normalizeProductForEdit({
    ...current,
    slug: current.slug ? `${current.slug}-copy` : '',
    title: current.title ? `${current.title}（副本）` : '',
    sortOrder: (current.sortOrder || 0) + 1,
    tags: [...(current.tags || [])],
  })
  products.value.splice(selectedProductIndex.value + 1, 0, copied)
  selectedProductIndex.value += 1
  syncProductsJson()
}

function removeSelectedProduct() {
  if (!selectedProduct.value || products.value.length === 0) return
  products.value.splice(selectedProductIndex.value, 1)
  selectedProductIndex.value = products.value.length > 0
    ? Math.max(0, Math.min(selectedProductIndex.value, products.value.length - 1))
    : -1
  syncProductsJson()
}

function moveSelectedProduct(direction: -1 | 1) {
  const target = selectedProductIndex.value + direction
  if (target < 0 || target >= products.value.length) return
  const next = [...products.value]
  ;[next[selectedProductIndex.value], next[target]] = [next[target], next[selectedProductIndex.value]]
  products.value = next
  selectedProductIndex.value = target
  syncProductsJson()
}

function applyJsonDraft() {
  try {
    const parsed = JSON.parse(jsonDraft.value)
    if (!Array.isArray(parsed)) throw new Error('JSON 顶层必须是数组')
    products.value = parsed.map(normalizeProductForEdit)
    selectedProductIndex.value = products.value.length > 0 ? 0 : -1
    syncProductsJson()
    appStore.showSuccess('已应用 JSON 草稿，记得点击保存商品设置')
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : 'JSON 解析失败')
  }
}

async function saveProducts() {
  productsSaving.value = true
  try {
    const res = await adminPaymentAPI.updateProducts(products.value.map(withCalculatedCatalogAmount))
    products.value = (res.data?.products || products.value).map(normalizeProductForEdit)
    selectedProductIndex.value = products.value.length > 0
      ? Math.max(0, Math.min(selectedProductIndex.value, products.value.length - 1))
      : -1
    syncProductsJson()
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    productsSaving.value = false
  }
}

// ==================== Lifecycle ====================

onMounted(() => {
  loadGroups()
  loadPlans()
  loadProducts()
})
</script>
