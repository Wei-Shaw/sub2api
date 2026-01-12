<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="space-y-2">
          <div class="h-5 w-48 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="h-4 w-72 animate-pulse rounded bg-gray-100 dark:bg-dark-700/70"></div>
        </div>
        <div class="flex items-center gap-3">
          <div class="h-9 w-28 animate-pulse rounded-xl bg-gray-200 dark:bg-dark-700"></div>
          <div class="h-9 w-28 animate-pulse rounded-xl bg-gray-200 dark:bg-dark-700"></div>
        </div>
      </div>

      <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div v-for="i in 4" :key="i" class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/30">
          <div class="h-3 w-16 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          <div class="mt-3 h-6 w-24 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
        </div>
      </div>
    </div>

    <!-- Charts -->
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
        <div class="h-4 w-40 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
        <div class="mt-6 h-64 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/70"></div>
      </div>
      <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
        <div class="h-4 w-40 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
        <div class="mt-6 h-64 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700/70"></div>
      </div>
    </div>

    <!-- Cards -->
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div
        v-for="i in 3"
        :key="i"
        class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
      >
        <div class="h-4 w-36 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
        <div class="mt-4 space-y-3">
          <div class="h-3 w-2/3 animate-pulse rounded bg-gray-100 dark:bg-dark-700/70"></div>
          <div class="h-3 w-1/2 animate-pulse rounded bg-gray-100 dark:bg-dark-700/70"></div>
          <div class="h-3 w-3/5 animate-pulse rounded bg-gray-100 dark:bg-dark-700/70"></div>
        </div>
      </div>
    </div>
  </div>
</template>

