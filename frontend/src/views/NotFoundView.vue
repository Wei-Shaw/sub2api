<template>
  <div
    class="flex min-h-screen items-center justify-center bg-white px-6 text-gray-900 dark:bg-dark-950 dark:text-gray-100"
  >
    <div class="w-full max-w-md text-center">
      <!-- 404 Display -->
      <div class="mb-8">
        <div class="relative inline-block">
          <span class="text-[12rem] font-bold leading-none text-gray-100 dark:text-dark-800"
            >404</span
          >
          <div class="absolute inset-0 flex items-center justify-center">
            <div
              class="flex h-24 w-24 items-center justify-center rounded-md border border-gray-200 bg-gray-100 text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200"
            >
              <svg
                class="h-12 w-12 text-current"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z"
                />
              </svg>
            </div>
          </div>
        </div>
      </div>

      <!-- Text Content -->
      <div class="mb-8">
        <h1 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('errors.pageNotFound') }}
        </h1>
        <p class="text-gray-500 dark:text-dark-400">
          The page you are looking for doesn't exist or has been moved.
        </p>
      </div>

      <!-- Action Buttons -->
      <div class="flex flex-col justify-center gap-3 sm:flex-row">
        <button @click="goBack" class="btn btn-secondary">
          <Icon name="arrowLeft" size="md" class="mr-2" />
          Go Back
        </button>
        <router-link to="/dashboard" class="btn btn-primary">
          <Icon name="home" size="md" class="mr-2" />
          Go to Dashboard
        </router-link>
      </div>

      <!-- Help Link -->
      <p class="mt-8 text-sm text-gray-400 dark:text-dark-500">
        Need help?
        <a
          href="#"
          class="text-gray-900 underline underline-offset-4 transition-colors hover:text-gray-600 dark:text-gray-100 dark:hover:text-gray-300"
        >
          Contact support
        </a>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()

function goBack(): void {
  router.back()
}
</script>
