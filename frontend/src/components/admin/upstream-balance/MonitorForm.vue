<template>
  <BaseDialog :show="show" :title="monitor ? t('admin.upstreamBalance.editTitle') : t('admin.upstreamBalance.addTitle')" width="wide" @close="$emit('close')">
    <form id="upstream-balance-form" class="space-y-4" @submit.prevent="submit">
      <div><label class="input-label">{{ t('admin.upstreamBalance.form.name') }} *</label><input v-model.trim="form.name" class="input" required maxlength="100" /></div>
      <div><label class="input-label">{{ t('admin.upstreamBalance.form.type') }} *</label><select v-model="form.type" class="input"><option value="sub2api">Sub2API</option><option value="newapi">New-API</option></select></div>
      <div><label class="input-label">{{ t('admin.upstreamBalance.form.baseUrl') }} *</label><input v-model.trim="form.base_url" class="input" type="url" required placeholder="https://api.example.com" /></div>
      <div><label class="input-label">{{ t('admin.upstreamBalance.form.credentialMode') }}</label><select v-model="form.credential_mode" class="input"><option value="password">{{ t('admin.upstreamBalance.form.passwordMode') }}</option><option value="token">{{ t('admin.upstreamBalance.form.tokenMode') }}</option></select></div>
      <template v-if="form.credential_mode === 'password'">
        <div>
          <label class="input-label">{{ form.type === 'sub2api' ? t('admin.upstreamBalance.form.email') : t('admin.upstreamBalance.form.username') }}<span v-if="credentialsRequired"> *</span></label>
          <input v-model.trim="form.username" class="input" :required="credentialsRequired" autocomplete="username" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreamBalance.form.password') }}<span v-if="credentialsRequired"> *</span></label>
          <input v-model="form.password" class="input" type="password" :required="credentialsRequired" autocomplete="current-password" :placeholder="monitor && monitor.credential_mode === form.credential_mode ? '****' : ''" />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.upstreamBalance.form.passwordHelp') }}</p>
        </div>
      </template>
      <div v-else-if="form.type === 'sub2api'">
        <label class="input-label">{{ t('admin.upstreamBalance.form.accessToken') }}<span v-if="credentialsRequired"> *</span></label>
        <input v-model="form.api_key" class="input" type="password" :required="credentialsRequired" :placeholder="monitor && monitor.type === form.type ? monitor.api_key_masked : t('admin.upstreamBalance.form.accessTokenPlaceholder')" />
        <p class="mt-1 text-xs text-gray-500">{{ t('admin.upstreamBalance.form.accessTokenHelp') }}</p>
      </div>
      <template v-else>
        <div>
          <label class="input-label">{{ t('admin.upstreamBalance.form.cookie') }}<span v-if="credentialsRequired"> *</span></label>
          <input v-model="form.cookie" class="input" type="password" :required="credentialsRequired" :placeholder="monitor && monitor.type === form.type ? monitor.api_key_masked : 'session=...'" />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.upstreamBalance.form.cookieHelp') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstreamBalance.form.userId') }}<span v-if="credentialsRequired"> *</span></label>
          <input v-model.trim="form.user_id" class="input" :required="credentialsRequired" placeholder="123" />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.upstreamBalance.form.userIdHelp') }}</p>
        </div>
      </template>
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="input-label">{{ t('admin.upstreamBalance.form.interval') }}</label><input v-model.number="form.probe_interval_minutes" class="input" type="number" min="5" max="1440" required /></div>
        <div><label class="input-label">{{ t('admin.upstreamBalance.form.order') }}</label><input v-model.number="form.display_order" class="input" type="number" /></div>
      </div>
      <div><label class="input-label">{{ t('admin.upstreamBalance.form.lowThreshold') }}</label><input v-model.number="form.low_balance_threshold_usd" class="input" type="number" min="0" step="0.01" /></div>
      <div class="flex items-center justify-between"><label class="input-label mb-0">{{ t('admin.upstreamBalance.form.enabled') }}</label><Toggle v-model="form.enabled" /></div>
    </form>
    <template #footer>
      <div class="flex justify-between gap-3">
        <button v-if="monitor" type="button" class="btn btn-danger" :disabled="submitting" @click="$emit('delete', monitor)">{{ t('common.delete') }}</button><span v-else />
        <div class="flex gap-3"><button type="button" class="btn btn-secondary" @click="$emit('close')">{{ t('common.cancel') }}</button><button form="upstream-balance-form" class="btn btn-primary" :disabled="submitting">{{ submitting ? t('common.submitting') : t('common.save') }}</button></div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { UpstreamBalanceMonitor, UpstreamBalanceMonitorInput } from '@/api/admin/upstreamBalance'

const props = defineProps<{ show: boolean; monitor: UpstreamBalanceMonitor | null; submitting?: boolean }>()
const emit = defineEmits<{ close: []; save: [input: UpstreamBalanceMonitorInput]; delete: [monitor: UpstreamBalanceMonitor] }>()
const { t } = useI18n()
const form = reactive<UpstreamBalanceMonitorInput>({ name: '', type: 'sub2api', base_url: '', credential_mode: 'password', username: '', password: '', api_key: '', cookie: '', user_id: '', enabled: true, display_order: 0, probe_interval_minutes: 30, low_balance_threshold_usd: 10 })
const credentialsRequired = computed(() => !props.monitor || props.monitor.type !== form.type || props.monitor.credential_mode !== form.credential_mode)

watch(() => [props.show, props.monitor] as const, () => {
  const m = props.monitor
  Object.assign(form, m ? { name: m.name, type: m.type, base_url: m.base_url, credential_mode: m.credential_mode || 'token', username: m.username || '', password: '', api_key: '', cookie: '', user_id: '', enabled: m.enabled, display_order: m.display_order, probe_interval_minutes: m.probe_interval_minutes, low_balance_threshold_usd: m.low_balance_threshold_usd } : { name: '', type: 'sub2api', base_url: '', credential_mode: 'password', username: '', password: '', api_key: '', cookie: '', user_id: '', enabled: true, display_order: 0, probe_interval_minutes: 30, low_balance_threshold_usd: 10 })
}, { immediate: true })

function submit() { emit('save', { ...form }) }
</script>
