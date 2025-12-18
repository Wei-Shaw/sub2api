# Layout Component Examples

## Example 1: Dashboard Page

```vue
<template>
  <AppLayout>
    <div class="space-y-6">
      <h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>

      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Stats Cards -->
        <div class="bg-white p-6 rounded-lg shadow">
          <div class="text-sm text-gray-600">API Keys</div>
          <div class="text-2xl font-bold text-gray-900">5</div>
        </div>

        <div class="bg-white p-6 rounded-lg shadow">
          <div class="text-sm text-gray-600">Total Usage</div>
          <div class="text-2xl font-bold text-gray-900">1,234</div>
        </div>

        <div class="bg-white p-6 rounded-lg shadow">
          <div class="text-sm text-gray-600">Balance</div>
          <div class="text-2xl font-bold text-indigo-600">${{ balance REDACTEDREDACTED</div>
        </div>

        <div class="bg-white p-6 rounded-lg shadow">
          <div class="text-sm text-gray-600">Status</div>
          <div class="text-2xl font-bold text-green-600">Active</div>
        </div>
      </div>

      <div class="bg-white p-6 rounded-lg shadow">
        <h2 class="text-xl font-semibold mb-4">Recent Activity</h2>
        <p class="text-gray-600">No recent activity</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue';
import { AppLayout REDACTED from '@/components/layout';
import { useAuthStore REDACTED from '@/stores';

const authStore = useAuthStore();
const balance = computed(() => authStore.user?.balance.toFixed(2) || '0.00');
</script>
```

---

## Example 2: Login Page

```vue
<template>
  <AuthLayout>
    <h2 class="text-2xl font-bold text-gray-900 mb-6">Welcome Back</h2>

    <form @submit.prevent="handleSubmit" class="space-y-4">
      <div>
        <label for="username" class="block text-sm font-medium text-gray-700 mb-1">
          Username
        </label>
        <input
          id="username"
          v-model="form.username"
          type="text"
          required
          class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          placeholder="Enter your username"
        />
      </div>

      <div>
        <label for="password" class="block text-sm font-medium text-gray-700 mb-1">
          Password
        </label>
        <input
          id="password"
          v-model="form.password"
          type="password"
          required
          class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          placeholder="Enter your password"
        />
      </div>

      <button
        type="submit"
        :disabled="loading"
        class="w-full bg-indigo-600 text-white py-2 px-4 rounded-lg hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {{ loading ? 'Logging in...' : 'Login' REDACTEDREDACTED
      </button>
    </form>

    <template #footer>
      <p class="text-gray-600">
        Don't have an account?
        <router-link to="/register" class="text-indigo-600 hover:underline font-medium">
          Sign up
        </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue';
import { useRouter REDACTED from 'vue-router';
import { AuthLayout REDACTED from '@/components/layout';
import { useAuthStore, useAppStore REDACTED from '@/stores';

const router = useRouter();
const authStore = useAuthStore();
const appStore = useAppStore();

const form = ref({
  username: '',
  password: '',
REDACTED);

const loading = ref(false);

async function handleSubmit() {
  loading.value = true;
  try {
    await authStore.login(form.value);
    appStore.showSuccess('Login successful!');
    await router.push('/dashboard');
  REDACTED catch (error) {
    appStore.showError('Invalid username or password');
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED
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
          class="bg-indigo-600 text-white px-4 py-2 rounded-lg hover:bg-indigo-700 transition-colors"
        >
          Create New Key
        </button>
      </div>

      <!-- API Keys List -->
      <div class="bg-white rounded-lg shadow overflow-hidden">
        <table class="min-w-full divide-y divide-gray-200">
          <thead class="bg-gray-50">
            <tr>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Name
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Key
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Status
              </th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Created
              </th>
              <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">
                Actions
              </th>
            </tr>
          </thead>
          <tbody class="bg-white divide-y divide-gray-200">
            <tr v-for="key in apiKeys" :key="key.id">
              <td class="px-6 py-4 whitespace-nowrap">{{ key.name REDACTEDREDACTED</td>
              <td class="px-6 py-4 font-mono text-sm">{{ key.key REDACTEDREDACTED</td>
              <td class="px-6 py-4">
                <span
                  class="px-2 py-1 text-xs rounded-full"
                  :class="key.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'"
                >
                  {{ key.status REDACTEDREDACTED
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-gray-500">
                {{ new Date(key.created_at).toLocaleDateString() REDACTEDREDACTED
              </td>
              <td class="px-6 py-4 text-right">
                <button class="text-red-600 hover:text-red-800 text-sm">
                  Delete
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue';
import { AppLayout REDACTED from '@/components/layout';
import type { ApiKey REDACTED from '@/types';

const showCreateModal = ref(false);
const apiKeys = ref<ApiKey[]>([]);

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
          class="bg-indigo-600 text-white px-4 py-2 rounded-lg hover:bg-indigo-700 transition-colors"
        >
          Create User
        </button>
      </div>

      <!-- Users Table -->
      <div class="bg-white rounded-lg shadow">
        <div class="p-6">
          <div class="space-y-4">
            <div v-for="user in users" :key="user.id" class="flex items-center justify-between border-b pb-4">
              <div>
                <div class="font-medium text-gray-900">{{ user.username REDACTEDREDACTED</div>
                <div class="text-sm text-gray-500">{{ user.email REDACTEDREDACTED</div>
              </div>
              <div class="flex items-center space-x-4">
                <span
                  class="px-2 py-1 text-xs rounded-full"
                  :class="user.role === 'admin' ? 'bg-purple-100 text-purple-800' : 'bg-blue-100 text-blue-800'"
                >
                  {{ user.role REDACTEDREDACTED
                </span>
                <span class="text-sm font-medium text-gray-700">
                  ${{ user.balance.toFixed(2) REDACTEDREDACTED
                </span>
                <button class="text-indigo-600 hover:text-indigo-800 text-sm">
                  Edit
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue';
import { AppLayout REDACTED from '@/components/layout';
import type { User REDACTED from '@/types';

const showCreateUser = ref(false);
const users = ref<User[]>([]);

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
      <div class="bg-white rounded-lg shadow p-6 space-y-4">
        <h2 class="text-xl font-semibold text-gray-900">Account Information</h2>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Username
            </label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg text-gray-900">
              {{ user?.username REDACTEDREDACTED
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Email
            </label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg text-gray-900">
              {{ user?.email REDACTEDREDACTED
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Role
            </label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg">
              <span
                class="px-2 py-1 text-xs rounded-full"
                :class="user?.role === 'admin' ? 'bg-purple-100 text-purple-800' : 'bg-blue-100 text-blue-800'"
              >
                {{ user?.role REDACTEDREDACTED
              </span>
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Balance
            </label>
            <div class="px-3 py-2 bg-gray-50 rounded-lg text-indigo-600 font-semibold">
              ${{ user?.balance.toFixed(2) REDACTEDREDACTED
            </div>
          </div>
        </div>
      </div>

      <!-- Change Password Card -->
      <div class="bg-white rounded-lg shadow p-6 space-y-4">
        <h2 class="text-xl font-semibold text-gray-900">Change Password</h2>

        <form @submit.prevent="handleChangePassword" class="space-y-4">
          <div>
            <label for="old-password" class="block text-sm font-medium text-gray-700 mb-1">
              Current Password
            </label>
            <input
              id="old-password"
              v-model="passwordForm.old_password"
              type="password"
              required
              class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <div>
            <label for="new-password" class="block text-sm font-medium text-gray-700 mb-1">
              New Password
            </label>
            <input
              id="new-password"
              v-model="passwordForm.new_password"
              type="password"
              required
              class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <button
            type="submit"
            class="bg-indigo-600 text-white px-4 py-2 rounded-lg hover:bg-indigo-700 transition-colors"
          >
            Update Password
          </button>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed REDACTED from 'vue';
import { AppLayout REDACTED from '@/components/layout';
import { useAuthStore, useAppStore REDACTED from '@/stores';

const authStore = useAuthStore();
const appStore = useAppStore();

const user = computed(() => authStore.user);

const passwordForm = ref({
  old_password: '',
  new_password: '',
REDACTED);

async function handleChangePassword() {
  try {
    // await changePasswordAPI(passwordForm.value);
    appStore.showSuccess('Password updated successfully!');
    passwordForm.value = { old_password: '', new_password: '' REDACTED;
  REDACTED catch (error) {
    appStore.showError('Failed to update password');
  REDACTED
REDACTED
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
