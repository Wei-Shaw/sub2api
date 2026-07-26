<template>
  <AppLayout>
    <div class="space-y-5">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-xl font-semibold">{{ t('organization.admin.title') }}</h2>
      <div class="flex gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-800">
        <button class="rounded px-3 py-2 text-sm" :class="queue === 'applications' ? selectedClass : mutedClass" @click="selectQueue('applications')">{{ t('organization.admin.upgrades') }}</button>
        <button class="rounded px-3 py-2 text-sm" :class="queue === 'names' ? selectedClass : mutedClass" @click="selectQueue('names')">{{ t('organization.admin.nameChanges') }}</button>
		<button class="rounded px-3 py-2 text-sm" :class="queue === 'organizations' ? selectedClass : mutedClass" @click="selectQueue('organizations')">{{ t('organization.admin.organizations') }}</button>
      </div>
      <Select
        v-model="status"
        class="w-40"
        :options="statusOptions"
        :aria-label="t('common.status')"
        @change="load"
      />
    </div>

    <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

    <div v-if="queue === 'applications'" class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left dark:bg-dark-800"><tr><th class="p-3">{{ t('organization.companyName') }}</th><th class="p-3">{{ t('organization.admin.applicant') }}</th><th class="p-3">{{ t('organization.upgrade.chargedFee') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr></thead>
        <tbody><tr v-for="app in applications" :key="app.id" class="cursor-pointer border-t border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800" @click="openApplication(app)">
          <td class="p-3"><div class="font-medium">{{ app.requested_name }}</div><div v-if="app.similar_names?.length" class="mt-1 text-xs text-amber-600">{{ t('organization.admin.similar') }}: {{ app.similar_names.join(', ') }}</div></td>
          <td class="p-3">{{ app.applicant_email || app.applicant_user_id }}</td>
          <td data-testid="admin-upgrade-fee" class="p-3 font-mono">{{ formatUpgradeFee(app.fee_amount, app.fee_currency) }}</td>
          <td class="p-3">{{ t(`organization.status.${app.status}`) }}</td>
          <td class="p-3" @click.stop><template v-if="app.status === 'pending'"><button class="btn btn-primary btn-sm" @click="decideApplication(app, 'approve')">{{ t('organization.admin.approve') }}</button><button class="btn btn-ghost btn-sm text-red-600" @click="openReject(app)">{{ t('organization.admin.reject') }}</button></template><span v-else>{{ app.review_reason || '-' }}</span></td>
        </tr></tbody>
      </table>
    </div>

	<div v-else-if="queue === 'names'" class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left dark:bg-dark-800"><tr><th class="p-3">{{ t('organization.admin.currentName') }}</th><th class="p-3">{{ t('organization.admin.requestedName') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr></thead>
        <tbody><tr v-for="request in nameChanges" :key="request.id" class="border-t border-gray-100 dark:border-dark-700">
          <td class="p-3">{{ request.old_name }}</td><td class="p-3"><div class="font-medium">{{ request.new_name }}</div><div v-if="request.similar_names.length" class="mt-1 text-xs text-amber-600">{{ t('organization.admin.similar') }}: {{ request.similar_names.join(', ') }}</div></td><td class="p-3">{{ t(`organization.status.${request.status}`) }}</td>
          <td class="p-3"><template v-if="request.status === 'pending'"><button class="btn btn-primary btn-sm" @click="decideName(request, 'approve')">{{ t('organization.admin.approve') }}</button><button class="btn btn-ghost btn-sm text-red-600" @click="openReject(request)">{{ t('organization.admin.reject') }}</button></template><span v-else>{{ request.review_reason || '-' }}</span></td>
        </tr></tbody>
      </table>
    </div>

	<div v-else class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
		<table class="w-full text-sm">
			<thead class="bg-gray-50 text-left dark:bg-dark-800"><tr><th class="p-3">{{ t('organization.companyName') }}</th><th class="p-3">{{ t('organization.accountId') }}</th><th class="p-3">{{ t('organization.admin.members') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr></thead>
			<tbody><tr v-for="organization in organizations" :key="organization.id" class="cursor-pointer border-t border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800" @click="openOrganization(organization)">
				<td class="p-3"><div class="font-medium">{{ organization.name }}</div><div class="text-xs text-gray-500">{{ organization.owner_email || organization.owner_user_id }}</div></td>
				<td class="p-3 font-mono text-xs">{{ organization.account_id }}</td>
				<td class="p-3">{{ organization.member_count }}/{{ organization.member_limit }}</td>
				<td class="p-3">{{ t(`organization.status.${organization.status}`) }}</td>
				<td class="p-3" @click.stop><button v-if="organization.status === 'active'" class="btn btn-ghost btn-sm text-red-600" @click="setOrganizationStatus(organization, 'suspended')">{{ t('organization.admin.suspend') }}</button><button v-else class="btn btn-primary btn-sm" @click="setOrganizationStatus(organization, 'active')">{{ t('organization.admin.reactivate') }}</button></td>
			</tr></tbody>
		</table>
	</div>

    <div v-if="detail" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4" @click.self="detail = undefined">
      <section data-testid="company-application-detail" class="max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
        <div class="flex items-start justify-between gap-3"><div><h3 class="font-semibold">{{ detail.application.requested_name }}</h3><p class="mt-1 font-mono text-xs text-gray-500">#{{ detail.application.id }}</p></div><button class="icon-btn" :aria-label="t('common.close')" @click="detail = undefined"><Icon name="x" size="sm" /></button></div>
        <dl class="mt-5 grid gap-4 sm:grid-cols-2"><div><dt class="text-xs text-gray-500">{{ t('organization.admin.applicant') }}</dt><dd class="mt-1">{{ detail.application.applicant_email || detail.application.applicant_user_id }}</dd></div><div><dt class="text-xs text-gray-500">{{ t('organization.upgrade.chargedFee') }}</dt><dd data-testid="admin-upgrade-detail-fee" class="mt-1 font-mono">{{ formatUpgradeFee(detail.application.fee_amount, detail.application.fee_currency) }}</dd></div></dl>
        <div v-if="detail.application.similar_names.length" class="mt-4 text-sm text-amber-600">{{ t('organization.admin.similar') }}: {{ detail.application.similar_names.join(', ') }}</div>
        <h4 class="mt-6 text-sm font-medium">{{ t('organization.admin.audit') }}</h4>
        <ol class="mt-2 divide-y divide-gray-100 text-sm dark:divide-dark-700"><li v-for="event in detail.audit" :key="event.id" class="py-3"><div class="flex justify-between gap-3"><span>{{ event.action }} · {{ event.result }}</span><time class="text-xs text-gray-500">{{ new Date(event.created_at).toLocaleString() }}</time></div><div v-if="event.correlation_id" class="mt-1 font-mono text-xs text-gray-500">{{ event.correlation_id }}</div></li></ol>
      </section>
    </div>

	<div v-if="organizationDetail" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4" @click.self="organizationDetail = undefined">
		<section class="max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
			<div class="flex items-start justify-between gap-3"><div><h3 class="font-semibold">{{ organizationDetail.organization.name }}</h3><p class="mt-1 font-mono text-xs text-gray-500">{{ organizationDetail.organization.account_id }}</p></div><button class="icon-btn" :aria-label="t('common.close')" @click="organizationDetail = undefined"><Icon name="x" size="sm" /></button></div>
			<dl class="mt-5 grid gap-4 sm:grid-cols-2"><div><dt class="text-xs text-gray-500">{{ t('organization.admin.members') }}</dt><dd class="mt-1">{{ organizationDetail.organization.member_count }}/{{ organizationDetail.organization.member_limit }}</dd></div><div><dt class="text-xs text-gray-500">{{ t('common.status') }}</dt><dd class="mt-1">{{ t(`organization.status.${organizationDetail.organization.status}`) }}</dd></div></dl>
			<h4 class="mt-6 text-sm font-medium">{{ t('organization.admin.audit') }}</h4>
			<ol class="mt-2 divide-y divide-gray-100 text-sm dark:divide-dark-700"><li v-for="event in organizationDetail.audit" :key="event.id" class="py-3"><div class="flex justify-between gap-3"><span>{{ event.action }} · {{ event.result }}</span><time class="text-xs text-gray-500">{{ new Date(event.created_at).toLocaleString() }}</time></div><div v-if="event.correlation_id" class="mt-1 font-mono text-xs text-gray-500">{{ event.correlation_id }}</div></li></ol>
		</section>
	</div>

    <div v-if="rejecting" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4"><form class="w-full max-w-md space-y-4 rounded-md bg-white p-5 dark:bg-dark-800" @submit.prevent="submitReject"><h3 class="font-semibold">{{ t('organization.admin.reject') }}</h3><textarea v-model.trim="reason" class="input min-h-28" required maxlength="1000" /><div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="rejecting = undefined">{{ t('common.cancel') }}</button><button class="btn btn-danger" type="submit">{{ t('organization.admin.reject') }}</button></div></form></div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationAPI } from '@/api'
import Select from '@/components/common/Select.vue'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { AdminOrganization, AdminOrganizationDetail, CompanyApplication, CompanyApplicationDetail, OrganizationNameChangeRequest } from '@/types'

const { t } = useI18n()
const selectedClass = 'bg-white font-medium shadow-sm dark:bg-dark-700'
const mutedClass = 'text-gray-600 dark:text-dark-300'
const queue = ref<'applications' | 'names' | 'organizations'>('applications')
const applications = ref<CompanyApplication[]>([])
const nameChanges = ref<OrganizationNameChangeRequest[]>([])
const organizations = ref<AdminOrganization[]>([])
const status = ref('pending')
const detail = ref<CompanyApplicationDetail>()
const organizationDetail = ref<AdminOrganizationDetail>()
const rejecting = ref<CompanyApplication | OrganizationNameChangeRequest>()
const reason = ref('')
const error = ref('')
const statusOptions = computed(() => queue.value === 'organizations'
  ? [
      { value: '', label: t('common.all') },
      { value: 'active', label: t('organization.status.active') },
      { value: 'suspended', label: t('organization.status.suspended') },
    ]
  : [
      { value: '', label: t('common.all') },
      { value: 'pending', label: t('organization.status.pending') },
      { value: 'approved', label: t('organization.status.approved') },
      { value: 'rejected', label: t('organization.status.rejected') },
    ])

function formatUpgradeFee(amount: string | number | undefined, currency: string): string {
  const parsed = Number(amount)
  const formattedAmount = Number.isFinite(parsed) ? parsed.toFixed(2) : '0.00'
  const normalizedCurrency = currency.trim().toUpperCase()
  return normalizedCurrency === 'USD' ? `$${formattedAmount}` : `${formattedAmount} ${normalizedCurrency}`
}

async function load() {
  error.value = ''
  try {
    if (queue.value === 'applications') applications.value = (await organizationAPI.listApplications({ status: status.value || undefined })).items
	else if (queue.value === 'names') nameChanges.value = (await organizationAPI.listNameChanges({ status: status.value || undefined })).items
	else organizations.value = (await organizationAPI.listOrganizations({ status: status.value || undefined })).items
  } catch (cause) { error.value = (cause as { message?: string }).message || t('common.error') }
}
function selectQueue(value: 'applications' | 'names' | 'organizations') { queue.value = value; status.value = value === 'organizations' ? '' : 'pending'; void load() }
async function openApplication(app: CompanyApplication) { detail.value = await organizationAPI.getApplication(app.id) }
async function openOrganization(organization: AdminOrganization) { organizationDetail.value = await organizationAPI.getOrganization(organization.id) }
async function setOrganizationStatus(organization: AdminOrganization, nextStatus: 'active' | 'suspended') { error.value = ''; try { await organizationAPI.setOrganizationStatus(organization.id, nextStatus); await load() } catch (cause) { error.value = (cause as { message?: string }).message || t('common.error') } }
async function decideApplication(app: CompanyApplication, decision: 'approve' | 'reject', reviewReason = '') { error.value = ''; try { await organizationAPI.decideApplication(app.id, decision, reviewReason); await load() } catch (cause) { error.value = (cause as { message?: string }).message || t('common.error') } }
async function decideName(request: OrganizationNameChangeRequest, decision: 'approve' | 'reject', reviewReason = '') { error.value = ''; try { await organizationAPI.decideNameChange(request.id, decision, reviewReason); await load() } catch (cause) { error.value = (cause as { message?: string }).message || t('common.error') } }
function openReject(item: CompanyApplication | OrganizationNameChangeRequest) { rejecting.value = item; reason.value = '' }
async function submitReject() { if (!rejecting.value || !reason.value) return; if ('requested_name' in rejecting.value) await decideApplication(rejecting.value, 'reject', reason.value); else await decideName(rejecting.value, 'reject', reason.value); rejecting.value = undefined }
onMounted(load)
</script>
