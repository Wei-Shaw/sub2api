<template>
  <div v-if="quota && quota.source !== 'none'" class="sora-quota-info">
    <div class="sora-quota-bar-wrapper">
      <div
        class="sora-quota-bar-fill"
        :class="{ warning: percentage > 80, danger: percentage > 95 REDACTED"
        :style="{ width: `${Math.min(percentage, 100)REDACTED%` REDACTED"
      />
    </div>
    <span class="sora-quota-text" :class="{ warning: percentage > 80, danger: percentage > 95 REDACTED">
      {{ formatBytes(quota.used_bytes) REDACTEDREDACTED / {{ quota.quota_bytes === 0 ? '∞' : formatBytes(quota.quota_bytes) REDACTEDREDACTED
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { QuotaInfo REDACTED from '@/api/sora'

const props = defineProps<{ quota: QuotaInfo REDACTED>()

const percentage = computed(() => {
  if (!props.quota || props.quota.quota_bytes === 0) return 0
  return (props.quota.used_bytes / props.quota.quota_bytes) * 100
REDACTED)

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)REDACTED ${units[i]REDACTED`
REDACTED
</script>

<style scoped>
.sora-quota-info {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 14px;
  background: var(--sora-bg-secondary);
  border-radius: var(--sora-radius-full, 9999px);
  font-size: 12px;
  color: var(--sora-text-secondary, #A0A0A0);
REDACTED

.sora-quota-bar-wrapper {
  width: 80px;
  height: 4px;
  background: var(--sora-bg-hover, #333);
  border-radius: 2px;
  overflow: hidden;
REDACTED

.sora-quota-bar-fill {
  height: 100%;
  background: var(--sora-accent-gradient, linear-gradient(135deg, #14b8a6, #0d9488));
  border-radius: 2px;
  transition: width 400ms ease;
REDACTED

.sora-quota-bar-fill.warning {
  background: var(--sora-warning, #F59E0B) !important;
REDACTED

.sora-quota-bar-fill.danger {
  background: var(--sora-error, #EF4444) !important;
REDACTED

.sora-quota-text {
  white-space: nowrap;
REDACTED

.sora-quota-text.warning {
  color: var(--sora-warning, #F59E0B);
REDACTED

.sora-quota-text.danger {
  color: var(--sora-error, #EF4444);
REDACTED

@media (max-width: 900px) {
  .sora-quota-info {
    display: none;
  REDACTED
REDACTED
</style>
