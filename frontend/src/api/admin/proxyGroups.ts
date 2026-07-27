/**
 * Admin Proxy Groups API — 代理池/代理分组管理
 */

import { apiClient } from "../client";
import type {
	ProxyGroup,
	CreateProxyGroupRequest,
	UpdateProxyGroupRequest,
	PaginatedResponse,
} from "@/types";

export async function list(
	page: number = 1,
	pageSize: number = 20,
	filters?: {
		sort_by?: string;
		sort_order?: "asc" | "desc";
	},
	options?: { signal?: AbortSignal },
): Promise<PaginatedResponse<ProxyGroup>> {
	const { data } = await apiClient.get<PaginatedResponse<ProxyGroup>>(
		"/admin/proxy-groups",
		{
			params: {
				page,
				page_size: pageSize,
				...filters,
			},
			signal: options?.signal,
		},
	);
	return data;
}

export async function getAll(): Promise<ProxyGroup[]> {
	const { data } = await apiClient.get<ProxyGroup[]>("/admin/proxy-groups/all");
	return data;
}

export async function getById(id: number): Promise<ProxyGroup> {
	const { data } = await apiClient.get<ProxyGroup>(`/admin/proxy-groups/${id}`);
	return data;
}

export async function create(
	payload: CreateProxyGroupRequest,
): Promise<ProxyGroup> {
	const { data } = await apiClient.post<ProxyGroup>(
		"/admin/proxy-groups",
		payload,
	);
	return data;
}

export async function update(
	id: number,
	payload: UpdateProxyGroupRequest,
): Promise<ProxyGroup> {
	const { data } = await apiClient.put<ProxyGroup>(
		`/admin/proxy-groups/${id}`,
		payload,
	);
	return data;
}

export async function deleteGroup(id: number): Promise<void> {
	await apiClient.delete(`/admin/proxy-groups/${id}`);
}

export async function setMembers(
	id: number,
	proxyIds: number[],
): Promise<ProxyGroup> {
	const { data } = await apiClient.put<ProxyGroup>(
		`/admin/proxy-groups/${id}/members`,
		{
			proxy_ids: proxyIds,
		},
	);
	return data;
}

export const proxyGroupsAPI = {
	list,
	getAll,
	getById,
	create,
	update,
	delete: deleteGroup,
	setMembers,
};

export default proxyGroupsAPI;
