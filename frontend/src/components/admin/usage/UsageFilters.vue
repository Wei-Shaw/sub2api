<template>
  <div class="card p-6"><div class="flex flex-wrap items-end gap-4">
    <div class="min-w-[200px] relative"><label class="input-label">{{ t('admin.usage.userFilter') REDACTEDREDACTED</label>
      <input v-model="userKW" type="text" class="input pr-8" :placeholder="t('admin.usage.searchUserPlaceholder')" @input="debounceSearch" @focus="showDD = true" />
      <button v-if="modelValue.user_id" @click="clearUser" class="absolute right-2 top-9 text-gray-400">✕</button>
      <div v-if="showDD && (results.length > 0 || userKW)" class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-gray-800">
        <button v-for="u in results" :key="u.id" @click="selectUser(u)" class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-700"><span>{{ u.email REDACTEDREDACTED</span><span class="ml-2 text-xs text-gray-400">#{{ u.id REDACTEDREDACTED</span></button>
      </div>
    </div>
    <div class="min-w-[180px]"><label class="input-label">{{ t('usage.model') REDACTEDREDACTED</label><Select v-model="filters.model" :options="mOpts" searchable @change="emitChange" /></div>
    <div class="min-w-[150px]"><label class="input-label">{{ t('admin.usage.group') REDACTEDREDACTED</label><Select v-model="filters.group_id" :options="gOpts" @change="emitChange" /></div>
    <div><label class="input-label">{{ t('usage.timeRange') REDACTEDREDACTED</label><DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="emitChange" /></div>
    <div class="ml-auto flex gap-3"><button @click="$emit('reset')" class="btn btn-secondary">{{ t('common.reset') REDACTEDREDACTED</button><button @click="$emit('export')" :disabled="exporting" class="btn btn-primary">{{ t('usage.exportExcel') REDACTEDREDACTED</button></div>
  </div></div>
</template>

<script setup lang="ts">
import { ref, onMounted REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import { adminAPI REDACTED from '@/api/admin'; import Select from '@/components/common/Select.vue'; import DateRangePicker from '@/components/common/DateRangePicker.vue'
const props = defineProps(['modelValue', 'exporting', 'startDate', 'endDate']); const emit = defineEmits(['update:modelValue', 'update:startDate', 'update:endDate', 'change', 'reset', 'export'])
const { t REDACTED = useI18n(); const filters = props.modelValue
const userKW = ref(''); const results = ref<any[]>([]); const showDD = ref(false); let timeout: any = null
const mOpts = ref<{value: string | null, label: stringREDACTED[]>([{ value: null, label: t('admin.usage.allModels') REDACTED]); const gOpts = ref<any[]>([{ value: null, label: t('admin.usage.allGroups') REDACTED])
const emitChange = () => emit('change')
const debounceSearch = () => { clearTimeout(timeout); timeout = setTimeout(async () => { if(!userKW.value) { results.value = []; return REDACTED; try { results.value = await adminAPI.usage.searchUsers(userKW.value) REDACTED catch {REDACTED REDACTED, 300) REDACTED
const selectUser = (u: any) => { userKW.value = u.email; showDD.value = false; filters.user_id = u.id; emitChange() REDACTED
const clearUser = () => { userKW.value = ''; results.value = []; filters.user_id = undefined; emitChange() REDACTED
onMounted(async () => {
  try { const [gs, ms] = await Promise.all([adminAPI.groups.list(1, 1000), adminAPI.dashboard.getModelStats({ start_date: props.startDate, end_date: props.endDate REDACTED)])
    gOpts.value.push(...gs.items.map((g: any) => ({ value: g.id, label: g.name REDACTED)))
    const unique = new Set<string>(); ms.models?.forEach((s: any) => s.model && unique.add(s.model))
    mOpts.value.push(...Array.from(unique).sort().map(m => ({ value: m, label: m REDACTED)))
  REDACTED catch {REDACTED
  document.addEventListener('click', (e) => { if(!(e.target as HTMLElement).closest('.relative')) showDD.value = false REDACTED)
REDACTED)
</script>
