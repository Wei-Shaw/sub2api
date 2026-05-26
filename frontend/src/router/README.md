# Vue Router Configuration

## Overview

This directory contains the Vue Router configuration for the Sub2API frontend application. The router implements a comprehensive navigation system with authentication guards, role-based access control, and lazy loading.

## Files

- **index.ts**: Main router configuration with route definitions and navigation guards
- **meta.d.ts**: TypeScript type definitions for route meta fields

## Route Structure

### Public Routes (No Authentication Required)

| Path        | Component    | Description            |
| ----------- | ------------ | ---------------------- |
| `/login`    | LoginView    | User login page        |
| `/register` | RegisterView | User registration page |

### User Routes (Authentication Required)

| Path         | Component     | Description                  |
| ------------ | ------------- | ---------------------------- |
| `/`          | -             | Redirects to `/dashboard`    |
| `/dashboard` | DashboardView | User dashboard with stats    |
| `/keys`      | KeysView      | API key management           |
| `/usage`     | UsageView     | Usage records and statistics |
| `/redeem`    | RedeemView    | Redeem code interface        |
| `/profile`   | ProfileView   | User profile settings        |

### Admin Routes (Admin Role Required)

| Path               | Component          | Description                     |
| ------------------ | ------------------ | ------------------------------- |
| `/admin`           | -                  | Redirects to `/admin/dashboard` |
| `/admin/dashboard` | AdminDashboardView | Admin dashboard                 |
| `/admin/users`     | AdminUsersView     | User management                 |
| `/admin/groups`    | AdminGroupsView    | Group management                |
| `/admin/accounts`  | AdminAccountsView  | Account management              |
| `/admin/proxies`   | AdminProxiesView   | Proxy management                |
| `/admin/redeem`    | AdminRedeemView    | Redeem code management          |

### Special Routes

| Path              | Component    | Description    |
| ----------------- | ------------ | -------------- |
| `/:pathMatch(.*)` | NotFoundView | 404 error page |

## Navigation Guards

### Authentication Guard (beforeEach)

The router implements a comprehensive navigation guard that:

1. **Sets Page Title**: Updates document title based on route meta
2. **Checks Authentication**:
   - Public routes (`requiresAuth: false`) are accessible without login
   - Protected routes require authentication
   - Redirects to `/login` if not authenticated
3. **Prevents Double Login**:
   - Redirects authenticated users away from login/register pages
4. **Role-Based Access Control**:
   - Admin routes (`requiresAdmin: true`) require admin role
   - Non-admin users are redirected to `/dashboard`
5. **Preserves Intended Destination**:
   - Saves original URL in query parameter for post-login redirect

### Flow Diagram

```
User navigates to route
        ↓
Set page title from meta
        ↓
Is route public? ──Yes──→ Already authenticated? ──Yes──→ Redirect to /dashboard
        ↓ No                                        ↓ No
        ↓                                      Allow access
        ↓
Is user authenticated? ──No──→ Redirect to /login with redirect query
        ↓ Yes
        ↓
Requires admin role? ──Yes──→ Is user admin? ──No──→ Redirect to /dashboard
        ↓ No                                  ↓ Yes
        ↓                                     ↓
Allow access ←────────────────────────────────┘
```

## Route Meta Fields

Each route can define the following meta fields:

```typescript
interface RouteMeta {
  requiresAuth?: boolean // Default: true (requires authentication)
  requiresAdmin?: boolean // Default: false (admin access only)
  title?: string // Page title
  breadcrumbs?: Array<{
    // Breadcrumb navigation
    label: string
    to?: string
  }>
  icon?: string // Icon for navigation menu
  hideInMenu?: boolean // Hide from navigation menu
}
```

## Lazy Loading

All route components use dynamic imports for code splitting:

```typescript
component: () => import('@/views/user/DashboardView.vue')
```

Benefits:

- Reduced initial bundle size
- Faster initial page load
- Components loaded on-demand
- Automatic code splitting by Vite

## Authentication Store Integration

The router integrates with the Pinia auth store (`@/stores/auth`):

```typescript
const authStore = useAuthStore()

// Check authentication status
authStore.isAuthenticated

// Check admin role
authStore.isAdmin
```

## Usage Examples

### Programmatic Navigation

```typescript
import { useRouter } from 'vue-router'

const router = useRouter()

// Navigate to a route
router.push('/dashboard')

// Navigate with query parameters
router.push({
  path: '/usage',
  query: { filter: 'today' }
})

// Navigate to admin route (will be blocked if not admin)
router.push('/admin/users')
```

### Route Links

```vue
<template>
  <!-- Simple link -->
  <router-link to="/dashboard">Dashboard</router-link>

  <!-- Named route -->
  <router-link :to="{ name: 'Keys' }">API Keys</router-link>

  <!-- With query parameters -->
  <router-link :to="{ path: '/usage', query: { page: 1 } }"> Usage </router-link>
</template>
```

### Checking Current Route

```typescript
import { useRoute } from 'vue-router'

const route = useRoute()

// Check if on admin page
const isAdminPage = route.path.startsWith('/admin')

// Get route meta
const requiresAdmin = route.meta.requiresAdmin
```

## Scroll Behavior

The router implements automatic scroll management:

- **Browser Navigation**: Restores saved scroll position
- **New Routes**: Scrolls to top of page
- **Hash Links**: Scrolls to anchor (when implemented)

## Error Handling

The router includes error handling for navigation failures:

```typescript
router.onError((error) => {
  console.error('Router error:', error)
})
```

## Testing Routes

To test navigation guards and route access:

1. **Public Route Access**: Visit `/login` without authentication
2. **Protected Route**: Try accessing `/dashboard` without login (should redirect)
3. **Admin Access**: Login as regular user, try `/admin/users` (should redirect to dashboard)
4. **Admin Success**: Login as admin, access `/admin/users` (should succeed)
5. **404 Handling**: Visit non-existent route (should show 404 page)

## Development Tips

### Adding New Routes

1. Add route definition in `routes` array
2. Create corresponding view component
3. Set appropriate meta fields (`requiresAuth`, `requiresAdmin`)
4. Use lazy loading with `() => import()`
5. Update this README with route documentation

### Debugging Navigation

Enable Vue Router debug mode:

```typescript
// In browser console
window.__VUE_ROUTER__ = router

// Check current route
router.currentRoute.value
```

### Common Issues

**Issue**: 404 on page refresh

- **Cause**: Server not configured for SPA
- **Solution**: Configure server to serve `index.html` for all routes

**Issue**: Navigation guard runs twice

- **Cause**: Multiple `next()` calls
- **Solution**: Ensure only one `next()` call per code path

**Issue**: User data not loaded

- **Cause**: Auth store not initialized
- **Solution**: Call `authStore.checkAuth()` in App.vue or main.ts

## Security Considerations

1. **Client-Side Only**: Navigation guards are client-side; server must also validate
2. **Token Validation**: API should verify JWT token on every request
3. **Role Checking**: Backend must verify admin role, not just frontend
4. **XSS Protection**: Vue automatically escapes template content
5. **CSRF Protection**: Use CSRF tokens for state-changing operations

## Performance Optimization

1. **Lazy Loading**: All routes use dynamic imports
2. **Code Splitting**: Vite automatically splits route chunks
3. **Prefetching**: Consider adding route prefetch for common paths
4. **Route Caching**: Vue Router caches component instances

## Future Enhancements

- [ ] Add breadcrumb navigation system
- [ ] Implement route-based permissions beyond admin/user
- [ ] Add route transition animations
- [ ] Implement route prefetching for anticipated navigation
- [ ] Add navigation analytics tracking
