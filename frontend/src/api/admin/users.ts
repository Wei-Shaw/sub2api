/**
 * Admin Users API endpoints
 * Handles user management for administrators
 */

import { apiClient } from '../client';
import type { User, UpdateUserRequest, PaginatedResponse } from '@/types';

/**
 * List all users with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (status, role, search)
 * @returns Paginated list of users
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: 'active' | 'disabled';
    role?: 'admin' | 'user';
    search?: string;
  }
): Promise<PaginatedResponse<User>> {
  const { data } = await apiClient.get<PaginatedResponse<User>>('/admin/users', {
    params: {
      page,
      page_size: pageSize,
      ...filters,
    },
  });
  return data;
}

/**
 * Get user by ID
 * @param id - User ID
 * @returns User details
 */
export async function getById(id: number): Promise<User> {
  const { data } = await apiClient.get<User>(`/admin/users/${id}`);
  return data;
}

/**
 * Create new user
 * @param userData - User data (email, password, etc.)
 * @returns Created user
 */
export async function create(userData: {
  email: string;
  password: string;
  balance?: number;
  concurrency?: number;
  allowed_groups?: number[] | null;
}): Promise<User> {
  const { data } = await apiClient.post<User>('/admin/users', userData);
  return data;
}

/**
 * Update user
 * @param id - User ID
 * @param updates - Fields to update
 * @returns Updated user
 */
export async function update(id: number, updates: UpdateUserRequest): Promise<User> {
  const { data } = await apiClient.put<User>(`/admin/users/${id}`, updates);
  return data;
}

/**
 * Delete user
 * @param id - User ID
 * @returns Success confirmation
 */
export async function deleteUser(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/users/${id}`);
  return data;
}

/**
 * Update user balance
 * @param id - User ID
 * @param balance - New balance
 * @param operation - Operation type ('set', 'add', 'subtract')
 * @param notes - Optional notes for the balance adjustment
 * @returns Updated user
 */
export async function updateBalance(
  id: number,
  balance: number,
  operation: 'set' | 'add' | 'subtract' = 'set',
  notes?: string
): Promise<User> {
  const { data } = await apiClient.post<User>(`/admin/users/${id}/balance`, {
    balance,
    operation,
    notes: notes || '',
  });
  return data;
}

/**
 * Update user concurrency
 * @param id - User ID
 * @param concurrency - New concurrency limit
 * @returns Updated user
 */
export async function updateConcurrency(id: number, concurrency: number): Promise<User> {
  return update(id, { concurrency });
}

/**
 * Toggle user status
 * @param id - User ID
 * @param status - New status
 * @returns Updated user
 */
export async function toggleStatus(id: number, status: 'active' | 'disabled'): Promise<User> {
  return update(id, { status });
}

/**
 * Get user's API keys
 * @param id - User ID
 * @returns List of user's API keys
 */
export async function getUserApiKeys(id: number): Promise<PaginatedResponse<any>> {
  const { data } = await apiClient.get<PaginatedResponse<any>>(`/admin/users/${id}/api-keys`);
  return data;
}

/**
 * Get user's usage statistics
 * @param id - User ID
 * @param period - Time period
 * @returns User usage statistics
 */
export async function getUserUsageStats(
  id: number,
  period: string = 'month'
): Promise<{
  total_requests: number;
  total_cost: number;
  total_tokens: number;
}> {
  const { data } = await apiClient.get<{
    total_requests: number;
    total_cost: number;
    total_tokens: number;
  }>(`/admin/users/${id}/usage`, {
    params: { period },
  });
  return data;
}

export const usersAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteUser,
  updateBalance,
  updateConcurrency,
  toggleStatus,
  getUserApiKeys,
  getUserUsageStats,
};

export default usersAPI;
