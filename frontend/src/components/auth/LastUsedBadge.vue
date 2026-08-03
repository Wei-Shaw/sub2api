<template>
  <span
    class="pointer-events-none absolute -right-2 -top-2 z-10 rounded-full bg-primary-500 px-2 py-0.5 text-[10px] font-medium leading-tight text-white shadow-sm shadow-primary-500/30"
  >
    {{ t('auth.lastTimeUsed') }}
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
