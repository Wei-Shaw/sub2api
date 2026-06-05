# Authentication Views

This directory contains Vue 3 authentication views for the Sub2API frontend application.

## Components

### LoginView.vue

Login page for existing users to authenticate.

**Features:**

- Username and password inputs with validation
- Remember me checkbox for persistent sessions
- Form validation with real-time error display
- Loading state during authentication
- Error message display for failed login attempts
- Redirect to dashboard on successful login
- Link to registration page for new users

**Usage:**

```vue
<template>
  <LoginView />
</template>

<script setup lang="ts">
import { LoginView } from '@/views/auth'
</script>
```

**Route:**

- Path: `/login`
- Name: `Login`
- Meta: `{ requiresAuth: false }`

**Validation Rules:**

- Username: Required, minimum 3 characters
- Password: Required, minimum 6 characters

**Behavior:**

- Calls `authStore.login()` with credentials
- Shows success toast on successful login
- Shows error toast and inline error message on failure
- Redirects to `/dashboard` or intended route from query parameter
- Redirects authenticated users away from login page

### RegisterView.vue

Registration page for new users to create accounts.

**Features:**

- Username, email, password, and confirm password inputs
- Comprehensive form validation
- Password strength requirements (8+ characters, letters + numbers)
- Email format validation with regex
- Password match validation
- Loading state during registration
- Error message display for failed registration
- Redirect to dashboard on success
- Link to login page for existing users

**Usage:**

```vue
<template>
  <RegisterView />
</template>

<script setup lang="ts">
import { RegisterView } from '@/views/auth'
</script>
```

**Route:**

- Path: `/register`
- Name: `Register`
- Meta: `{ requiresAuth: false }`

**Validation Rules:**

- Username:
  - Required
  - 3-50 characters
  - Only letters, numbers, underscores, and hyphens
- Email:
  - Required
  - Valid email format (RFC 5322 regex)
- Password:
  - Required
  - Minimum 8 characters
  - Must contain at least one letter and one number
- Confirm Password:
  - Required
  - Must match password

**Behavior:**

- Calls `authStore.register()` with user data
- Shows success toast on successful registration
- Shows error toast and inline error message on failure
- Redirects to `/dashboard` after successful registration
- Redirects authenticated users away from register page

## Architecture

### Component Structure

Both views follow a consistent structure:

```
<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <!-- Form -->
      <!-- Error Message -->
      <!-- Submit Button -->
    </div>

    <template #footer>
      <!-- Footer Links -->
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
// Imports
// State
// Validation
// Form Handlers
</script>
```

### State Management

Both views use:

- `useAuthStore()` - For authentication actions (login, register)
- `useAppStore()` - For toast notifications and UI feedback
- `useRouter()` - For navigation and redirects

### Validation Strategy

**Client-side Validation:**

- Real-time validation on form submission
- Field-level error messages
- Comprehensive validation rules
- TypeScript type safety

**Server-side Validation:**

- Backend API validates all inputs
- Error responses handled gracefully
- User-friendly error messages displayed

### Styling

**Design System:**

- TailwindCSS utility classes
- Consistent color scheme (indigo primary)
- Responsive design
- Accessible form controls
- Loading states with spinner animations

**Visual Feedback:**

- Red border on invalid fields
- Error messages below inputs
- Global error banner for API errors
- Success toasts on completion
- Loading spinner on submit button

## Dependencies

### Components

- `AuthLayout` - Layout wrapper for auth pages from `@/components/layout`

### Stores

- `authStore` - Authentication state management from `@/stores/auth`
- `appStore` - Application state and toasts from `@/stores/app`

### Libraries

- Vue 3 Composition API
- Vue Router for navigation
- Pinia for state management
- TypeScript for type safety

## Usage Examples

### Basic Login Flow

```typescript
// User enters credentials
formData.username = 'john_doe'
formData.password = 'SecurePass123'

// Submit form
await handleLogin()

// On success:
// - authStore.login() called
// - Token and user stored
// - Success toast shown
// - Redirected to /dashboard

// On error:
// - Error message displayed
// - Error toast shown
// - Form remains editable
```

### Basic Registration Flow

```typescript
// User enters registration data
formData.username = 'jane_smith'
formData.email = 'jane@example.com'
formData.password = 'SecurePass123'
formData.confirmPassword = 'SecurePass123'

// Submit form
await handleRegister()

// On success:
// - authStore.register() called
// - Token and user stored
// - Success toast shown
// - Redirected to /dashboard

// On error:
// - Error message displayed
// - Error toast shown
// - Form remains editable
```

## Error Handling

### Client-side Errors

```typescript
// Validation errors
errors.username = 'Username must be at least 3 characters'
errors.email = 'Please enter a valid email address'
errors.password = 'Password must be at least 8 characters with letters and numbers'
errors.confirmPassword = 'Passwords do not match'
```

### Server-side Errors

```typescript
// API error responses
{
  response: {
    data: {
      detail: 'Username already exists'
    }
  }
}

// Displayed as:
errorMessage.value = 'Username already exists'
appStore.showError('Username already exists')
```

## Accessibility

- Semantic HTML elements (`<label>`, `<input>`, `<button>`)
- Proper `for` attributes on labels
- ARIA attributes for loading states
- Keyboard navigation support
- Focus management
- Error announcements
- Sufficient color contrast

## Testing Considerations

### Unit Tests

- Form validation logic
- Error handling
- State management
- Router navigation

### Integration Tests

- Full login flow
- Full registration flow
- Error scenarios
- Redirect behavior

### E2E Tests

- Complete user journeys
- Form interactions
- API integration
- Success/error states

## Future Enhancements

Potential improvements:

- OAuth/SSO integration (Google, GitHub)
- Two-factor authentication (2FA)
- Password strength meter
- Email verification flow
- Forgot password functionality
- Social login buttons
- CAPTCHA integration
- Session timeout warnings
- Password visibility toggle
- Autofill support enhancement

## Security Considerations

- Passwords are never logged or displayed
- HTTPS required in production
- JWT tokens stored securely in localStorage
- CORS protection on API
- XSS protection with Vue's automatic escaping
- CSRF protection with token-based auth
- Rate limiting on backend API
- Input sanitization
- Secure password requirements

## Performance

- Lazy-loaded routes
- Minimal bundle size
- Fast initial render
- Optimized re-renders with reactive refs
- No unnecessary watchers
- Efficient form validation

## Browser Support

- Modern browsers (Chrome, Firefox, Safari, Edge)
- ES2015+ required
- Flexbox and CSS Grid
- Tailwind CSS utilities
- Vue 3 runtime

## Related Documentation

- [Auth Store Documentation](/src/stores/README.md#auth-store)
- [AuthLayout Component](/src/components/layout/README.md#authlayout)
- [Router Configuration](/src/router/index.ts)
- [API Documentation](/src/api/README.md#authentication)
- [Type Definitions](/src/types/index.ts)
