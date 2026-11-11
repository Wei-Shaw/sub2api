<template>
  <main class="mx-auto max-w-6xl p-6 text-gray-900">
    <h1 class="text-xl font-semibold">Isolated prepaid funding test</h1>
    <p class="my-3 rounded-lg bg-sky-50 p-3 text-sm">Test-only PostgreSQL schema and deterministic vendor. No real provider keys, model calls, purchases or card charges are possible.</p>
    <div class="flex flex-wrap gap-2">
      <button class="btn btn-secondary" @click="fixture('submit')">Submit fixture video</button>
      <button class="btn btn-secondary" @click="fixture('succeed')">Mark fixture jobs succeeded</button>
      <button class="btn btn-secondary" @click="fixture('expire')">Expire fixture funding</button>
      <button class="btn btn-secondary" @click="fixture('counter')">Read upstream call counter</button>
    </div>
    <pre class="my-3 whitespace-pre-wrap rounded-lg bg-gray-100 p-3 text-xs" role="status">{{ outcome }}</pre>
    <BytePlusPrepaidFunding :account-id="11" account-name="Browser fixture pool" />
  </main>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import BytePlusPrepaidFunding from '../../src/components/account/BytePlusPrepaidFunding.vue'

const outcome = ref('No test action submitted.')
async function fixture(action: string) {
  const response = await fetch(`/fixture/${action}`, { method: action === 'counter' ? 'GET' : 'POST', headers: { Authorization: 'Bearer fixture-admin-not-production' } })
  outcome.value = `HTTP ${response.status}: ${await response.text()}`
}
</script>
