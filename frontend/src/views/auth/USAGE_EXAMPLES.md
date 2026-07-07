# Authentication Views Usage Examples

This document provides practical examples of how to use the authentication views in the Sub2API frontend.

## Quick Start

### 1. Login Flow

**Scenario:** User wants to log into their existing account

```typescript
// Route: /login
// Component: LoginView.vue

// User interactions:
// 1. Navigate to /login
// 2. Enter username: "john_doe"
// 3. Enter password: "MySecurePass123"
// 4. Optionally check "Remember me"
// 5. Click "Sign In"

// What happens:
// - Form validation runs (client-side)
// - If valid, authStore.login() is called
// - API request to POST /api/auth/login
// - On success:
//   - Token stored in localStorage
//   - User data stored in state
//   - Success toast: "Login successful! Welcome back."
//   - Redirect to /dashboard (or intended route)
// - On error:
//   - Error message displayed inline
//   - Error toast shown
//   - User can retry
```

### 2. Registration Flow

**Scenario:** New user wants to create an account

```typescript
// Route: /register
// Component: RegisterView.vue

// User interactions:
// 1. Navigate to /register
// 2. Enter username: "jane_smith"
// 3. Enter email: "jane@example.com"
// 4. Enter password: "SecurePass123"
// 5. Enter confirm password: "SecurePass123"
// 6. Click "Create Account"

// What happens:
// - Form validation runs (client-side)
//   - Username: 3-50 chars, alphanumeric + _ -
//   - Email: Valid format
//   - Password: 8+ chars, letters + numbers
//   - Passwords match
// - If valid, authStore.register() is called
// - API request to POST /api/auth/register
// - On success:
//   - Token stored in localStorage
//   - User data stored in state
//   - Success toast: "Account created successfully! Welcome to Sub2API."
//   - Redirect to /dashboard
// - On error:
//   - Error message displayed inline
//   - Error toast shown
//   - User can retry
```

## Code Examples

### Importing the Views

```typescript
// Method 1: Direct import
import LoginView from '@/views/auth/LoginView.vue'
import RegisterView from '@/views/auth/RegisterView.vue'

// Method 2: Named exports from index
import { LoginView, RegisterView } from '@/views/auth'

// Method 3: Lazy loading (recommended for routes)
const LoginView = () => import('@/views/auth/LoginView.vue')
const RegisterView = () => import('@/views/auth/RegisterView.vue')
```

### Using in Router

```typescript
import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/views/auth/RegisterView.vue'),
    meta: { requiresAuth: false }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
```

### Navigation to Auth Views

```typescript
// From template
<router-link to="/login">Login</router-link>
<router-link to="/register">Sign Up</router-link>

// From script
import { useRouter } from 'vue-router';

const router = useRouter();

// Navigate to login
router.push('/login');

// Navigate to register
router.push('/register');

// Navigate with redirect query
router.push({
  path: '/login',
  query: { redirect: '/dashboard' }
});
```

### Programmatic Auth Flow

```typescript
import { useAuthStore } from '@/stores'
import { useAppStore } from '@/stores'
import { useRouter } from 'vue-router'

const authStore = useAuthStore()
const appStore = useAppStore()
const router = useRouter()

// Login
async function login() {
  try {
    await authStore.login({
      username: 'john_doe',
      password: 'MySecurePass123'
    })

    appStore.showSuccess('Login successful!')
    router.push('/dashboard')
  } catch (error) {
    appStore.showError('Login failed. Please check your credentials.')
  }
}

// Register
async function register() {
  try {
    await authStore.register({
      username: 'jane_smith',
      email: 'jane@example.com',
      password: 'SecurePass123'
    })

    appStore.showSuccess('Account created successfully!')
    router.push('/dashboard')
  } catch (error) {
    appStore.showError('Registration failed. Please try again.')
  }
}
```

## Validation Examples

### Login Validation

```typescript
// Valid inputs
✅ Username: "john_doe" (3+ chars)
✅ Password: "SecurePass123" (6+ chars)

// Invalid inputs
❌ Username: "jo" → Error: "Username must be at least 3 characters"
❌ Password: "12345" → Error: "Password must be at least 6 characters"
❌ Username: "" → Error: "Username is required"
❌ Password: "" → Error: "Password is required"
```

### Registration Validation

```typescript
// Valid inputs
✅ Username: "jane_smith" (3-50 chars, alphanumeric + _ -)
✅ Email: "jane@example.com" (valid format)
✅ Password: "SecurePass123" (8+ chars, letters + numbers)
✅ Confirm: "SecurePass123" (matches password)

// Invalid inputs
❌ Username: "ja" → Error: "Username must be at least 3 characters"
❌ Username: "jane@smith" → Error: "Username can only contain letters, numbers, underscores, and hyphens"
❌ Email: "invalid-email" → Error: "Please enter a valid email address"
❌ Password: "short" → Error: "Password must be at least 8 characters with letters and numbers"
❌ Password: "12345678" → Error: "Password must be at least 8 characters with letters and numbers" (no letters)
❌ Password: "password" → Error: "Password must be at least 8 characters with letters and numbers" (no numbers)
❌ Confirm: "DifferentPass" → Error: "Passwords do not match"
```

## Error Handling Examples

### Backend Errors

```typescript
// Example 1: Username already exists
{
  response: {
    data: {
      detail: "Username 'john_doe' is already taken"
    }
  }
}
// Displayed: "Username 'john_doe' is already taken"

// Example 2: Invalid credentials
{
  response: {
    data: {
      detail: 'Invalid username or password'
    }
  }
}
// Displayed: "Invalid username or password"

// Example 3: Network error
{
  message: 'Network Error'
}
// Displayed: "Network Error" + Error toast

// Example 4: Unknown error
{
}
// Displayed: "Login failed. Please check your credentials and try again." (default)
```

### Client-side Validation Errors

```typescript
// Multiple validation errors displayed simultaneously
errors = {
  username: 'Username must be at least 3 characters',
  email: 'Please enter a valid email address',
  password: 'Password must be at least 8 characters with letters and numbers',
  confirmPassword: 'Passwords do not match'
}

// Each error appears below its respective input field with red styling
```

## Testing Examples

### Unit Test: Login View

```typescript
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import LoginView from '@/views/auth/LoginView.vue'

describe('LoginView', () => {
  it('validates required fields', async () => {
    const wrapper = mount(LoginView, {
      global: {
        plugins: [createPinia()]
      }
    })

    // Submit empty form
    await wrapper.find('form').trigger('submit')

    // Check for validation errors
    expect(wrapper.text()).toContain('Username is required')
    expect(wrapper.text()).toContain('Password is required')
  })

  it('calls authStore.login on valid submission', async () => {
    const wrapper = mount(LoginView, {
      global: {
        plugins: [createPinia()]
      }
    })

    // Fill in form
    await wrapper.find('#username').setValue('john_doe')
    await wrapper.find('#password').setValue('SecurePass123')

    // Submit form
    await wrapper.find('form').trigger('submit')

    // Verify authStore.login was called
    // (mock implementation needed)
  })
})
```

### E2E Test: Registration Flow

```typescript
import { test, expect } from '@playwright/test'

test('user can register successfully', async ({ page }) => {
  // Navigate to register page
  await page.goto('/register')

  // Fill in registration form
  await page.fill('#username', 'new_user')
  await page.fill('#email', 'new_user@example.com')
  await page.fill('#password', 'SecurePass123')
  await page.fill('#confirmPassword', 'SecurePass123')

  // Submit form
  await page.click('button[type="submit"]')

  // Wait for redirect to dashboard
  await page.waitForURL('/dashboard')

  // Verify success toast appears
  await expect(page.locator('.toast-success')).toBeVisible()
  await expect(page.locator('.toast-success')).toContainText('Account created successfully')
})

test('shows validation errors for invalid inputs', async ({ page }) => {
  await page.goto('/register')

  // Enter mismatched passwords
  await page.fill('#password', 'SecurePass123')
  await page.fill('#confirmPassword', 'DifferentPass')

  // Submit form
  await page.click('button[type="submit"]')

  // Verify error message
  await expect(page.locator('text=Passwords do not match')).toBeVisible()
})
```

## Integration with Navigation Guards

### Router Guard Example

```typescript
import { useAuthStore } from '@/stores'

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()

  // Redirect authenticated users away from auth pages
  if (authStore.isAuthenticated && (to.path === '/login' || to.path === '/register')) {
    next('/dashboard')
    return
  }

  // Redirect unauthenticated users to login
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next({
      path: '/login',
      query: { redirect: to.fullPath }
    })
    return
  }

  next()
})
```

## Customization Examples

### Custom Success Redirect

```typescript
// In LoginView.vue
async function handleLogin(): Promise<void> {
  try {
    await authStore.login({
      username: formData.username,
      password: formData.password
    })

    appStore.showSuccess('Login successful!')

    // Custom redirect logic
    const isAdmin = authStore.isAdmin
    const redirectTo = isAdmin ? '/admin/dashboard' : '/dashboard'

    await router.push(redirectTo)
  } catch (error) {
    // Error handling...
  }
}
```

### Custom Validation Rules

```typescript
// Custom password strength validation
function validatePasswordStrength(password: string): boolean {
  const hasMinLength = password.length >= 12
  const hasUpperCase = /[A-Z]/.test(password)
  const hasLowerCase = /[a-z]/.test(password)
  const hasNumber = /[0-9]/.test(password)
  const hasSpecialChar = /[!@#$%^&*(),.?":{}|<>]/.test(password)

  return hasMinLength && hasUpperCase && hasLowerCase && hasNumber && hasSpecialChar
}

// Use in validation
if (!validatePasswordStrength(formData.password)) {
  errors.password =
    'Password must be at least 12 characters with uppercase, lowercase, numbers, and special characters'
  isValid = false
}
```

### Custom Error Handling

```typescript
// In RegisterView.vue
async function handleRegister(): Promise<void> {
  try {
    await authStore.register({
      username: formData.username,
      email: formData.email,
      password: formData.password
    })

    appStore.showSuccess('Account created successfully!')
    await router.push('/dashboard')
  } catch (error: unknown) {
    const err = error as { response?: { status?: number; data?: { detail?: string } } }

    // Custom error handling based on status code
    if (err.response?.status === 409) {
      errorMessage.value =
        'This username or email is already registered. Please use a different one.'
    } else if (err.response?.status === 422) {
      errorMessage.value = 'Invalid input. Please check your information and try again.'
    } else if (err.response?.status === 500) {
      errorMessage.value = 'Server error. Please try again later.'
    } else {
      errorMessage.value = err.response?.data?.detail || 'Registration failed. Please try again.'
    }

    appStore.showError(errorMessage.value)
  }
}
```

## Accessibility Examples

### Keyboard Navigation

```typescript
// Tab order:
// 1. Username input
// 2. Password input
// 3. Remember me checkbox (login) / Confirm password (register)
// 4. Submit button
// 5. Footer link (register/login)

// Enter key submits form
// Escape key can be used to clear focus
```

### Screen Reader Support

```html
<!-- Proper labels for screen readers -->
<label for="username" class="mb-1 block text-sm font-medium text-gray-700"> Username </label>
<input
  id="username"
  type="text"
  aria-label="Username"
  aria-required="true"
  aria-invalid="false"
  aria-describedby="username-error"
/>
<p id="username-error" role="alert" class="text-sm text-red-600">
  <!-- Error message here -->
</p>

<!-- Loading state announced -->
<button type="submit" aria-busy="true" aria-label="Signing in...">
  <span class="sr-only">Signing in...</span>
  <!-- Visual content -->
</button>
```

## Performance Considerations

### Lazy Loading

```typescript
// Router configuration with lazy loading
{
  path: '/login',
  component: () => import('@/views/auth/LoginView.vue'), // ✅ Lazy loaded
}

// Direct import (not recommended for routes)
import LoginView from '@/views/auth/LoginView.vue'; // ❌ Eager loaded
```

### Optimization Tips

1. Use `v-once` for static content
2. Debounce expensive validation operations
3. Minimize reactive dependencies
4. Use `shallowRef` for complex objects when possible
5. Avoid unnecessary watchers

## Security Best Practices

1. Never log passwords or tokens
2. Use HTTPS in production
3. Implement rate limiting on backend
4. Validate all inputs server-side
5. Use secure password hashing (bcrypt, argon2)
6. Implement CSRF protection
7. Set secure cookie flags
8. Use Content Security Policy headers
9. Sanitize all user inputs
10. Implement account lockout after failed attempts

## Common Issues and Solutions

### Issue: Token not persisting after refresh

```typescript
// Solution: Initialize auth state on app mount
// In main.ts or App.vue
import { useAuthStore } from '@/stores'

const authStore = useAuthStore()
authStore.checkAuth() // Restore auth from localStorage
```

### Issue: Redirect loop after login

```typescript
// Solution: Check router guard logic
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()

  // ✅ Correct: Check specific routes
  if (authStore.isAuthenticated && (to.path === '/login' || to.path === '/register')) {
    next('/dashboard')
    return
  }

  // ❌ Wrong: Blanket redirect
  // if (authStore.isAuthenticated) {
  //   next('/dashboard'); // This causes loops!
  // }

  next()
})
```

### Issue: Form not clearing after successful submission

```typescript
// Solution: Reset form data
async function handleLogin(): Promise<void> {
  try {
    await authStore.login({...});

    // Reset form
    formData.username = '';
    formData.password = '';
    formData.remember = false;

    // Clear errors
    errors.username = '';
    errors.password = '';

    await router.push('/dashboard');
  } catch (error) {
    // Error handling...
  }
}
```

## Additional Resources

- [Vue 3 Documentation](https://vuejs.org/)
- [Vue Router Documentation](https://router.vuejs.org/)
- [Pinia Documentation](https://pinia.vuejs.org/)
- [TailwindCSS Documentation](https://tailwindcss.com/)
- [TypeScript Handbook](https://www.typescriptlang.org/docs/)
