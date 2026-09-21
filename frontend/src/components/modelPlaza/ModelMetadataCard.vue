<template>
  <article
    class="group flex h-full cursor-pointer flex-col rounded-2xl border border-gray-200 bg-white p-5 shadow-sm transition hover:-translate-y-0.5 hover:border-primary-300 hover:shadow-card dark:border-dark-700 dark:bg-dark-800/70 dark:hover:border-primary-700"
    tabindex="0"
    role="button"
    :aria-label="t('modelPlaza.card.viewDetails', { name: model.display_name })"
    @click="detailsOpen = true"
    @keydown.enter="detailsOpen = true"
    @keydown.space.prevent="detailsOpen = true"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
          {{ model.display_name }}
        </h2>
        <div class="mt-1 flex items-center gap-1.5">
          <code class="truncate text-xs text-gray-500 dark:text-dark-400">{{ model.name }}</code>
          <button
            type="button"
            class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-300"
            :aria-label="t('modelPlaza.card.copyCallName')"
            @click.stop="copyCallName"
          >
            <Icon name="copy" size="xs" />
          </button>
        </div>
      </div>
    </div>

    <div class="mt-3 flex flex-wrap gap-1.5">
      <span
        v-for="category in model.categories"
        :key="category"
        class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300"
      >
        {{ category }}
      </span>
      <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">
        {{ model.tier_condition }}
      </span>
    </div>

    <dl class="mt-4 space-y-3 text-sm">
      <div>
        <dt class="font-medium text-gray-700 dark:text-dark-200">{{ t('modelPlaza.card.capability') }}</dt>
        <dd class="summary mt-1 text-gray-500 dark:text-dark-400">{{ model.capability }}</dd>
      </div>
      <div>
        <dt class="font-medium text-gray-700 dark:text-dark-200">{{ t('modelPlaza.card.useCases') }}</dt>
        <dd class="summary mt-1 text-gray-500 dark:text-dark-400">{{ model.use_cases }}</dd>
      </div>
    </dl>

    <div class="mt-5 grid grid-cols-2 gap-2 border-t border-gray-100 pt-4 text-xs dark:border-dark-700">
      <PriceItem :label="t('modelPlaza.card.inputPrice')" :value="model.input_price" />
      <PriceItem :label="t('modelPlaza.card.outputPrice')" :value="model.output_price" />
      <PriceItem :label="t('modelPlaza.card.cacheRead')" :value="model.cache_read_price" />
      <PriceItem :label="t('modelPlaza.card.cacheWrite')" :value="model.cache_write_price" />
    </div>
  </article>

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
      <DetailItem :label="t('modelPlaza.card.capability')" :value="model.capability" />
      <DetailItem :label="t('modelPlaza.card.useCases')" :value="model.use_cases" />
      <DetailItem :label="t('modelPlaza.card.tier')" :value="model.tier_condition" />
      <div class="grid gap-3 sm:grid-cols-2">
        <DetailItem :label="t('modelPlaza.card.inputPrice')" :value="model.input_price" />
        <DetailItem :label="t('modelPlaza.card.outputPrice')" :value="model.output_price" />
        <DetailItem :label="t('modelPlaza.card.cacheRead')" :value="model.cache_read_price" />
        <DetailItem :label="t('modelPlaza.card.cacheWrite')" :value="model.cache_write_price" />
      </div>
      <DetailItem :label="t('modelPlaza.card.glossary')" :value="model.glossary" />
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaModel } from '@/api/modelPlaza'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ model: PlazaModel }>()
const { t } = useI18n()
const appStore = useAppStore()
const detailsOpen = ref(false)

const PriceItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(item) {
    return () => h('div', { class: 'rounded-xl bg-gray-50 p-2.5 dark:bg-dark-900/50' }, [
      h('div', { class: 'text-gray-400 dark:text-dark-500' }, item.label),
      h('div', { class: 'mt-1 font-semibold text-gray-900 dark:text-white' }, item.value)
    ])
  }
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
.summary {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
</style>
