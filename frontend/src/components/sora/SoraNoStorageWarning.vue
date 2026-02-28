<template>
  <div class="sora-no-storage-warning">
    <span>⚠️</span>
    <div>
      <p class="sora-no-storage-title">{{ t('sora.noStorageWarningTitle') REDACTEDREDACTED</p>
      <p class="sora-no-storage-desc">{{ t('sora.noStorageWarningDesc') REDACTEDREDACTED</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()
</script>

<style scoped>
.sora-no-storage-warning {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 14px 20px;
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.2);
  border-radius: 12px;
  font-size: 13px;
REDACTED

.sora-no-storage-title {
  font-weight: 600;
  color: var(--sora-warning, #F59E0B);
  margin-bottom: 4px;
REDACTED

.sora-no-storage-desc {
  color: var(--sora-text-secondary, #A0A0A0);
  line-height: 1.5;
REDACTED
</style>
