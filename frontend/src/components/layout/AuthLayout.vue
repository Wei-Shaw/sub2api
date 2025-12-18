<template>
  <div class="min-h-screen flex items-center justify-center p-4 relative overflow-hidden">
    <!-- Background -->
    <div class="absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"></div>

    <!-- Decorative Elements -->
    <div class="absolute inset-0 overflow-hidden pointer-events-none">
      <!-- Gradient Orbs -->
      <div class="absolute -top-40 -right-40 w-80 h-80 bg-primary-400/20 rounded-full blur-3xl"></div>
      <div class="absolute -bottom-40 -left-40 w-80 h-80 bg-primary-500/15 rounded-full blur-3xl"></div>
      <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-96 h-96 bg-primary-300/10 rounded-full blur-3xl"></div>

      <!-- Grid Pattern -->
      <div class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"></div>
    </div>

    <!-- Content Container -->
    <div class="relative w-full max-w-md z-10">
      <!-- Logo/Brand -->
      <div class="text-center mb-8">
        <!-- Custom Logo or Default Logo -->
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-2xl overflow-hidden shadow-lg shadow-primary-500/30 mb-4">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="w-full h-full object-contain" />
        </div>
        <h1 class="text-3xl font-bold text-gradient mb-2">
          {{ siteName REDACTEDREDACTED
        </h1>
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ siteSubtitle REDACTEDREDACTED
        </p>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="text-center mt-6 text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="text-center mt-8 text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear REDACTEDREDACTED {{ siteName REDACTEDREDACTED. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted REDACTED from 'vue';
import { getPublicSettings REDACTED from '@/api/auth';

const siteName = ref('Sub2API');
const siteLogo = ref('');
const siteSubtitle = ref('Subscription to API Conversion Platform');

const currentYear = computed(() => new Date().getFullYear());

onMounted(async () => {
  try {
    const settings = await getPublicSettings();
    siteName.value = settings.site_name || 'Sub2API';
    siteLogo.value = settings.site_logo || '';
    siteSubtitle.value = settings.site_subtitle || 'Subscription to API Conversion Platform';
  REDACTED catch (error) {
    console.error('Failed to load public settings:', error);
  REDACTED
REDACTED);
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
REDACTED
</style>
