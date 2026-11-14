<template>
  <div class="card">
    <div class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ tr('额度单位管理', 'Quota units') }}</h2>
        <p class="mt-1 text-sm text-gray-500">{{ tr('定义额度的展示单位，并在分组中选择使用。', 'Define how quotas are displayed, then choose a unit for each group.') }}</p>
      </div>
      <button type="button" class="btn btn-primary" @click="openNew">{{ tr('添加单位', 'Add unit') }}</button>
    </div>
    <div class="p-6">
      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <table class="w-full text-left text-sm">
          <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800"><tr>
            <th class="px-4 py-3">{{ tr('单位名称', 'Unit') }}</th>
            <th class="px-4 py-3">{{ tr('换算关系', 'Conversion') }}</th>
            <th class="px-4 py-3 text-right">{{ tr('操作', 'Actions') }}</th>
          </tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr><td class="px-4 py-4 font-medium">{{ tr('美元', 'US dollar') }} <span class="ml-2 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700">{{ tr('内置', 'Built in') }}</span></td>
              <td class="px-4 py-4 text-gray-500">$1 = 1 USD</td><td class="px-4 py-4 text-right text-gray-400">—</td></tr>
            <tr v-for="unit in modelValue" :key="unit.id">
              <td class="px-4 py-4 font-medium">{{ unit.name }}</td>
              <td class="px-4 py-4 tabular-nums">$1 = {{ unit.units_per_usd }} {{ unit.name }}</td>
              <td class="px-4 py-4"><div class="flex justify-end gap-3">
                <button type="button" class="text-primary-600 hover:underline" @click="openEdit(unit)">{{ tr('编辑', 'Edit') }}</button>
                <button type="button" class="text-red-500 hover:underline" @click="remove(unit.id)">{{ tr('删除', 'Remove') }}</button>
              </div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="mt-3 text-xs leading-5 text-gray-500">{{ tr('例如：1 美元 = 100 积分，原价 $0.15/次将显示为 15 积分/次。修改比例后，相关分组的价格、总额度和已用额度会一起换算，已有额度的实际价值不变。', 'Example: at 100 points per USD, $0.15/request displays as 15 points/request. Rate changes rescale prices, quotas and usage together without changing their underlying value.') }}</p>
      <p class="mt-1 text-xs text-gray-500">{{ tr('修改后请保存设置。使用中的单位须先在相关分组中替换，才能删除。', 'Save settings to apply changes. Replace a unit in all groups before removing it.') }}</p>
    </div>
    <BaseDialog :show="editing" :title="draft.id ? tr('编辑额度单位', 'Edit quota unit') : tr('添加额度单位', 'Add quota unit')" @close="editing = false">
      <div class="space-y-5">
        <div><label class="input-label" for="quota-unit-name">{{ tr('单位名称', 'Name') }}</label>
          <input id="quota-unit-name" v-model="draft.name" class="input" maxlength="20" :placeholder="tr('例如：积分、代币', 'e.g. points, credits')" /></div>
        <div><label class="input-label" for="quota-unit-rate">{{ tr('换算比例', 'Conversion rate') }}</label>
          <div class="flex items-center gap-3"><span class="shrink-0 text-sm text-gray-500">$1 =</span>
            <input id="quota-unit-rate" v-model.number="draft.units_per_usd" type="number" step="any" min="0" max="1000000000000" class="input min-w-0" />
            <span class="shrink-0 text-sm">{{ draft.name || tr('单位', 'units') }}</span></div></div>
        <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800">
          <span class="text-gray-500">{{ tr('显示示例：', 'Preview: ') }}</span>
          $0.15 / {{ tr('次', 'request') }} → {{ preview }} {{ draft.name || tr('单位', 'units') }} / {{ tr('次', 'request') }}
        </div>
        <p v-if="error" role="alert" class="text-sm text-red-500">{{ error }}</p>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="editing = false">{{ tr('取消', 'Cancel') }}</button>
          <button type="button" class="btn btn-primary" @click="apply">{{ tr('确定', 'Apply') }}</button></div>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { SubscriptionUnit } from '@/types'
const props = defineProps<{ modelValue: SubscriptionUnit[] }>()
const emit = defineEmits<{ 'update:modelValue': [units: SubscriptionUnit[]] }>()
const { locale } = useI18n()
const tr = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const editing = ref(false)
const error = ref('')
const draft = reactive<SubscriptionUnit>({ id: '', name: '', units_per_usd: 100 })
const preview = computed(() => Number((0.15 * (Number(draft.units_per_usd) || 0)).toPrecision(10)))
function openNew() { Object.assign(draft, { id: '', name: '', units_per_usd: 100 }); error.value = ''; editing.value = true }
function openEdit(unit: SubscriptionUnit) { Object.assign(draft, unit); error.value = ''; editing.value = true }
function apply() {
  const name = draft.name.trim(); const rate = Number(draft.units_per_usd)
  if (!name || name.length > 20) { error.value = tr('请填写 1 到 20 个字符的单位名称。', 'Enter a name of 1–20 characters.'); return }
  if (props.modelValue.some(u => u.id !== draft.id && u.name.toLowerCase() === name.toLowerCase())) { error.value = tr('该单位名称已存在。', 'This name already exists.'); return }
  if (!Number.isFinite(rate) || rate <= 0 || rate > 1e12) { error.value = tr('换算比例须大于 0 且不超过 1 万亿。', 'Rate must be positive and no greater than 1 trillion.'); return }
  const unit = { id: draft.id || `u-${crypto.randomUUID().slice(0, 16)}`, name, units_per_usd: rate }
  emit('update:modelValue', draft.id ? props.modelValue.map(u => u.id === draft.id ? unit : u) : [...props.modelValue, unit])
  editing.value = false
}
function remove(id: string) { emit('update:modelValue', props.modelValue.filter(u => u.id !== id)) }
</script>
