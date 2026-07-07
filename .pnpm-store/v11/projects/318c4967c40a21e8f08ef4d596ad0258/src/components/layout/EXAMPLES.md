# Layout Component Examples

## Example 1: Dashboard Page

```vue
<template>
  <AppLayout>
    <div class="space-y-6">
      <h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <!-- Stats Cards -->
        <div class="rounded-lg bg-white p-6 shadow">
          <div class="text-sm text-gray-600">API Keys</div>
          <div class="text-2xl font-bold text-gray-900">5</div>
        </div>

        <div class="rounded-lg bg-white p-6 shadow">
          <div class="text-sm text-gray-600">Total Usage</div>
          <div class="text-2xl font-bold text-gray-900">1,234</div>
        </div>

        <div class="rounded-lg bg-white p-6 shadow">
          <div class="text-sm text-gray-600">Balance</div>
          <div class="text-2xl font-bold text-indigo-600">${{ balance }}</div>
        </div>

        <div class="rounded-lg bg-white p-6 shadow">
          <div class="text-sm text-gray-600">Status</div>
          <div class="text-2xl font-bold text-green-600">Active</div>
        </div>
      </div>

      <div class="rounded-lg bg-white p-6 shadow">
        <h2 class="mb-4 text-xl font-semibold">Recent Activity</h2>
        <p class="text-gray-600">No recent activity</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { AppLayout } from '@/components/layout'
import { useAuthStore } from '@/stores'

const authStore = useAuthStore()
const balance = computed(() => authStore.user?.balance.toFixed(2) || '0.00')
</script>
```

---

## Example 2: Login Page

```vue
<template>
  <AuthLayout>
    <h2 class="mb-6 text-2xl font-bold text-gray-900">Welcome Back</h2>

    <form @submit.prevent="handleSubmit" class="space-y-4">
      <div>
        <label for="username" class="mb-1 block text-sm font-medium text-gray-700">
          Username
        </label>
        <input
          id="username"
          v-model="form.username"
          type="text"
          required
          class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-transparent focus:ring-2 focus:ring-indigo-500"
          placeholder="Enter your username"
        />
      </div>

      <div>
        <label for="password" class="mb-1 block text-sm font-medium text-gray-700">
          Password
        </label>
        <input
          id="password"
          v-model="form.password"
          type="password"
          required
          class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-transparent focus:ring-2 focus:ring-indigo-500"
          placeholder="Enter your password"
        />
      </div>

      <button
        type="submit"
        :disabled="loading"
        class="w-full rounded-lg bg-indigo-600 px-4 py-2 text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {{ loading ? 'Logging in...' : 'Login' }}
      </button>
    </form>

    <template #footer>
      <p class="text-gray-600">
        Don't have an account?
        <router-link to="/register" class="font-medium text-indigo-600 hover:underline">
          Sign up
        </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { AuthLayout } from '@/components/layout'
import { useAuthStore, useAppStore } from '@/stores'

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

const form = ref({
  username: '',
  password: ''
})

const loading = ref(false)

async function handleSubmit() {
  loading.value = true
  try {
    await authStore.login(form.value)
    appStore.showSuccess('Login successful!')
    await router.push('/dashboard')
  } catch (error) {
    appStore.showError('Invalid username or password')
  } finally {
    loading.value = false
  }
}
</script>
```

---

## Example 3: API Keys Page with Custom Header Title

```vue
<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Custom page header -->
      <div class="flex items-center justify-between">
        <h1 class="text-3xl font-bold text-gray-900">API Keys</h1>
        <button
          @click="showCreateModal = true"
          class="rounded-lg bg-indigo-600 px-4 py-2 text-white transition-colors hover:bg-indigo-700"
        >
          Create New Key
        </button>
      </div>

      <!-- API Keys List -->
      <div class="overflow-hidden rounded-lg bg-white shadow">
        <table class="min-w-full divide-y divide-gray-200">
          <thead class="bg-gray-50">
            <tr>
              <th class="px-6 py-3 text-left text-xs font-medium uppercase text-gray-500">Name</th>
              <th class="px-6 py-3 text-left text-xs font-medium uppercase text-gray-500">Key</th>
              <th class="px-6 py-3 text-left text-xs font-medium uppercase text-gray-500">
                Status
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium uppercase text-gray-500">
                Created
              </th>
              <th class="px-6 py-3 text-right text-xs font-medium uppercase text-gray-500">
                Actions
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white">
            <tr v-for="key in apiKeys" :key="key.id">
              <td class="whitespace-nowrap px-6 py-4">{{ key.name }}</td>
              <td class="px-6 py-4 font-mono text-sm">{{ key.key }}</td>
              <td class="px-6 py-4">
                <span
                  class="rounded-full px-2 py-1 text-xs"
                  :class="
                    key.status === 'active'
                      ? 'bg-green-100 text-green-800'
                      : 'bg-red-100 text-red-800'
                  "
                >
                  {{ key.status }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-gray-500">
                {{ new Date(key.created_at).toLocaleDateString() }}
              </td>
              <td class="px-6 py-4 text-right">
                <button class="text-sm text-red-600 hover:text-red-800">Delete</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { AppLayout } from '@/components/layout'
import type { ApiKey } from '@/types'

const showCreateModal = ref(false)
const apiKeys = ref<ApiKey[]>([])

// Fetch API keys on mount
// fetchApiKeys();
</script>
```

---

## Example 4: Admin Users Page

```vue
<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex items-center justify-between">
        <h1 class="text-3xl font-bold text-gray-900">User Management</h1>
        <button
          @click="showCreateUser = true"
          class="rounded-lg bg-indigo-600 px-4 py-2 text-white transition-colors hover:bg-indigo-700"
        >
          Create User
        </button>
      </div>

      <!-- Users Table -->
      <div class="rounded-lg bg-white shadow">
        <div class="p-6">
          <div class="space-y-4">
            <div
              v-for="user in users"
              :key="user.id"
              class="flex items-center justify-between border-b pb-4"
            >
              <div>
                <div class="font-medium text-gray-900">{{ user.username }}</div>
                <div class="text-sm text-gray-500">{{ user.email }}</div>
              </div>
              <div class="flex items-center space-x-4">
                <span
                  class="rounded-full px-2 py-1 text-xs"
                  :class="
                    user.role === 'admin'
                      ? 'bg-purple-100 text-purple-800'
                      : 'bg-blue-100 text-blue-800'
                  "
                >
                  {{ user.role }}
                </span>
                <span class="text-sm font-medium text-gray-700">
                  ${{ user.balance.toFixed(2) }}
                </span>
                <button class="text-sm text-indigo-600 hover:text-indigo-800">Edit</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { AppLayout } from '@/components/layout'
import type { User } from '@/types'

const showCreateUser = ref(false)
const users = ref<User[]>([])

// Fetch users on mount
// fetchUsers();
</script>
```

---

## Example 5: Profile Page

```vue
<template>
  <AppLayout>
    <div class="max-w-2xl space-y-6">
      <h1 class="text-3xl font-bold text-gray-900">Profile Settings</h1>

      <!-- User Info Card -->
      <div class="space-y-4 rounded-lg bg-white p-6 shadow">
        <h2 class="text-xl font-semibold text-gray-900">Account Information</h2>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700"> Username </label>
            <div class="rounded-lg bg-gray-50 px-3 py-2 text-gray-900">
              {{ user?.username }}
            </div>
          </div>

          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700"> Email </label>
            <div class="rounded-lg bg-gray-50 px-3 py-2 text-gray-900">
              {{ user?.email }}
            </div>
          </div>

          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700"> Role </label>
            <div class="rounded-lg bg-gray-50 px-3 py-2">
              <span
                class="rounded-full px-2 py-1 text-xs"
                :class="
                  user?.role === 'admin'
                    ? 'bg-purple-100 text-purple-800'
                    : 'bg-blue-100 text-blue-800'
                "
              >
                {{ user?.role }}
              </span>
            </div>
          </div>

          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700"> Balance </label>
            <div class="rounded-lg bg-gray-50 px-3 py-2 font-semibold text-indigo-600">
              ${{ user?.balance.toFixed(2) }}
            </div>
          </div>
        </div>
      </div>

      <!-- Change Password Card -->
      <div class="space-y-4 rounded-lg bg-white p-6 shadow">
        <h2 class="text-xl font-semibold text-gray-900">Change Password</h2>

        <form @submit.prevent="handleChangePassword" class="space-y-4">
          <div>
            <label for="old-password" class="mb-1 block text-sm font-medium text-gray-700">
              Current Password
            </label>
            <input
              id="old-password"
              v-model="passwordForm.old_password"
              type="password"
              required
              class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <div>
            <label for="new-password" class="mb-1 block text-sm font-medium text-gray-700">
              New Password
            </label>
            <input
              id="new-password"
              v-model="passwordForm.new_password"
              type="password"
              required
              class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <button
            type="submit"
            class="rounded-lg bg-indigo-600 px-4 py-2 text-white transition-colors hover:bg-indigo-700"
          >
            Update Password
          </button>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { AppLayout } from '@/components/layout'
import { useAuthStore, useAppStore } from '@/stores'

const authStore = useAuthStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)

const passwordForm = ref({
  old_password: '',
  new_password: ''
})

async function handleChangePassword() {
  try {
    // await changePasswordAPI(passwordForm.value);
    appStore.showSuccess('Password updated successfully!')
    passwordForm.value = { old_password: '', new_password: '' }
  } catch (error) {
    appStore.showError('Failed to update password')
  }
}
</script>
```

---

## Tips for Using Layouts

1. **Page Titles**: Set route meta to automatically display page titles in the header
2. **Loading States**: Use `appStore.setLoading(true/false)` for global loading indicators
3. **Toast Notifications**: Use `appStore.showSuccess()`, `appStore.showError()`, etc.
4. **Authentication**: All authenticated pages should use `AppLayout`
5. **Auth Pages**: Login and Register pages should use `AuthLayout`
6. **Sidebar State**: The sidebar state persists across navigation
7. **Mobile First**: All examples are responsive by default using Tailwind's mobile-first approach
