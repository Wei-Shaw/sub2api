# Pinia Stores Documentation

This directory contains all Pinia stores for the Sub2API frontend application.

## Stores Overview

### 1. Auth Store (`auth.ts`)

Manages user authentication state, login/logout, and token persistence.

**State:**

- `user: User | null` - Current authenticated user
- `token: string | null` - JWT authentication token

**Computed:**

- `isAuthenticated: boolean` - Whether user is currently authenticated

**Actions:**

- `login(credentials)` - Authenticate user with username/password
- `register(userData)` - Register new user account
- `logout()` - Clear authentication and logout
- `checkAuth()` - Restore session from localStorage
- `refreshUser()` - Fetch latest user data from server

### 2. App Store (`app.ts`)

Manages global UI state including sidebar, loading indicators, and toast notifications.

**State:**

- `sidebarCollapsed: boolean` - Sidebar collapsed state
- `loading: boolean` - Global loading state
- `toasts: Toast[]` - Active toast notifications

**Computed:**

- `hasActiveToasts: boolean` - Whether any toasts are active

**Actions:**

- `toggleSidebar()` - Toggle sidebar state
- `setSidebarCollapsed(collapsed)` - Set sidebar state explicitly
- `setLoading(isLoading)` - Set loading state
- `showToast(type, message, duration?)` - Show toast notification
- `showSuccess(message, duration?)` - Show success toast
- `showError(message, duration?)` - Show error toast
- `showInfo(message, duration?)` - Show info toast
- `showWarning(message, duration?)` - Show warning toast
- `hideToast(id)` - Hide specific toast
- `clearAllToasts()` - Clear all toasts
- `withLoading(operation)` - Execute async operation with loading state
- `withLoadingAndError(operation, errorMessage?)` - Execute with loading and error handling
- `reset()` - Reset store to defaults

## Usage Examples

### Auth Store

```typescript
import { useAuthStore REDACTED from '@/stores'

// In component setup
const authStore = useAuthStore()

// Initialize on app startup
authStore.checkAuth()

// Login
try {
  await authStore.login({ username: 'user', password: 'pass' REDACTED)
  console.log('Logged in:', authStore.user)
REDACTED catch (error) {
  console.error('Login failed:', error)
REDACTED

// Check authentication
if (authStore.isAuthenticated) {
  console.log('User is logged in:', authStore.user?.username)
REDACTED

// Logout
authStore.logout()
```

### App Store

```typescript
import { useAppStore REDACTED from '@/stores'

// In component setup
const appStore = useAppStore()

// Sidebar control
appStore.toggleSidebar()
appStore.setSidebarCollapsed(true)

// Loading state
appStore.setLoading(true)
// ... do work
appStore.setLoading(false)

// Or use helper
await appStore.withLoading(async () => {
  const data = await fetchData()
  return data
REDACTED)

// Toast notifications
appStore.showSuccess('Operation completed!')
appStore.showError('Something went wrong!', 5000)
appStore.showInfo('FYI: This is informational')
appStore.showWarning('Be careful!')

// Custom toast
const toastId = appStore.showToast('info', 'Custom message', undefined) // No auto-dismiss
// Later...
appStore.hideToast(toastId)
```

### Combined Usage in Vue Component

```vue
<script setup lang="ts">
import { useAuthStore, useAppStore REDACTED from '@/stores'
import { onMounted REDACTED from 'vue'

const authStore = useAuthStore()
const appStore = useAppStore()

onMounted(() => {
  // Check for existing session
  authStore.checkAuth()
REDACTED)

async function handleLogin(username: string, password: string) {
  try {
    await appStore.withLoading(async () => {
      await authStore.login({ username, password REDACTED)
    REDACTED)
    appStore.showSuccess('Welcome back!')
  REDACTED catch (error) {
    appStore.showError('Login failed. Please check your credentials.')
  REDACTED
REDACTED

async function handleLogout() {
  authStore.logout()
  appStore.showInfo('You have been logged out.')
REDACTED
</script>

<template>
  <div>
    <button @click="appStore.toggleSidebar">Toggle Sidebar</button>

    <div v-if="appStore.loading">Loading...</div>

    <div v-if="authStore.isAuthenticated">
      Welcome, {{ authStore.user?.username REDACTEDREDACTED!
      <button @click="handleLogout">Logout</button>
    </div>
    <div v-else>
      <button @click="handleLogin('user', 'pass')">Login</button>
    </div>
  </div>
</template>
```

## Persistence

- **Auth Store**: Token and user data are automatically persisted to `localStorage`
  - Keys: `auth_token`, `auth_user`
  - Restored on `checkAuth()` call
- **App Store**: No persistence (UI state resets on page reload)

## TypeScript Support

All stores are fully typed with TypeScript. Import types from `@/types`:

```typescript
import type { User, Toast, ToastType REDACTED from '@/types'
```

## Testing

Stores can be reset to initial state:

```typescript
// Auth store
authStore.logout() // Clears all auth state

// App store
appStore.reset() // Resets to defaults
```
