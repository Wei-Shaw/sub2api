<template>
  <header class="sticky top-0 z-30 glass border-b border-gray-200/50 dark:border-dark-700/50">
    <div class="flex items-center justify-between h-16 px-4 md:px-6">
      <!-- Left: Mobile Menu Toggle + Page Title -->
      <div class="flex items-center gap-4">
        <button
          @click="toggleMobileSidebar"
          class="lg:hidden btn-ghost btn-icon"
          aria-label="Toggle Menu"
        >
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
          </svg>
        </button>

        <div class="hidden lg:block">
          <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ pageTitle }}
          </h1>
          <p v-if="pageDescription" class="text-xs text-gray-500 dark:text-dark-400">
            {{ pageDescription }}
          </p>
        </div>
      </div>

      <!-- Right: Language + Subscriptions + Balance + User Dropdown -->
      <div class="flex items-center gap-3">
        <!-- Language Switcher -->
        <LocaleSwitcher />

        <!-- Subscription Progress (for users with active subscriptions) -->
        <SubscriptionProgressMini v-if="user" />

        <!-- Balance Display -->
        <div v-if="user" class="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-xl bg-primary-50 dark:bg-primary-900/20">
          <svg class="w-4 h-4 text-primary-600 dark:text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" />
          </svg>
          <span class="text-sm font-semibold text-primary-700 dark:text-primary-300">
            ${{ user.balance?.toFixed(2) || '0.00' }}
          </span>
        </div>

        <!-- User Dropdown -->
        <div v-if="user" class="relative" ref="dropdownRef">
          <button
            @click="toggleDropdown"
            class="flex items-center gap-2 p-1.5 rounded-xl hover:bg-gray-100 dark:hover:bg-dark-800 transition-colors"
            aria-label="User Menu"
          >
            <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 text-white flex items-center justify-center text-sm font-medium shadow-sm">
              {{ userInitials }}
            </div>
            <div class="hidden md:block text-left">
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ displayName }}
              </div>
              <div class="text-xs text-gray-500 dark:text-dark-400 capitalize">
                {{ user.role }}
              </div>
            </div>
            <svg class="w-4 h-4 text-gray-400 hidden md:block" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
            </svg>
          </button>

          <!-- Dropdown Menu -->
          <transition name="dropdown">
            <div
              v-if="dropdownOpen"
              class="dropdown right-0 mt-2 w-56"
            >
              <!-- User Info -->
              <div class="px-4 py-3 border-b border-gray-100 dark:border-dark-700">
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ displayName }}</div>
                <div class="text-xs text-gray-500 dark:text-dark-400">{{ user.email }}</div>
              </div>

              <!-- Balance (mobile only) -->
              <div class="sm:hidden px-4 py-2 border-b border-gray-100 dark:border-dark-700">
                <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('common.balance') }}</div>
                <div class="text-sm font-semibold text-primary-600 dark:text-primary-400">
                  ${{ user.balance?.toFixed(2) || '0.00' }}
                </div>
              </div>

              <div class="py-1">
                <router-link
                  to="/profile"
                  @click="closeDropdown"
                  class="dropdown-item"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z" />
                  </svg>
                  {{ t('nav.profile') }}
                </router-link>

                <router-link
                  to="/keys"
                  @click="closeDropdown"
                  class="dropdown-item"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
                  </svg>
                  {{ t('nav.apiKeys') }}
                </router-link>

                <a
                  href="https://github.com/Wei-Shaw/sub2api"
                  target="_blank"
                  rel="noopener noreferrer"
                  @click="closeDropdown"
                  class="dropdown-item"
                >
                  <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                    <path fill-rule="evenodd" clip-rule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z" />
                  </svg>
                  {{ t('nav.github') }}
                </a>
              </div>

              <!-- Contact Support (only show if configured) -->
              <div v-if="contactInfo" class="border-t border-gray-100 dark:border-dark-700 px-4 py-2.5">
                <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M20.25 8.511c.884.284 1.5 1.128 1.5 2.097v4.286c0 1.136-.847 2.1-1.98 2.193-.34.027-.68.052-1.02.072v3.091l-3-3c-1.354 0-2.694-.055-4.02-.163a2.115 2.115 0 01-.825-.242m9.345-8.334a2.126 2.126 0 00-.476-.095 48.64 48.64 0 00-8.048 0c-1.131.094-1.976 1.057-1.976 2.192v4.286c0 .837.46 1.58 1.155 1.951m9.345-8.334V6.637c0-1.621-1.152-3.026-2.76-3.235A48.455 48.455 0 0011.25 3c-2.115 0-4.198.137-6.24.402-1.608.209-2.76 1.614-2.76 3.235v6.226c0 1.621 1.152 3.026 2.76 3.235.577.075 1.157.14 1.74.194V21l4.155-4.155" />
                  </svg>
                  <span>{{ t('common.contactSupport') }}:</span>
                  <span class="font-medium text-gray-700 dark:text-gray-300">{{ contactInfo }}</span>
                </div>
              </div>

              <div class="border-t border-gray-100 dark:border-dark-700 py-1">
                <button
                  @click="handleLogout"
                  class="dropdown-item w-full text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 9l-3 3m0 0l3 3m-3-3h12.75" />
                  </svg>
                  {{ t('nav.logout') }}
                </button>
              </div>
            </div>
          </transition>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAppStore, useAuthStore } from '@/stores';
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue';
import SubscriptionProgressMini from '@/components/common/SubscriptionProgressMini.vue';

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const appStore = useAppStore();
const authStore = useAuthStore();

const user = computed(() => authStore.user);
const dropdownOpen = ref(false);
const dropdownRef = ref<HTMLElement | null>(null);
const contactInfo = computed(() => appStore.contactInfo);

const userInitials = computed(() => {
  if (!user.value) return '';
  // Prefer username, fallback to email
  if (user.value.username) {
    return user.value.username.substring(0, 2).toUpperCase();
  }
  if (user.value.email) {
    // Get the part before @ and take first 2 chars
    const localPart = user.value.email.split('@')[0];
    return localPart.substring(0, 2).toUpperCase();
  }
  return '';
});

const displayName = computed(() => {
  if (!user.value) return '';
  return user.value.username || user.value.email?.split('@')[0] || '';
});

const pageTitle = computed(() => {
  const titleKey = route.meta.titleKey as string;
  if (titleKey) {
    return t(titleKey);
  }
  return (route.meta.title as string) || '';
});

const pageDescription = computed(() => {
  const descKey = route.meta.descriptionKey as string;
  if (descKey) {
    return t(descKey);
  }
  return (route.meta.description as string) || '';
});

function toggleMobileSidebar() {
  appStore.toggleMobileSidebar();
}

function toggleDropdown() {
  dropdownOpen.value = !dropdownOpen.value;
}

function closeDropdown() {
  dropdownOpen.value = false;
}

async function handleLogout() {
  closeDropdown();
  // 先通知后端清理 Cookie，再跳转登录页。
  await authStore.logout();
  await router.push('/login');
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    closeDropdown();
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside);
});
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
