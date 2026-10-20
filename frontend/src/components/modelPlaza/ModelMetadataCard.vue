<template>
  <div class="model-card-wrapper flex flex-col">
    <article
    class="model-card group relative flex flex-col rounded-[22px] border border-primary-200 bg-white p-5 shadow-card transition hover:border-primary-400 hover:shadow-card-hover dark:border-primary-800/70 dark:bg-dark-800/80 dark:hover:border-primary-600"
    :class="{ 'pricing-open rounded-b-none border-b-0': pricingOpen }"
  >
    <div class="flex items-start gap-4">
      <div
        class="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl border border-gray-200 bg-gray-50 text-gray-900 shadow-sm dark:border-dark-600 dark:bg-dark-900/60 dark:text-white"
        :aria-label="platformName"
      >
        <PlatformIcon
          v-if="useSharedPlatformIcon"
          :platform="model.platform as GroupPlatform"
          size="lg"
          :class="platformIconClass"
        />
        <ModelPlazaPlatformIcon v-else :platform="model.platform" />
      </div>
      <div class="min-w-0 flex-1">
        <h2 class="truncate pr-24 text-xl font-semibold leading-tight text-gray-900 dark:text-white">
          {{ model.display_name }}
        </h2>
        <p class="mt-1 text-base text-gray-500 dark:text-dark-400">{{ platformName }}</p>
      </div>
    </div>

    <div class="absolute right-5 top-5 flex items-center gap-2">
      <button
        type="button"
        class="flex h-10 w-10 items-center justify-center rounded-xl border border-gray-300 bg-white text-gray-500 transition hover:border-primary-400 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-primary-500 dark:hover:bg-primary-950/30 dark:hover:text-primary-300"
        :aria-label="t('modelPlaza.card.copyCallName')"
        @click.stop="copyCallName"
      >
        <Icon name="copy" size="sm" />
      </button>
      <button
        type="button"
        class="flex h-10 w-10 items-center justify-center rounded-xl border border-gray-300 bg-white text-gray-500 transition hover:border-primary-400 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-primary-500 dark:hover:bg-primary-950/30 dark:hover:text-primary-300"
        :aria-label="t('modelPlaza.card.viewDetails', { name: model.display_name })"
        @click.stop="detailsOpen = true"
      >
        <Icon name="more" size="sm" />
      </button>
    </div>

    <p class="model-description mt-5 text-base leading-7 text-gray-500 dark:text-dark-300">
      {{ descriptionText }}
    </p>

    <div class="mt-auto flex flex-wrap items-center justify-between gap-3 pt-5" @click.stop>
      <button
        v-if="hasTieredPricing"
        type="button"
        class="inline-flex items-center rounded-full border border-orange-300 bg-orange-50 px-3.5 py-1.5 text-sm font-medium text-orange-700 transition hover:bg-orange-100 dark:border-orange-400/40 dark:bg-orange-950/30 dark:text-orange-300 dark:hover:bg-orange-950/50"
        :aria-expanded="pricingOpen"
        @click.stop="pricingOpen = !pricingOpen"
      >
        {{ t('modelPlaza.card.tieredBilling') }}
      </button>
      <span v-else class="text-sm text-gray-400 dark:text-dark-500">
        {{ t('modelPlaza.card.standardBilling') }}
      </span>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <span
          v-for="category in model.categories"
          :key="category"
          class="rounded-full border border-gray-200 bg-gray-50 px-3.5 py-1.5 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-200"
        >
          {{ category }}
        </span>
      </div>
    </div>
    </article>

    <div
      v-if="pricingOpen"
      class="pricing-dropdown rounded-b-[22px] border border-t-0 border-primary-200 bg-white px-5 pb-5 pt-1 dark:border-primary-800/70 dark:bg-dark-800"
      @click.stop
    >
      <div v-if="model.price_table?.length" class="space-y-3 text-sm">
        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-full text-left">
            <thead class="bg-gray-50 dark:bg-dark-700/60">
              <tr>
                <th v-for="key in priceTableKeys" :key="key" class="whitespace-nowrap px-3 py-2 font-semibold text-gray-700 dark:text-dark-200">{{ key }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, rowIndex) in model.price_table" :key="rowIndex" class="border-t border-gray-100 dark:border-dark-700">
                <td v-for="key in priceTableKeys" :key="key" class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-dark-300">{{ row[key] || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="model.pricing_notes?.length" class="whitespace-pre-wrap text-gray-500 dark:text-dark-400">{{ model.pricing_notes.join('；') }}</p>
      </div>
      <div v-else class="grid gap-4 text-sm sm:grid-cols-2">
        <DetailItem :label="t('modelPlaza.card.inputPrice')" :value="displayMetadata(model.input_price)" />
        <DetailItem :label="t('modelPlaza.card.outputPrice')" :value="displayMetadata(model.output_price)" />
        <DetailItem :label="t('modelPlaza.card.cacheRead')" :value="displayMetadata(model.cache_read_price)" />
        <DetailItem :label="t('modelPlaza.card.cacheWrite')" :value="displayMetadata(model.cache_write_price)" />
      </div>
    </div>
  </div>

  <BaseDialog
    :show="detailsOpen"
    :title="model.display_name"
    width="wide"
    close-on-click-outside
    @close="detailsOpen = false"
  >
    <div class="space-y-5 text-sm">
      <div class="flex flex-wrap items-center gap-2">
        <code class="rounded-lg bg-gray-100 px-3 py-2 text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ model.name }}</code>
        <button type="button" class="btn btn-secondary btn-sm" @click="copyCallName">
          {{ t('modelPlaza.card.copyCallName') }}
        </button>
        <span v-for="category in model.categories" :key="category" class="rounded-full bg-primary-50 px-2.5 py-1 text-xs text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">
          {{ category }}
        </span>
      </div>
      <DetailItem :label="t('modelPlaza.card.capability')" :value="displayMetadata(model.capability)" />
      <DetailItem :label="t('modelPlaza.card.useCases')" :value="displayMetadata(model.use_cases)" />
      <DetailItem
        v-if="showTierCondition"
        :label="t('modelPlaza.card.tier')"
        :value="displayMetadata(model.tier_condition)"
      />
      <div class="grid gap-3 sm:grid-cols-2">
        <template v-if="!model.price_table?.length">
          <DetailItem :label="t('modelPlaza.card.inputPrice')" :value="displayMetadata(model.input_price)" />
          <DetailItem :label="t('modelPlaza.card.outputPrice')" :value="displayMetadata(model.output_price)" />
          <DetailItem :label="t('modelPlaza.card.cacheRead')" :value="displayMetadata(model.cache_read_price)" />
          <DetailItem :label="t('modelPlaza.card.cacheWrite')" :value="displayMetadata(model.cache_write_price)" />
        </template>
      </div>
      <div v-if="model.price_table?.length" class="space-y-3">
        <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-full text-left text-sm">
            <thead class="bg-gray-50 dark:bg-dark-700/60">
              <tr><th v-for="key in priceTableKeys" :key="key" class="whitespace-nowrap px-3 py-2 font-semibold text-gray-700 dark:text-dark-200">{{ key }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="(row, rowIndex) in model.price_table" :key="rowIndex" class="border-t border-gray-100 dark:border-dark-700">
                <td v-for="key in priceTableKeys" :key="key" class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-dark-300">{{ row[key] || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <DetailItem v-if="model.pricing_notes?.length" :label="t('modelPlaza.card.pricingNotes')" :value="model.pricing_notes.join('；')" />
      </div>
      <DetailItem :label="t('modelPlaza.card.glossary')" :value="displayMetadata(model.glossary)" />
    </div>
  </BaseDialog>

</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaModel } from '@/api/modelPlaza'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelPlazaPlatformIcon from './ModelPlazaPlatformIcon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import type { GroupPlatform } from '@/types'
import { platformLabel, platformTextClass } from '@/utils/platformColors'

const props = defineProps<{ model: PlazaModel }>()
const { t } = useI18n()
const appStore = useAppStore()
const detailsOpen = ref(false)
const pricingOpen = ref(false)
const platformName = computed(() => platformLabel(props.model.platform))
const sharedPlatformIcons = new Set(['openai', 'gemini', 'antigravity', 'grok', 'seedance', 'kimi', 'minimax', 'opencode_go'])
const useSharedPlatformIcon = computed(() => sharedPlatformIcons.has(props.model.platform))
const platformIconClass = computed(() =>
  props.model.platform === 'openai' ? 'text-gray-900 dark:text-white' : platformTextClass(props.model.platform)
)

function displayMetadata(value: string | null | undefined): string {
  const normalized = value?.trim()
  return !normalized || normalized === '待补充' ? '-' : normalized
}

const descriptionText = computed(() => {
  return displayMetadata(props.model.capability || props.model.use_cases)
})

// Seedance carries its provider-specific resolution pricing in price_table;
// the generic metadata tier label is usually just "见价格表" and adds no value.
const showTierCondition = computed(() =>
  props.model.platform !== 'seedance' && Boolean(props.model.tier_condition?.trim())
)

const hasTieredPricing = computed(() => {
  return Boolean(props.model.price_table?.length) || Boolean(props.model.tier_condition?.trim()) || (props.model.pricing?.intervals?.length ?? 0) > 1
})

const priceTableKeys = computed(() => {
  const rows = props.model.price_table ?? []
  const keys: string[] = []
  for (const row of rows) {
    for (const key of Object.keys(row)) if (!keys.includes(key)) keys.push(key)
  }
  return keys
})

const DetailItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(item) {
    return () => h('div', {}, [
      h('h3', { class: 'font-semibold text-gray-900 dark:text-white' }, item.label),
      h('p', { class: 'mt-1 whitespace-pre-wrap leading-6 text-gray-600 dark:text-dark-300' }, item.value)
    ])
  }
})

async function copyCallName() {
  await navigator.clipboard.writeText(props.model.name)
  appStore.showSuccess(t('modelPlaza.card.copySuccess'))
}
</script>

<style scoped>
.model-card {
  min-height: 15rem;
}

.model-card.pricing-open {
  border-bottom-width: 0;
  border-bottom-left-radius: 0;
  border-bottom-right-radius: 0;
  box-shadow: none;
}

.pricing-dropdown {
  animation: pricing-dropdown-in 0.16s ease-out;
}

@keyframes pricing-dropdown-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.model-description {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
</style>
