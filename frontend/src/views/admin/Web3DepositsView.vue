<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-3">
        <div class="card p-4"><div class="text-sm text-gray-500">Runtime</div><div class="mt-1 font-semibold">{{ runtime?.state || '—' }} · lag {{ runtime?.lag_blocks || '0' }}</div></div>
        <div class="card p-4"><div class="text-sm text-gray-500">Manual review</div><div class="mt-1 text-2xl font-semibold">{{ stats.manual_review || 0 }}</div></div>
        <div class="card p-4"><div class="text-sm text-gray-500">Failed</div><div class="mt-1 text-2xl font-semibold">{{ stats.failed || 0 }}</div></div>
      </div>
      <div class="card p-4">
        <div class="flex flex-wrap gap-2">
          <select v-model="status" class="input w-44"><option value="">All statuses</option><option value="manual_review">Manual review</option><option value="failed">Failed</option><option value="credited">Credited</option><option value="confirming">Confirming</option></select>
          <input v-model="keyword" class="input min-w-64 flex-1" placeholder="Transaction hash or address" />
          <button class="btn btn-primary" @click="load">Search</button>
          <button class="btn btn-secondary" @click="showRescan = true">Bounded rescan</button>
        </div>
      </div>
      <div class="card overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700"><thead><tr class="text-left text-xs uppercase text-gray-500"><th class="p-4">ID / User</th><th class="p-4">Amount</th><th class="p-4">Status</th><th class="p-4">Transaction</th><th class="p-4">Actions</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="item in items" :key="item.id"><td class="p-4 text-sm">#{{ item.id }}<div class="text-xs text-gray-500">User {{ item.user_id }}</div></td><td class="p-4 text-sm font-semibold">{{ item.token_amount }} USDT0</td><td class="p-4 text-sm"><span class="rounded-full bg-gray-100 px-2 py-1 dark:bg-dark-700">{{ item.status }}</span><div class="mt-1 max-w-xs text-xs text-red-500">{{ item.failure_reason || item.review_reason }}</div></td><td class="p-4 font-mono text-xs">{{ item.tx_hash.slice(0,12) }}…</td><td class="p-4"><div class="flex gap-2"><button v-if="item.status==='manual_review'" class="text-green-600" @click="approve(item)">Approve</button><button v-if="item.status==='manual_review'" class="text-gray-600" @click="ignore(item)">Ignore</button><button v-if="item.status==='failed'" class="text-primary-600" @click="retry(item)">Retry</button></div></td></tr></tbody></table>
      </div>
      <Pagination v-if="total" :page="page" :total="total" :page-size="pageSize" @update:page="p => { page=p; load() }" @update:pageSize="p => { pageSize=p; page=1; load() }" />
    </div>
    <BaseDialog :show="showRescan" title="Bounded rescan" @close="showRescan=false"><div class="space-y-3"><input v-model="fromBlock" class="input w-full" placeholder="From block" /><input v-model="toBlock" class="input w-full" placeholder="To block" /></div><template #footer><button class="btn btn-secondary" @click="showRescan=false">Cancel</button><button class="btn btn-primary" @click="rescan">Rescan</button></template></BaseDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import web3DepositsAPI, { type AdminWeb3Deposit, type Web3DepositRuntime } from '@/api/admin/web3Deposits'
import { useStepUp, isStepUpCancelled } from '@/composables/useStepUp'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
const app = useAppStore(); const stepUp = useStepUp(); const items=ref<AdminWeb3Deposit[]>([]); const stats=ref<Record<string,number>>({}); const runtime=ref<Web3DepositRuntime>(); const page=ref(1); const pageSize=ref(20); const total=ref(0); const status=ref(''); const keyword=ref(''); const showRescan=ref(false); const fromBlock=ref(''); const toBlock=ref('')
async function load(){ try { const params:Record<string,unknown>={page:page.value,page_size:pageSize.value,status:status.value}; if(keyword.value.startsWith('0x')&&keyword.value.length===66) params.tx_hash=keyword.value; else if(keyword.value) params.address=keyword.value; const [list,counts,state]=await Promise.all([web3DepositsAPI.list(params),web3DepositsAPI.stats(),web3DepositsAPI.runtime()]); items.value=list.data.items; total.value=list.data.total; stats.value=counts.data; runtime.value=state.data } catch(e){ app.showError(extractApiErrorMessage(e,'Failed to load Web3 deposits')) } }
async function run(action:()=>Promise<unknown>){ try { await stepUp.run(action); app.showSuccess('Operation completed'); await load() } catch(e){ if(!isStepUpCancelled(e)) app.showError(extractApiErrorMessage(e,'Operation failed')) } }
function approve(item:AdminWeb3Deposit){ if(confirm('Approve this finalized deposit?')) void run(()=>web3DepositsAPI.approve(item.id)) }
function ignore(item:AdminWeb3Deposit){ const reason=prompt('Ignore reason'); if(reason) void run(()=>web3DepositsAPI.ignore(item.id,reason)) }
function retry(item:AdminWeb3Deposit){ if(confirm('Retry this failed credit?')) void run(()=>web3DepositsAPI.retry(item.id)) }
function rescan(){ void run(()=>web3DepositsAPI.rescan(fromBlock.value,toBlock.value)).then(()=>{showRescan.value=false}) }
onMounted(load)
</script>
