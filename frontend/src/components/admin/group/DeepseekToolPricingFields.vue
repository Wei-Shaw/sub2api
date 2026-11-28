<template>
  <section class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-400">
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.groups.deepseekToolPricing.title') }}</h4>
    <p class="input-hint">{{ t('admin.groups.deepseekToolPricing.hint') }}</p>
    <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div v-for="{ tool, price, rule } in fields" :key="tool.id">
        <label class="input-label" :for="id + tool.id">{{ t('admin.groups.deepseekToolPricing.' + tool.id) }}</label>
        <input
          :id="id + tool.id" :name="'deepseek-tools-' + tool.id" type="number" min="0" step="any" class="input"
          :value="price" :placeholder="String(tool.usd)" :disabled="!!rule" :aria-describedby="id + tool.id + '-hint'"
          @input="update(tool, $event)"
        />
        <p :id="id + tool.id + '-hint'" class="input-hint">
          {{ t('admin.groups.deepseekToolPricing.reference', { usd: tool.usd, cny: tool.cny }) }}
          <span v-if="rule">{{ t('admin.groups.deepseekToolPricing.managedByRule', { rule }) }}</span>
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { deepseekTools, matchesToolModel, setDeepseekToolPrice, type DeepseekToolPrices } from './deepseekToolPricing'

const prices = defineModel<DeepseekToolPrices>({ required: true })
const props = defineProps<{ genericPricing: { models: string[]; billing_mode: string }[] }>()
const { t } = useI18n()
const id = useId()
const fields = computed(() => {
  const patterns = props.genericPricing.filter(entry => entry.billing_mode === 'per_request').flatMap(entry => entry.models)
  return deepseekTools.map(tool => {
    const unitPrice = prices.value[tool.id]?.per_request_price
    const rule = patterns.find(pattern => matchesToolModel(pattern, tool))
    return {
      tool,
      price: rule || unitPrice == null ? '' : Number((unitPrice * tool.units).toPrecision(15)),
      rule,
    }
  })
})

function update(tool: typeof deepseekTools[number], event: Event) {
  const input = event.target as HTMLInputElement
  if (input.validity.badInput) return
  prices.value = setDeepseekToolPrice(prices.value, tool, input.value === '' ? null : input.valueAsNumber)
}
</script>
