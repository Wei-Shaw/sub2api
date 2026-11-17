<template>
  <button type="button" class="btn btn-secondary" :aria-pressed="fullUsageDate" @click="toggleUsageDate">{{ fullUsageDate ? labels.timeFull : labels.timeClock }}</button>
</template>
<script setup lang="ts">
import { usePersonalUsageLabels } from './index'
import { fullUsageDate, toggleUsageDate } from './timeDisplay'
const labels = usePersonalUsageLabels()
</script>
