<template>
  <div class="relative">
    <!-- Admin: Full version badge with dropdown -->
    <template v-if="isAdmin">
      <button
        @click="toggleDropdown"
        class="flex items-center gap-1.5 px-2 py-1 text-xs rounded-lg transition-colors"
        :class="[
          hasUpdate
            ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 hover:bg-amber-200 dark:hover:bg-amber-900/50'
            : 'bg-gray-100 dark:bg-dark-800 text-gray-600 dark:text-dark-400 hover:bg-gray-200 dark:hover:bg-dark-700'
        ]"
        :title="hasUpdate ? 'New version available' : 'Up to date'"
      >
        <span class="font-medium">v{{ currentVersion REDACTEDREDACTED</span>
        <!-- Update indicator -->
        <span v-if="hasUpdate" class="relative flex h-2 w-2">
          <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
          <span class="relative inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
        </span>
      </button>

      <!-- Dropdown -->
      <transition name="dropdown">
        <div
          v-if="dropdownOpen"
          ref="dropdownRef"
          class="absolute left-0 mt-2 w-64 bg-white dark:bg-dark-800 rounded-xl shadow-lg border border-gray-200 dark:border-dark-700 z-50 overflow-hidden"
        >
          <!-- Header with refresh button -->
          <div class="flex items-center justify-between px-4 py-3 border-b border-gray-100 dark:border-dark-700">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('version.currentVersion') REDACTEDREDACTED</span>
            <button
              @click="refreshVersion(true)"
              class="p-1.5 rounded-lg text-gray-400 hover:text-gray-600 dark:hover:text-dark-200 hover:bg-gray-100 dark:hover:bg-dark-700 transition-colors"
              :disabled="loading"
              :title="t('version.refresh')"
            >
              <svg class="w-4 h-4" :class="{ 'animate-spin': loading REDACTED" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
              </svg>
            </button>
          </div>

          <div class="p-4">
            <!-- Loading state -->
            <div v-if="loading" class="flex items-center justify-center py-6">
              <svg class="animate-spin h-6 w-6 text-primary-500" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>

            <!-- Content -->
            <template v-else>
              <!-- Version display - centered and prominent -->
              <div class="text-center mb-4">
                <div class="inline-flex items-center gap-2">
                  <span class="text-2xl font-bold text-gray-900 dark:text-white">v{{ currentVersion REDACTEDREDACTED</span>
                  <!-- Show check mark when up to date -->
                  <span v-if="!hasUpdate" class="flex items-center justify-center w-5 h-5 rounded-full bg-green-100 dark:bg-green-900/30">
                    <svg class="w-3 h-3 text-green-600 dark:text-green-400" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </span>
                </div>
                <p class="text-xs text-gray-500 dark:text-dark-400 mt-1">
                  {{ hasUpdate ? t('version.latestVersion') + ': v' + latestVersion : t('version.upToDate') REDACTEDREDACTED
                </p>
              </div>

              <!-- Update available for source build - show git pull hint -->
              <div v-if="hasUpdate && !isReleaseBuild" class="space-y-2">
                <a
                  v-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                  :href="releaseInfo.html_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="flex items-center gap-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/50 hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors group"
                >
                  <div class="flex-shrink-0 w-8 h-8 rounded-full bg-amber-100 dark:bg-amber-900/50 flex items-center justify-center">
                    <svg class="w-4 h-4 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                    </svg>
                  </div>
                  <div class="flex-1 min-w-0">
                    <p class="text-sm font-medium text-amber-700 dark:text-amber-300">{{ t('version.updateAvailable') REDACTEDREDACTED</p>
                    <p class="text-xs text-amber-600/70 dark:text-amber-400/70">v{{ latestVersion REDACTEDREDACTED</p>
                  </div>
                  <svg class="w-4 h-4 text-amber-500 dark:text-amber-400 group-hover:translate-x-0.5 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                </a>
                <!-- Source build hint -->
                <div class="flex items-center gap-2 p-2 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800/50">
                  <svg class="w-3.5 h-3.5 text-blue-500 dark:text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <p class="text-xs text-blue-600 dark:text-blue-400">{{ t('version.sourceModeHint') REDACTEDREDACTED</p>
                </div>
              </div>

              <!-- Update available for release build - show download link -->
              <a
                v-else-if="hasUpdate && isReleaseBuild && releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                :href="releaseInfo.html_url"
                target="_blank"
                rel="noopener noreferrer"
                class="flex items-center gap-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/50 hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors group"
              >
                <div class="flex-shrink-0 w-8 h-8 rounded-full bg-amber-100 dark:bg-amber-900/50 flex items-center justify-center">
                  <svg class="w-4 h-4 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                  </svg>
                </div>
                <div class="flex-1 min-w-0">
                  <p class="text-sm font-medium text-amber-700 dark:text-amber-300">{{ t('version.updateAvailable') REDACTEDREDACTED</p>
                  <p class="text-xs text-amber-600/70 dark:text-amber-400/70">v{{ latestVersion REDACTEDREDACTED</p>
                </div>
                <svg class="w-4 h-4 text-amber-500 dark:text-amber-400 group-hover:translate-x-0.5 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </a>

              <!-- GitHub link when up to date -->
              <a
                v-else-if="releaseInfo?.html_url && releaseInfo.html_url !== '#'"
                :href="releaseInfo.html_url"
                target="_blank"
                rel="noopener noreferrer"
                class="flex items-center justify-center gap-2 py-2 text-sm text-gray-500 dark:text-dark-400 hover:text-gray-700 dark:hover:text-dark-200 transition-colors"
              >
                <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                  <path fill-rule="evenodd" clip-rule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z" />
                </svg>
                {{ t('version.viewRelease') REDACTEDREDACTED
              </a>
            </template>
          </div>
        </div>
      </transition>
    </template>

    <!-- Non-admin: Simple static version text -->
    <span
      v-else-if="version"
      class="text-xs text-gray-500 dark:text-dark-400"
    >
      v{{ version REDACTEDREDACTED
    </span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount REDACTED from 'vue';
import { useI18n REDACTED from 'vue-i18n';
import { useAuthStore REDACTED from '@/stores';
import { checkUpdates, type VersionInfo, type ReleaseInfo REDACTED from '@/api/admin/system';

const { t REDACTED = useI18n();

const props = defineProps<{
  version?: string;
REDACTED>();

const authStore = useAuthStore();

const isAdmin = computed(() => authStore.isAdmin);

const loading = ref(false);
const dropdownOpen = ref(false);
const dropdownRef = ref<HTMLElement | null>(null);

const currentVersion = ref('0.1.0');
const latestVersion = ref('0.1.0');
const hasUpdate = ref(false);
const releaseInfo = ref<ReleaseInfo | null>(null);
const buildType = ref('source'); // "source" or "release"

// Only show update check for release builds (binary/docker deployment)
const isReleaseBuild = computed(() => buildType.value === 'release');

function toggleDropdown() {
  dropdownOpen.value = !dropdownOpen.value;
REDACTED

function closeDropdown() {
  dropdownOpen.value = false;
REDACTED

async function refreshVersion(force = true) {
  if (!isAdmin.value) return;

  loading.value = true;
  try {
    const data: VersionInfo = await checkUpdates(force);
    currentVersion.value = data.current_version;
    latestVersion.value = data.latest_version;
    buildType.value = data.build_type || 'source';
    // Show update indicator for all build types
    hasUpdate.value = data.has_update;
    releaseInfo.value = data.release_info || null;
  REDACTED catch (error) {
    console.error('Failed to check updates:', error);
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node;
  const button = (event.target as Element).closest('button');
  if (dropdownRef.value && !dropdownRef.value.contains(target) && !button?.contains(target)) {
    closeDropdown();
  REDACTED
REDACTED

onMounted(() => {
  if (isAdmin.value) {
    refreshVersion(false);
  REDACTED else if (props.version) {
    currentVersion.value = props.version;
  REDACTED
  document.addEventListener('click', handleClickOutside);
REDACTED);

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside);
REDACTED);
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
REDACTED

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
REDACTED

.line-clamp-3 {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
REDACTED
</style>
