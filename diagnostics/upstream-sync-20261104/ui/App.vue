<script setup lang="ts">
import { ref } from 'vue'
import Field from '../src/features/admin-accounts/presentation/widgets/OpenAIImageSizeTiersField.vue'
import Bulk from '../src/features/admin-accounts/presentation/widgets/BulkEditOpenAIImageSizeOption.vue'
const tiers = ref(['1K', '2K', '4K'])
const update = ref(null)
</script>
<template>
  <main class="mx-auto max-w-2xl space-y-8 p-4">
    <section class="card p-4">
      <Field v-model="tiers" />
      <output aria-label="Selected tiers">{{ tiers.join(', ') }}</output>
    </section>
    <section class="card p-4">
      <Bulk v-model="update" />
      <output aria-label="Bulk update">{{ JSON.stringify(update) }}</output>
    </section>
  </main>
</template>
