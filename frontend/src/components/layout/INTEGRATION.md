# Layout Components Integration Guide

## Quick Start

### 1. Import Layout Components

```typescript
// In your view files
import { AppLayout, AuthLayout } from '@/components/layout'
```

### 2. Use in Routes

```typescript
// src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

// Views
import DashboardView from '@/views/DashboardView.vue'
import LoginView from '@/views/auth/LoginView.vue'
import RegisterView from '@/views/auth/RegisterView.vue'

const routes: RouteRecordRaw[] = [
  // Auth routes (no layout needed - views use AuthLayout internally)
  {
    path: '/login',
    name: 'Login',
    component: LoginView,
    meta: { requiresAuth: false }
  },
  {
    path: '/register',
    name: 'Register',
    component: RegisterView,
    meta: { requiresAuth: false }
  },

  // User routes (use AppLayout)
  {
    path: '/dashboard',
    name: 'Dashboard',
    component: DashboardView,
    meta: { requiresAuth: true, title: 'Dashboard' }
  },
  {
    path: '/api-keys',
    name: 'ApiKeys',
    component: () => import('@/views/ApiKeysView.vue'),
    meta: { requiresAuth: true, title: 'API Keys' }
  },
  {
    path: '/usage',
    name: 'Usage',
    component: () => import('@/views/UsageView.vue'),
    meta: { requiresAuth: true, title: 'Usage Statistics' }
  },
  {
    path: '/redeem',
    name: 'Redeem',
    component: () => import('@/views/RedeemView.vue'),
    meta: { requiresAuth: true, title: 'Redeem Code' }
  },
  {
    path: '/profile',
    name: 'Profile',
    component: () => import('@/views/ProfileView.vue'),
    meta: { requiresAuth: true, title: 'Profile Settings' }
  },

  // Admin routes (use AppLayout, admin only)
  {
    path: '/admin/dashboard',
    name: 'AdminDashboard',
    component: () => import('@/views/admin/DashboardView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Admin Dashboard' }
  },
  {
    path: '/admin/users',
    name: 'AdminUsers',
    component: () => import('@/views/admin/UsersView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'User Management' }
  },
  {
    path: '/admin/groups',
    name: 'AdminGroups',
    component: () => import('@/views/admin/GroupsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Groups' }
  },
  {
    path: '/admin/accounts',
    name: 'AdminAccounts',
    component: () => import('@/views/admin/AccountsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Accounts' }
  },
  {
    path: '/admin/proxies',
    name: 'AdminProxies',
    component: () => import('@/views/admin/ProxiesView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Proxies' }
  },
  {
    path: '/admin/redeem-codes',
    name: 'AdminRedeemCodes',
    component: () => import('@/views/admin/RedeemCodesView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Redeem Codes' }
  },

  // Default redirect
  {
    path: '/',
    redirect: '/dashboard'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Navigation guards
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()

  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    // Redirect to login if not authenticated
    next('/login')
  } else if (to.meta.requiresAdmin && !authStore.isAdmin) {
    // Redirect to dashboard if not admin
    next('/dashboard')
  } else {
    next()
  }
})

export default router
```

### 3. Initialize Stores in main.ts

```typescript
// src/main.ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

// Initialize auth state on app startup
import { useAuthStore } from '@/stores'
const authStore = useAuthStore()
authStore.checkAuth()

app.mount('#app')
```

### 4. Update App.vue

```vue
<!-- src/App.vue -->
<template>
  <router-view />
</template>

<script setup lang="ts">
// App.vue just renders the router view
// Layouts are handled by individual views
</script>
```

---

## View Component Templates

### Authenticated Page Template

```vue
<!-- src/views/DashboardView.vue -->
<template>
  <AppLayout>
    <div class="space-y-6">
      <h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>

      <!-- Your content here -->
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { AppLayout } from '@/components/layout'

// Your component logic here
</script>
```

### Auth Page Template

```vue
<!-- src/views/auth/LoginView.vue -->
<template>
  <AuthLayout>
    <h2 class="mb-6 text-2xl font-bold text-gray-900">Login</h2>

    <!-- Your login form here -->

    <template #footer>
      <p class="text-gray-600">
        Don't have an account?
        <router-link to="/register" class="text-indigo-600 hover:underline"> Sign up </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { AuthLayout } from '@/components/layout'

// Your login logic here
</script>
```

---

## Customization

### Changing Colors

The components use Tailwind's indigo color scheme by default. To change:

```vue
<!-- Change all instances of indigo-* to your preferred color -->
<div class="bg-blue-600">   <!-- Instead of bg-indigo-600 -->
<div class="text-blue-600">  <!-- Instead of text-indigo-600 -->
```

### Adding Custom Icons

Replace HTML entity icons with your preferred icon library:

```vue
<!-- Before (HTML entities) -->
<span class="text-lg">&#128200;</span>

<!-- After (Heroicons example) -->
<ChartBarIcon class="h-5 w-5" />
```

### Sidebar Customization

Modify navigation items in `AppSidebar.vue`:

```typescript
// Add/remove/modify navigation items
const userNavItems = [
  { path: '/dashboard', label: 'Dashboard', icon: '&#128200;' },
  { path: '/new-page', label: 'New Page', icon: '&#128196;' } // Add new item
  // ...
]
```

### Header Customization

Modify user dropdown in `AppHeader.vue`:

```vue
<!-- Add new dropdown items -->
<router-link
  to="/settings"
  @click="closeDropdown"
  class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
>
  <span class="mr-2">&#9881;</span>
  Settings
</router-link>
```

---

## Mobile Responsive Behavior

### Sidebar

- **Desktop (md+)**: Always visible, can be collapsed to icon-only view
- **Mobile**: Hidden by default, shown via menu toggle in header

### Header

- **Desktop**: Shows full user info and balance
- **Mobile**: Shows compact view with hamburger menu

To improve mobile experience, you can add overlay and transitions:

```vue
<!-- AppSidebar.vue enhancement for mobile -->
<aside
  class="fixed left-0 top-0 z-40 h-screen transition-transform duration-300"
  :class="[
    sidebarCollapsed ? 'w-16' : 'w-64',
    // Hide on mobile when collapsed
    'md:translate-x-0',
    sidebarCollapsed ? '-translate-x-full md:translate-x-0' : 'translate-x-0'
  ]"
>
  <!-- ... -->
</aside>

<!-- Add overlay for mobile -->
<div
  v-if="!sidebarCollapsed"
  @click="toggleSidebar"
  class="fixed inset-0 z-30 bg-black bg-opacity-50 md:hidden"
></div>
```

---

## State Management Integration

### Auth Store Usage

```typescript
import { useAuthStore } from '@/stores'

const authStore = useAuthStore()

// Check if user is authenticated
if (authStore.isAuthenticated) {
  // User is logged in
}

// Check if user is admin
if (authStore.isAdmin) {
  // User has admin role
}

// Get current user
const user = authStore.user
```

### App Store Usage

```typescript
import { useAppStore } from '@/stores'

const appStore = useAppStore()

// Toggle sidebar
appStore.toggleSidebar()

// Show notifications
appStore.showSuccess('Operation completed!')
appStore.showError('Something went wrong')
appStore.showInfo('Did you know...')
appStore.showWarning('Be careful!')

// Loading state
appStore.setLoading(true)
// ... perform operation
appStore.setLoading(false)

// Or use helper
await appStore.withLoading(async () => {
  // Your async operation
})
```

---

## Accessibility Features

All layout components include:

- **Semantic HTML**: Proper use of `<nav>`, `<header>`, `<main>`, `<aside>`
- **ARIA labels**: Buttons have descriptive labels
- **Keyboard navigation**: All interactive elements are keyboard accessible
- **Focus management**: Proper focus states with Tailwind's `focus:` utilities
- **Color contrast**: WCAG AA compliant color combinations

To enhance further:

```vue
<!-- Add skip to main content link -->
<a
  href="#main-content"
  class="sr-only rounded bg-white px-4 py-2 focus:not-sr-only focus:absolute focus:left-4 focus:top-4"
>
  Skip to main content
</a>

<main id="main-content">
  <!-- Content -->
</main>
```

---

## Testing

### Unit Testing Layout Components

```typescript
// AppHeader.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AppHeader from '@/components/layout/AppHeader.vue'

describe('AppHeader', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders user info when authenticated', () => {
    const wrapper = mount(AppHeader)
    // Add assertions
  })

  it('shows dropdown when clicked', async () => {
    const wrapper = mount(AppHeader)
    await wrapper.find('button').trigger('click')
    expect(wrapper.find('.dropdown').exists()).toBe(true)
  })
})
```

---

## Performance Optimization

### Lazy Loading

Views using layouts are already lazy loaded in the router example above.

### Code Splitting

Layout components are automatically code-split when imported:

```typescript
// This creates a separate chunk for layout components
import { AppLayout } from '@/components/layout'
```

### Reducing Re-renders

Layout components use `computed` refs to prevent unnecessary re-renders:

```typescript
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
// This only re-renders when sidebarCollapsed changes
```

---

## Troubleshooting

### Sidebar not showing

- Check if `useAppStore` is properly initialized
- Verify Tailwind classes are being processed
- Check z-index conflicts with other components

### Routes not highlighting in sidebar

- Ensure route paths match exactly
- Check `isActive()` function logic
- Verify `useRoute()` is working correctly

### User info not displaying

- Ensure auth store is initialized with `checkAuth()`
- Verify user is logged in
- Check localStorage for auth data

### Mobile menu not working

- Verify `toggleSidebar()` is called correctly
- Check responsive breakpoints (md:)
- Test on actual mobile device or browser dev tools
