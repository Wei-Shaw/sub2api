<template>
  <div
    v-if="ctx.showPreBlockRuntimeCard.value"
    data-test="pre-block-runtime-cards"
    class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,520px)_minmax(0,1fr)]"
  >
    <RiskPreBlockSyncCard />
    <RiskPreBlockApiKeyLoadCard />
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { RiskControlKey } from './riskControlContext'
import RiskPreBlockSyncCard from './RiskPreBlockSyncCard.vue'
import RiskPreBlockApiKeyLoadCard from './RiskPreBlockApiKeyLoadCard.vue'

const ctx = inject(RiskControlKey)!
</script>
