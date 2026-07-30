<template>
  <section class="models-page legacy-model-page">
    <a class="skip-link" href="#legacy-model-main">跳到正文</a>
    <header v-if="!embedded" class="site-header">
      <div class="shell header-inner">
        <a class="brand" href="/" aria-label="模型广场">
          <span class="brand-mark" aria-hidden="true">M</span>
          <span class="brand-copy"><strong>Model Plaza</strong><small>MODEL ACCESS</small></span>
        </a>
        <nav class="site-nav" :data-open="mobileMenuOpen" aria-label="主导航">
          <a href="/home/">首页</a>
          <a href="/model-plaza" aria-current="page">模型列表</a>
          <a href="/tutorial/" target="_blank" rel="noopener noreferrer">使用教程</a>
          <a href="/dashboard">控制台</a>
        </nav>
        <div class="header-actions">
          <a class="button button-secondary header-login" href="/login">登录</a>
          <button class="icon-button" type="button" aria-label="切换主题" title="切换主题" @click="toggleTheme"><svg class="icon theme-icon theme-icon-sun" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41"/></svg><svg class="icon theme-icon theme-icon-moon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M20.5 15.2A8.5 8.5 0 0 1 8.8 3.5 8.5 8.5 0 1 0 20.5 15.2Z"/></svg></button>
          <button class="icon-button menu-button" type="button" :aria-expanded="mobileMenuOpen" aria-label="打开导航菜单" @click="mobileMenuOpen = !mobileMenuOpen">☰</button>
        </div>
      </div>
    </header>

    <main id="legacy-model-main">
      <h1 class="sr-only">模型列表</h1>
      <div class="models-controls" :data-scroll-state="controlsVisible ? 'shown' : 'hidden'">
        <div class="shell">
          <div class="controls-layout">
            <div class="search-box-large">
              <svg class="icon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
              <input v-model="query" type="search" placeholder="搜索模型名称..." aria-label="搜索模型" data-model-search>
            </div>
            <div class="filter-toolbar">
              <div class="filter-section filter-section--type">
                <span class="filter-section-label">类型</span>
                <div class="filter-group">
                  <button v-for="item in typeOptions" :key="item.value" class="filter-button filter-button--compact" type="button" :data-active="activeType === item.value" :aria-pressed="activeType === item.value" @click="activeType = item.value">{{ item.label }} <span class="count nums">{{ typeCount(item.value) }}</span></button>
                </div>
              </div>
              <div class="divider-vertical" />
              <div class="filter-section filter-section--dropdown filter-section--vendors">
                <span class="filter-section-label">厂商</span>
                <div class="filter-menu" :data-open="vendorMenuOpen">
                  <button class="filter-menu-trigger" type="button" :aria-expanded="vendorMenuOpen" @click="vendorMenuOpen = !vendorMenuOpen"><span class="filter-menu-trigger-copy"><strong>{{ vendorSummary }}</strong><small>{{ selectedVendors.length }} / {{ vendorOptions.length }} 个厂商</small></span><span class="filter-menu-chevron">⌄</span></button>
                  <div class="filter-menu-panel filter-menu-panel--vendors" role="group" :hidden="!vendorMenuOpen">
                    <label v-for="vendor in vendorOptions" :key="vendor.id" class="filter-option-card"><input v-model="selectedVendors" class="filter-option-input" type="checkbox" :value="vendor.id"><span class="vendor-icon" :class="`vendor-icon--${vendor.id}`"><span class="vendor-letter">{{ vendor.label.slice(0, 1) }}</span></span><span class="filter-option-copy"><strong>{{ vendor.label }}</strong><small>{{ vendor.count }} 个模型</small></span><span class="filter-option-check">✓</span></label>
                  </div>
                </div>
              </div>
              <div class="divider-vertical" />
              <div class="filter-section filter-section--dropdown filter-section--groups">
                <span class="filter-section-label">分组</span>
                <div class="filter-menu" :data-open="groupMenuOpen">
                  <button class="filter-menu-trigger" type="button" :aria-expanded="groupMenuOpen" @click="groupMenuOpen = !groupMenuOpen"><span class="filter-menu-trigger-copy"><strong>{{ groupSummary }}</strong><small>{{ groupCount }}</small></span><span class="filter-menu-chevron">⌄</span></button>
                  <div class="filter-menu-panel filter-menu-panel--groups" role="radiogroup" :hidden="!groupMenuOpen">
                    <label class="filter-option-card filter-option-card--group"><input v-model="activeGroup" class="filter-option-input" type="radio" value="all"><span class="filter-option-copy"><strong>全部分组</strong><small>{{ modelCount }} 个模型</small></span><span class="filter-option-check">✓</span></label>
                    <label v-for="group in groups" :key="group.id" class="filter-option-card filter-option-card--group"><input v-model="activeGroup" class="filter-option-input" type="radio" :value="String(group.id)"><span class="filter-option-copy"><strong>{{ group.name }}</strong><small>{{ group.models.length }} 个模型 · x{{ formatRate(effectiveRate(group)) }}</small><em class="filter-option-note">{{ groupBillingLabel(group) }}</em></span><span class="filter-option-check">✓</span></label>
                  </div>
                </div>
              </div>
              <div class="divider-vertical" />
              <div class="filter-section filter-section--sort"><span class="filter-section-label">排序</span><select v-model="sortMode" class="sort-select" aria-label="模型排序方式"><option value="default">默认排序</option><option value="price-low">价格最低</option><option value="price-high">价格最高</option><option value="multiplier-low">最优倍率</option><option value="vendor">厂商分组</option></select></div>
            </div>
          </div>
        </div>
      </div>

      <div class="models-content"><div class="shell">
        <div v-if="loading" class="catalog-load-error" role="status">正在读取最新模型目录…</div>
        <div v-else-if="error" class="catalog-load-error" role="alert">模型目录读取失败，请稍后重试。</div>
        <div v-else-if="!filteredOffers.length" class="empty-state"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg><h2>没有找到匹配的模型</h2><p>尝试调整筛选条件或搜索关键词</p></div>
        <div v-else class="models-grid">
          <article v-for="offer in filteredOffers" :key="`${offer.group.id}:${offer.model.name}`" class="model-card">
            <div class="model-card-header"><div class="model-icon" :class="`model-icon--${offer.vendor}`"><span class="vendor-letter">{{ offer.vendorLabel.slice(0, 1) }}</span></div><div class="model-info"><span class="model-vendor">{{ offer.vendorLabel }}</span><div class="model-name-row"><h2 class="model-name">{{ offer.model.name }}</h2><button class="copy-id-btn" type="button" @click="copyModel(offer.model.name)">▣ <span class="copy-text">{{ copiedModel === offer.model.name ? '已复制' : '复制 ID' }}</span></button></div></div></div>
            <div class="model-info-grid"><div class="info-item"><span class="info-label">当前分组</span><span class="info-value">{{ offer.group.name }}</span></div><div class="info-item"><span class="info-label">生效倍率</span><span class="info-value multiplier nums">x{{ formatRate(effectiveRate(offer.group)) }}</span></div><div class="info-item info-item--full"><span class="info-label">模型信息</span><div class="protocol-badges"><span class="protocol-badge">{{ isImageOffer(offer.model) ? '按张计费' : offer.model.pricing?.billing_mode === 'per_request' ? '按次计费' : 'Token 计费' }}</span><span class="protocol-badge" :class="isSubscription(offer.group) ? 'protocol-badge--subscription' : 'protocol-badge--balance'">{{ groupBillingLabel(offer.group) }}</span><span v-if="offer.group.peak_rate_enabled" class="protocol-badge">峰值倍率 {{ formatRate(offer.group.peak_rate_multiplier) }}x</span></div></div><div class="info-item info-item--full"><span class="info-label">能力支持</span><div class="capability-badges"><span class="capability-badge">{{ isImageOffer(offer.model) ? '图片' : '文本' }}</span></div></div></div>
            <div class="model-pricing-detailed"><template v-for="row in priceRows(offer.model, offer.group)" :key="row.label"><div class="price-row"><div class="price-label-col"><span>{{ row.label }}</span><span class="price-unit">{{ row.unit }}</span></div><div class="price-current nums">{{ row.current }}</div><div class="price-detail-col"><span v-if="row.official" class="price-original">官方 {{ row.official }}</span><span v-if="row.discount" class="price-save">{{ row.discount }}</span><span v-if="!row.official && !row.discount" class="price-detail-empty">—</span></div></div></template></div>
          </article>
        </div>
      </div></div>
    </main>
    <footer v-if="!embedded" class="site-footer"><div class="shell footer-inner"><div><strong>Model Plaza</strong><div class="footer-copy">© 2026 · 统一模型接入</div></div><nav class="footer-links" aria-label="页脚导航"><a href="/model-plaza">模型列表</a><a href="/tutorial/" target="_blank" rel="noopener noreferrer">使用教程</a><a href="/login">登录</a></nav></div></footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import { calculateDiscountPercent, effectiveRate, formatChannelPricePerMillion, formatOfficialPricePerMillion, groupBillingLabel, isImageModel } from '@/features/model-plaza-custom/modelPlazaCustom'
import './legacyModelCatalog.css'

const props = defineProps<{ response: { description: string; groups: ModelPlazaGroup[] } | null; loading: boolean; error?: boolean; embedded?: boolean }>()
const query = ref(''); const activeType = ref<'all' | 'text' | 'image'>('all'); const activeGroup = ref('all'); const sortMode = ref('default'); const selectedVendors = ref<string[]>([]); const vendorMenuOpen = ref(false); const groupMenuOpen = ref(false); const copiedModel = ref(''); const controlsVisible = ref(true); const mobileMenuOpen = ref(false)
const embedded = computed(() => props.embedded === true)
const groups = computed(() => props.response?.groups ?? [])
const allOffers = computed(() => groups.value.flatMap(group => group.models.map(model => ({ group, model, vendor: model.platform || group.platform, vendorLabel: platformLabel(model.platform || group.platform) }))))
const vendorOptions = computed(() => [...new Map(allOffers.value.map(offer => [offer.vendor, { id: offer.vendor, label: offer.vendorLabel, count: allOffers.value.filter(item => item.vendor === offer.vendor).length }])).values()])
const modelCount = computed(() => new Set(allOffers.value.map(offer => offer.model.name)).size)
const vendorSummary = computed(() => selectedVendors.value.length === vendorOptions.value.length ? '全部厂商' : selectedVendors.value.length === 1 ? vendorOptions.value.find(v => v.id === selectedVendors.value[0])?.label || '未选厂商' : selectedVendors.value.length ? `${selectedVendors.value[0]} 等 ${selectedVendors.value.length} 家` : '未选厂商')
const groupSummary = computed(() => activeGroup.value === 'all' ? '全部分组' : groups.value.find(group => String(group.id) === activeGroup.value)?.name || '全部分组')
const groupCount = computed(() => activeGroup.value === 'all' ? `${groups.value.length} 个可用分组` : `${groupBillingLabel(groups.value.find(group => String(group.id) === activeGroup.value) || groups.value[0])}`)
const typeOptions = [{ value: 'all' as const, label: '全部' }, { value: 'text' as const, label: '文本' }, { value: 'image' as const, label: '图片' }]
const filteredOffers = computed(() => { let offers = allOffers.value.filter(offer => { const image = isImageOffer(offer.model); if (activeType.value !== 'all' && (activeType.value === 'image') !== image) return false; if (activeGroup.value !== 'all' && String(offer.group.id) !== activeGroup.value) return false; if (selectedVendors.value.length && !selectedVendors.value.includes(offer.vendor)) return false; if (query.value) { const text = [offer.model.name, offer.vendorLabel, offer.group.name, offer.model.platform].filter(Boolean).join(' ').toLowerCase(); if (!text.includes(query.value.trim().toLowerCase())) return false } return true }); if (sortMode.value === 'price-low') offers = [...offers].sort((a,b) => primaryPrice(a.model) - primaryPrice(b.model)); if (sortMode.value === 'price-high') offers = [...offers].sort((a,b) => primaryPrice(b.model) - primaryPrice(a.model)); if (sortMode.value === 'multiplier-low') offers = [...offers].sort((a,b) => effectiveRate(a.group) - effectiveRate(b.group)); if (sortMode.value === 'vendor') offers = [...offers].sort((a,b) => a.vendorLabel.localeCompare(b.vendorLabel, 'zh-CN')); return offers })
function platformLabel(value: string): string { const labels: Record<string, string> = { openai: 'OpenAI', anthropic: 'Anthropic', google: 'Google', gemini: 'Gemini', deepseek: 'DeepSeek', zhipu: '智谱', grok: 'Grok' }; return labels[value.toLowerCase()] || value }

function formatRate(value: number): string { return Number(value).toFixed(2).replace(/0+$/, '').replace(/\.$/, '') }
function isImageOffer(model: PlazaModel): boolean { return isImageModel(model) || model.pricing?.billing_mode === 'per_request' }
function isSubscription(group: ModelPlazaGroup): boolean { return group.subscription_type === 'subscription' }
function primaryPrice(model: PlazaModel): number { return model.pricing?.input_price ?? model.pricing?.per_request_price ?? Number.MAX_VALUE }
function typeCount(type: 'all' | 'text' | 'image'): number { const models = allOffers.value.filter(offer => type === 'all' || (type === 'image') === isImageOffer(offer.model)); return new Set(models.map(offer => offer.model.name)).size }
function priceRows(model: PlazaModel, group: ModelPlazaGroup) { if (isImageOffer(model)) return [{ label: model.pricing?.billing_mode === 'per_request' ? '单次价格' : '单张价格', unit: '/ 张', current: formatChannelPricePerMillion(model.pricing?.per_request_price, effectiveRate(group)), official: '', discount: '' }]; const rows = [['输入', model.pricing?.input_price, model.official_pricing?.input_price], ['输出', model.pricing?.output_price, model.official_pricing?.output_price], ['缓存写入', model.pricing?.cache_write_price, model.official_pricing?.cache_write_price], ['缓存读取', model.pricing?.cache_read_price, model.official_pricing?.cache_read_price]] as const; return rows.filter(row => row[1] != null || row[2] != null).map(([label, current, official]) => { const discount = calculateDiscountPercent(current, official, effectiveRate(group)); return { label, unit: '/M', current: formatChannelPricePerMillion(current, effectiveRate(group)), official: formatOfficialPricePerMillion(official), discount: discount == null ? '' : discount >= 0 ? `优惠 ${discount.toFixed(1)}%` : `高于官方 ${Math.abs(discount).toFixed(1)}%` } }) }
async function copyModel(name: string) { try { await navigator.clipboard.writeText(name); copiedModel.value = name; window.setTimeout(() => { if (copiedModel.value === name) copiedModel.value = '' }, 1500) } catch { copiedModel.value = '' } }
function toggleTheme() { const root = document.documentElement; const dark = root.dataset.theme !== 'dark'; root.dataset.theme = dark ? 'dark' : 'light'; root.classList.toggle('dark', dark) }
let vendorFilterInitialized = false
watch(vendorOptions, (options) => { if (!vendorFilterInitialized && options.length) { selectedVendors.value = options.map(vendor => vendor.id); vendorFilterInitialized = true } }, { immediate: true })
onMounted(() => { const root = document.documentElement; if (!root.dataset.theme) root.dataset.theme = root.classList.contains('dark') ? 'dark' : 'light'; let lastY = window.scrollY; window.addEventListener('scroll', () => { const currentY = window.scrollY; controlsVisible.value = currentY <= 16 || currentY < lastY; lastY = currentY }, { passive: true }) })
</script>
