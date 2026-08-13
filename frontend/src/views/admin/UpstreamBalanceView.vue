<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <div class="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
        <div><h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('admin.upstreamBalance.title') }}</h1><p class="mt-1 text-sm text-gray-500">{{ t('admin.upstreamBalance.description') }}</p></div>
        <div class="flex flex-wrap items-center gap-3">
          <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300"><Toggle v-model="enabledOnly" />{{ t('admin.upstreamBalance.enabledOnly') }}</label>
          <button class="btn btn-secondary" :disabled="refreshingAll" @click="refreshAll">{{ refreshingAll ? t('common.loading') : t('admin.upstreamBalance.refreshAll') }}</button>
          <button class="btn btn-primary" @click="openCreate">{{ t('admin.upstreamBalance.add') }}</button>
        </div>
      </div>

      <div v-if="!loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"><p class="text-sm text-gray-500">{{ t('admin.upstreamBalance.channelStatus') }}</p><p class="mt-2 text-2xl font-bold">{{ healthyCount }} / {{ monitors.length }}</p><p class="mt-1 text-sm text-emerald-600">{{ healthyCount }} {{ t('admin.upstreamBalance.healthy') }}</p></div>
        <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"><p class="text-sm text-gray-500">{{ t('admin.upstreamBalance.todayChanges') }}</p><p class="mt-2 text-2xl font-bold">{{ todayChanges.length }}</p><p class="mt-1 text-sm text-gray-500">{{ todayChanges.length ? t('admin.upstreamBalance.changedToday') : t('admin.upstreamBalance.noChangesToday') }}</p></div>
        <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"><p class="text-sm text-gray-500">{{ t('admin.upstreamBalance.totalGroups') }}</p><p class="mt-2 text-2xl font-bold">{{ totalGroups }}</p></div>
      </div>

      <div v-if="recentChanges.length" class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.upstreamBalance.recentChanges') }}</h2>
        <div class="mt-4 divide-y divide-gray-100 dark:divide-dark-700">
          <div v-for="change in recentChanges" :key="`${change.channelId}-${change.group}-${change.changed_at}`" class="flex flex-wrap items-center justify-between gap-2 py-3 text-sm">
            <div><span class="font-medium">{{ change.channel }}</span><span class="mx-2 text-gray-400">·</span><span>{{ change.group }}</span></div>
            <div><span class="text-gray-500">{{ change.old_ratio.toFixed(2) }}</span><span class="mx-2">→</span><span class="font-semibold" :class="change.new_ratio > change.old_ratio ? 'text-red-500' : 'text-emerald-600'">{{ change.new_ratio.toFixed(2) }}</span><span class="ml-3 text-xs text-gray-400">{{ formatChangeTime(change.changed_at) }}</span></div>
          </div>
        </div>
      </div>

      <div v-if="loading" class="py-16 text-center text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="visibleMonitors.length" class="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
        <MonitorCard v-for="monitor in visibleMonitors" :key="monitor.id" :monitor="monitor" :probing="probingIds.has(monitor.id)" @edit="openEdit" @probe="probeOne" />
      </div>
      <div v-else class="rounded-xl border border-dashed border-gray-300 py-16 text-center dark:border-dark-600">
        <p class="text-gray-500">{{ t('admin.upstreamBalance.empty') }}</p><button class="btn btn-primary mt-4" @click="openCreate">{{ t('admin.upstreamBalance.add') }}</button>
      </div>
    </div>

    <MonitorForm :show="showForm" :monitor="editing" :submitting="submitting" @close="showForm = false" @save="save" @delete="askDelete" />
    <ConfirmDialog :show="showDelete" :title="t('common.delete')" :message="t('admin.upstreamBalance.deleteConfirm', { name: deleting?.name || '' })" :danger="true" :confirm-text="t('common.delete')" :cancel-text="t('common.cancel')" @confirm="confirmDelete" @cancel="showDelete = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import MonitorCard from '@/components/admin/upstream-balance/MonitorCard.vue'
import MonitorForm from '@/components/admin/upstream-balance/MonitorForm.vue'
import upstreamBalanceAPI, { type UpstreamBalanceMonitor, type UpstreamBalanceMonitorInput } from '@/api/admin/upstreamBalance'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const monitors = ref<UpstreamBalanceMonitor[]>([])
const loading = ref(false), refreshingAll = ref(false), submitting = ref(false), enabledOnly = ref(false)
const showForm = ref(false), showDelete = ref(false)
const editing = ref<UpstreamBalanceMonitor | null>(null), deleting = ref<UpstreamBalanceMonitor | null>(null)
const probingIds = reactive(new Set<number>())
const visibleMonitors = computed(() => enabledOnly.value ? monitors.value.filter(m => m.enabled) : monitors.value)
const healthyCount = computed(() => monitors.value.filter(m => m.enabled && m.last_probe_status === 'ok').length)
const totalGroups = computed(() => monitors.value.reduce((sum, m) => sum + (m.balance_display?.rates?.length || 0), 0))
const allChanges = computed(() => monitors.value.flatMap(m => (m.balance_display?.rate_changes || []).map(change => ({ ...change, channel: m.name, channelId: m.id }))).sort((a, b) => Date.parse(b.changed_at) - Date.parse(a.changed_at)))
const todayChanges = computed(() => { const now = new Date(); return allChanges.value.filter(c => { const d = new Date(c.changed_at); return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate() }) })
const recentChanges = computed(() => allChanges.value.slice(0, 20))
function formatChangeTime(value: string) { return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) }

async function load() { loading.value = true; try { monitors.value = await upstreamBalanceAPI.list() } catch (e) { appStore.showError(extractApiErrorMessage(e, t('admin.upstreamBalance.loadError'))) } finally { loading.value = false } }
function openCreate() { editing.value = null; showForm.value = true }
function openEdit(m: UpstreamBalanceMonitor) { editing.value = m; showForm.value = true }
async function save(input: UpstreamBalanceMonitorInput) {
  submitting.value = true
  try { if (editing.value) await upstreamBalanceAPI.update(editing.value.id, input); else await upstreamBalanceAPI.create(input); showForm.value = false; await load(); appStore.showSuccess(t('common.saved')) }
  catch (e) { appStore.showError(extractApiErrorMessage(e, t('admin.upstreamBalance.saveError'))) } finally { submitting.value = false }
}
async function probeOne(m: UpstreamBalanceMonitor) { probingIds.add(m.id); try { await upstreamBalanceAPI.probe(m.id); await load() } catch (e) { appStore.showError(extractApiErrorMessage(e, t('admin.upstreamBalance.probeError'))); await load() } finally { probingIds.delete(m.id) } }
async function refreshAll() { refreshingAll.value = true; try { await upstreamBalanceAPI.probeAll(); await load() } catch (e) { appStore.showError(extractApiErrorMessage(e, t('admin.upstreamBalance.probeError'))); await load() } finally { refreshingAll.value = false } }
function askDelete(m: UpstreamBalanceMonitor) { deleting.value = m; showDelete.value = true }
async function confirmDelete() { if (!deleting.value) return; try { await upstreamBalanceAPI.remove(deleting.value.id); showDelete.value = false; showForm.value = false; await load() } catch (e) { appStore.showError(extractApiErrorMessage(e, t('admin.upstreamBalance.deleteError'))) } }
onMounted(load)
</script>
