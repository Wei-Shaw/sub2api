<template>
  <div class="code-panel">
    <div class="code-panel-header">
      <div>
        <p class="code-panel-title">{{ title }}</p>
        <p v-if="description" class="code-panel-desc">{{ description }}</p>
      </div>
      <button type="button" class="copy-button" @click="$emit('copy')">
        {{ copied ? copiedLabel : copyLabel }}
      </button>
    </div>
    <pre class="code-pre"><code>{{ code }}</code></pre>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  description?: string
  code: string
  copied: boolean
  copyLabel: string
  copiedLabel: string
}>()

defineEmits<{
  copy: []
}>()
</script>
