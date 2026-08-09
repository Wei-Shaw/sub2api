<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- 页头 -->
      <div class="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
            {{ t('videoModels.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('videoModels.description') }}
          </p>
        </div>
        <div class="flex items-center gap-3">
          <button
            @click="loadModels"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            <span class="ml-1 hidden sm:inline">{{ t('common.refresh') }}</span>
          </button>
        </div>
      </div>

      <!-- 筛选卡片 -->
      <div class="card p-6">
        <!-- 一级筛选：vendor -->
        <div>
          <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('videoModels.vendorFilter') }}
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="opt in vendorOptions"
              :key="opt.value"
              type="button"
              @click="selectVendor(opt.value)"
              :class="[
                'rounded-full border px-3 py-1 text-sm transition',
                vendorFilter === opt.value
                  ? 'border-blue-600 bg-blue-600 text-white shadow-sm hover:bg-blue-700'
                  : 'border-gray-200 bg-white text-gray-700 hover:border-blue-400 hover:text-blue-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-blue-500 dark:hover:text-blue-300'
              ]"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <!-- 二级筛选：family -->
        <div class="mt-4">
          <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('videoModels.familyFilter') }}
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="opt in familyOptions"
              :key="opt.value"
              type="button"
              @click="familyFilter = opt.value"
              :class="[
                'rounded-full border px-3 py-1 text-sm transition',
                familyFilter === opt.value
                  ? 'border-purple-600 bg-purple-600 text-white shadow-sm hover:bg-purple-700'
                  : 'border-gray-200 bg-white text-gray-700 hover:border-purple-400 hover:text-purple-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-purple-500 dark:hover:text-purple-300'
              ]"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <!-- 底部工具条：仅可用 + 重置 + 结果计数 -->
        <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-3 dark:border-dark-700">
          <div class="flex items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
              <input v-model="onlyAvailable" type="checkbox" class="rounded" />
              {{ t('videoModels.onlyAvailable') }}
            </label>
            <button type="button" @click="resetFilters" class="btn btn-secondary">
              {{ t('common.reset') }}
            </button>
          </div>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('videoModels.resultCount', { shown: filteredItems.length, total: items.length }) }}
          </span>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="loading && !items.length" class="card p-16 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>

      <!-- 空态 -->
      <div
        v-else-if="!filteredItems.length"
        class="card p-16 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ items.length === 0 ? t('videoModels.empty') : t('videoModels.emptyByFilter') }}
      </div>

      <!-- 模型卡片网格 —— 卡片整体缩小：一行更密（md:2 / lg:3 / 2xl:4），
           容器内 padding、图片高度、标题字号、间距全部同步收紧，视觉更紧凑。 -->
      <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
        <article
          v-for="model in filteredItems"
          :key="model.slug"
          class="card flex flex-col overflow-hidden p-0 transition hover:shadow-md"
        >
          <!-- 封面图（管理员配置了才展示）：
               - 容器锁 16:9 (aspect-video)，与用户提供的横图原始比例一致；
               - 图片使用 object-cover 保证铺满且不留黑边，同时因比例一致不会发生裁剪变形；
               - 加载失败时 onCoverError 会隐藏整个容器，防止破图。
               - 可用模型时封面整体可点，作为演练台的第二入口；不可用时保持只读手型。 -->
          <div
            v-if="model.intro && model.intro.cover_url"
            class="aspect-video w-full overflow-hidden bg-gray-100 dark:bg-dark-700"
            :class="model.available ? 'cursor-pointer group' : 'cursor-not-allowed'"
            :title="model.available ? t('videoModels.playground.tooltipAvailable') : t('videoModels.playground.tooltipUnavailable')"
            @click="openPlayground(model)"
          >
            <img
              :src="model.intro.cover_url"
              :alt="model.intro.title || model.slug"
              class="h-full w-full object-cover transition duration-200 group-hover:scale-[1.02]"
              loading="lazy"
              @error="onCoverError"
            />
          </div>

          <div class="flex flex-1 flex-col p-4">
          <!-- 卡片头部 —— 标题也是演练台入口，可用时高亮 hover 提示；
               标题限制单行 truncate，slug 保持 mono 小字。 -->
          <header class="mb-2 flex items-start justify-between gap-2">
            <div class="min-w-0">
              <h2
                class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100"
                :class="model.available ? 'cursor-pointer hover:text-blue-600 dark:hover:text-blue-400' : 'cursor-not-allowed opacity-80'"
                :title="model.available ? t('videoModels.playground.tooltipAvailable') : t('videoModels.playground.tooltipUnavailable')"
                @click="openPlayground(model)"
              >
                {{ (model.intro && model.intro.title) || model.display_name }}
              </h2>
              <!--
                备注：过去这里还展示了一行 mono 小灰字 slug 作为唯一标识。
                与大标题（display_name / intro.title）在多数模型下是同一份名字，
                视觉上重复且冗余；用户反馈后移除。vendor/family tag 已能说明
                模型出处，完整 slug 仍可在 “提交端点” 复制区看到。
              -->
            </div>
            <span
              :class="[
                'shrink-0 rounded-full px-2 py-0.5 text-xs font-medium',
                model.available
                  ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                  : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
              ]"
            >
              {{ model.available ? t('videoModels.badgeAvailable') : t('videoModels.badgeUnavailable') }}
            </span>
          </header>

          <!-- vendor + family tag —— 收紧字号，配合卡片整体缩小。 -->
          <div class="mb-2 flex flex-wrap gap-1.5">
            <span class="rounded bg-blue-50 px-1.5 py-0.5 text-[11px] text-blue-700 dark:bg-blue-950 dark:text-blue-300">
              {{ getVendor(model.slug) || '-' }}
            </span>
            <span class="rounded bg-purple-50 px-1.5 py-0.5 text-[11px] text-purple-700 dark:bg-purple-950 dark:text-purple-300">
              {{ getFamily(model.slug) || model.family || '-' }}
            </span>
          </div>

          <!-- 模型介绍（管理员配置，中英双文 + 保留换行）—— 缩小至 xs，最多 3 行避免撑高卡片。 -->
          <p
            v-if="model.intro && getIntroDescription(model.intro)"
            class="mb-2 whitespace-pre-line text-xs leading-relaxed text-gray-600 line-clamp-3 dark:text-gray-300"
          >
            {{ getIntroDescription(model.intro) }}
          </p>

          <!-- 定价表 —— 字号缩到 xs，行高更紧。 -->
          <section class="flex-1">
            <h3 class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('videoModels.pricingTitle') }}
            </h3>
            <div v-if="!model.pricing.length" class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.noPricing') }}
            </div>
            <table v-else class="w-full text-xs">
              <thead>
                <tr class="text-left text-[11px] text-gray-500 dark:text-gray-400">
                  <th class="pb-1 font-normal">{{ t('videoModels.resolution') }}</th>
                  <th class="pb-1 font-normal">{{ t('videoModels.pricePerSecond') }}</th>
                  <th class="pb-1 font-normal">{{ t('videoModels.status') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="p in sortedPricing(model.pricing)"
                  :key="p.resolution"
                  class="border-t border-gray-100 text-gray-700 dark:border-dark-700 dark:text-gray-300"
                >
                  <td class="py-1 font-medium">{{ p.resolution }}</td>
                  <td class="py-1">{{ formatPrice(p.price_per_second, p.currency) }}</td>
                  <td class="py-1">
                    <span :class="p.enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-400'">
                      {{ p.enabled ? t('videoModels.enabled') : t('videoModels.disabled') }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </section>

          <!-- 调用示例 -->
          <section class="mt-3">
            <h3 class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('videoModels.endpointTitle') }}
            </h3>
            <code
              class="block cursor-pointer break-all rounded bg-gray-100 px-2 py-1 text-[11px] text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
              @click="copy(model.submit_path)"
              :title="t('videoModels.clickToCopy')"
            >
              POST {{ model.submit_path }}
            </code>
          </section>

          <!-- 演练台入口：路由跳转到独立页面（不再用弹窗）。
               卡片封面和标题也已挂上 openPlayground，此按钮作为主要 CTA 保留，
               视觉上做大做突出，方便用户第一眼就看到。 -->
          <footer class="mt-3 flex items-center justify-end gap-2">
            <button
              class="inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-blue-700 hover:shadow disabled:cursor-not-allowed disabled:bg-blue-300 disabled:shadow-none"
              :disabled="!model.available"
              :title="model.available ? t('videoModels.playground.tooltipAvailable') : t('videoModels.playground.tooltipUnavailable')"
              @click="openPlayground(model)"
            >
              <Icon name="play" size="sm" />
              {{ t('videoModels.tryIt') }}
            </button>
          </footer>
          </div>
        </article>
      </div>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import videoModelsAPI, {
  type VideoModelItem,
  type VideoModelPricingItem
} from '@/api/videoModels'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t, locale } = useI18n()

/**
 * getIntroDescription：按当前 locale 挑选模型介绍文案。
 *   - 英文界面（locale 以 en 开头）：优先展示 description_en，缺失回落 description；
 *   - 其他语言（默认中文）：优先展示 description，缺失回落 description_en。
 * 两个都为空时返回空串，外层 v-if 会自动隐藏整个介绍段落。
 */
function getIntroDescription(intro: { description?: string; description_en?: string } | null | undefined): string {
  if (!intro) return ''
  const zh = (intro.description || '').trim()
  const en = (intro.description_en || '').trim()
  const isEn = String(locale.value || '').toLowerCase().startsWith('en')
  if (isEn) return en || zh
  return zh || en
}
const appStore = useAppStore()
const router = useRouter()
const route = useRoute()

const items = ref<VideoModelItem[]>([])
const loading = ref(false)
const onlyAvailable = ref(false)

// ============ 二维筛选 ============
// 一级：vendor = slug.split('/')[0]（如 bytedance / minimax / kling ...）
// 二级：family = slug.split('/')[1]（如 seedance-2.5 / hailuo-02 ...）
// slug 已经不含 "fal-ai/" 前缀（backend 侧已剥离），因此第一段就是真正的厂商目录。
const vendorFilter = ref<string>('all')
const familyFilter = ref<string>('all')

function getVendor(slug: string): string {
  if (!slug) return ''
  const parts = slug.split('/')
  return parts[0] || ''
}

function getFamily(slug: string): string {
  if (!slug) return ''
  const parts = slug.split('/')
  return parts[1] || ''
}

// vendorOptions 从当前模型集合动态推导 —— 只出现实际有模型的 vendor，避免死值。
const vendorOptions = computed(() => {
  const set = new Set<string>()
  for (const m of items.value) {
    const v = getVendor(m.slug)
    if (v) set.add(v)
  }
  const opts = Array.from(set)
    .sort((a, b) => a.localeCompare(b))
    .map((v) => ({ value: v, label: v }))
  return [{ value: 'all', label: t('videoModels.vendorAll') }, ...opts]
})

// familyOptions 跟随 vendorFilter 动态收缩：
//   - vendor=all 时聚合全部模型的 family；
//   - vendor 指定时仅列该 vendor 下出现过的 family。
const familyOptions = computed(() => {
  const set = new Set<string>()
  for (const m of items.value) {
    if (vendorFilter.value !== 'all' && getVendor(m.slug) !== vendorFilter.value) continue
    const f = getFamily(m.slug)
    if (f) set.add(f)
  }
  const opts = Array.from(set)
    .sort((a, b) => a.localeCompare(b))
    .map((f) => ({ value: f, label: f }))
  return [{ value: 'all', label: t('videoModels.familyAll') }, ...opts]
})

// 一级变更时，若当前二级不再存在于新的候选中，则回退到 all，避免"看似过滤实则空表"。
function selectVendor(value: string) {
  vendorFilter.value = value
  const validFamilies = new Set(
    items.value
      .filter((m) => vendorFilter.value === 'all' || getVendor(m.slug) === vendorFilter.value)
      .map((m) => getFamily(m.slug))
  )
  if (familyFilter.value !== 'all' && !validFamilies.has(familyFilter.value)) {
    familyFilter.value = 'all'
  }
}

function resetFilters() {
  vendorFilter.value = 'all'
  familyFilter.value = 'all'
  onlyAvailable.value = false
}

const filteredItems = computed(() => {
  return items.value.filter((m) => {
    if (onlyAvailable.value && !m.available) return false
    if (vendorFilter.value !== 'all' && getVendor(m.slug) !== vendorFilter.value) return false
    if (familyFilter.value !== 'all' && getFamily(m.slug) !== familyFilter.value) return false
    return true
  })
})

const RES_ORDER: Record<string, number> = { '480p': 0, '720p': 1, '1080p': 2, '4k': 3 }
function sortedPricing(list: VideoModelPricingItem[]): VideoModelPricingItem[] {
  return [...list].sort((a, b) => {
    const oa = RES_ORDER[a.resolution] ?? 99
    const ob = RES_ORDER[b.resolution] ?? 99
    return oa - ob
  })
}

function formatPrice(price: number, currency: string): string {
  const cur = currency || 'USD'
  if (!Number.isFinite(price) || price <= 0) return '-'
  return `${cur} $${price.toFixed(4)}/s`
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('videoModels.copied'))
  } catch {
    appStore.showError(t('videoModels.copyFailed'))
  }
}

async function loadModels() {
  loading.value = true
  try {
    const { data } = await videoModelsAPI.list()
    items.value = data.items ?? []
    // 首次数据加载完成后：把 URL 上的 vendor/family query 同步到过滤器状态。
    // 之所以放在这里而不是 onMounted：vendorOptions/familyOptions 都是从
    // items 计算的，query 只在能匹配到当前候选值时才应用，避免带上一个不存在
    // 的 vendor 导致列表空白。
    applyQueryFilter()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

/**
 * applyQueryFilter：读取 route.query.vendor / route.query.family 并同步到本地
 * vendorFilter / familyFilter。只有当值出现在候选集合里时才应用；不匹配则
 * 保持 'all' 兜底，避免出现"选中的 tag 不在下拉里"的诡异状态。
 * 这是"从演练台点 vendor/family chip 跳过来时自动过滤"的落点。
 */
function applyQueryFilter() {
  const q = route.query || {}
  const vendorQ = typeof q.vendor === 'string' ? q.vendor.trim() : ''
  const familyQ = typeof q.family === 'string' ? q.family.trim() : ''
  // vendorOptions/familyOptions 结构为 { value, label }[]（含 'all' 占位），
  // 因此用 .some(o => o.value === ...) 校验是否为合法候选值。
  if (vendorQ && vendorOptions.value.some((o) => o.value === vendorQ)) {
    vendorFilter.value = vendorQ
    // family 只在 vendor 已经应用后再校验，因为 familyOptions 会随 vendor 变化
    if (familyQ && familyOptions.value.some((o) => o.value === familyQ)) {
      familyFilter.value = familyQ
    }
  }
}

// ============ 演练台入口 ============
// 演练台已改为独立页面 VideoPlaygroundView；此处仅负责路由跳转，
// 模型数据由目标页面自行通过 videoModelsAPI.list() 按 slug 匹配加载。
// slug 可能含 "/"（如 "bytedance/seedance-2.5/text-to-video"），
// 使用 pathMatch 通配路由，因此这里把 slug 按 "/" 拆成 params 数组喂给 vue-router，
// 避免整段 slug 被再次 encodeURIComponent 破坏路径分段。
function openPlayground(model: VideoModelItem) {
  if (!model.available) return
  const parts = String(model.slug || '').split('/').filter(Boolean)
  router.push({
    name: 'VideoPlayground',
    params: { slug: parts },
  })
}

// onCoverError 在封面图加载失败时隐藏图片容器，避免卡片顶部出现破图图标。
function onCoverError(e: Event) {
  const el = e.target as HTMLImageElement | null
  const parent = el?.parentElement as HTMLElement | null
  if (parent) parent.style.display = 'none'
}

onMounted(loadModels)
</script>
