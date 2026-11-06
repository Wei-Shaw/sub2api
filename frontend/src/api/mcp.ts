import { apiClient } from './client'

export type MCPAccessScope = 'user' | 'admin'

export interface MCPGrant {
  id: string
  client_id: string
  client_name: string
  scope: MCPAccessScope
  read_only: boolean
  created_at: string
  expires_at: string
  last_used_at: string | null
  revoked_at: string | null
  invalidated: boolean
}

export interface MCPAuthorization {
  response_type: string
  client_id: string
  redirect_uri: string
  scope?: string
  state?: string
  code_challenge: string
  code_challenge_method: string
  resource: string
}

export interface MCPConsent {
  client_id: string
  client_name: string
  redirect_uri: string
  resource: string
  scope: 'mcp'
}

export interface MCPAuthorizationDecision {
  authorization: MCPAuthorization
  approved: boolean
  scope: MCPAccessScope
  read_only: boolean
}

export const mcpAPI = {
  async getConnectionConfig(): Promise<{ resource: string }> {
    const { data } = await apiClient.get<{ resource: string }>('/user/mcp/config')
    return data
  },
  async listGrants(): Promise<MCPGrant[]> {
    const { data } = await apiClient.get<MCPGrant[]>('/user/mcp/grants')
    return data
  },
  async revokeGrant(id: string): Promise<void> {
    await apiClient.delete(`/user/mcp/grants/${encodeURIComponent(id)}`)
  },
  async previewAuthorization(authorization: MCPAuthorization): Promise<MCPConsent> {
    const { data } = await apiClient.get<MCPConsent>('/user/mcp/authorize', { params: authorization })
    return data
  },
  async authorize(input: MCPAuthorizationDecision): Promise<{ redirect_url: string }> {
    const { data } = await apiClient.post<{ redirect_url: string }>('/user/mcp/authorize', input)
    return data
  }
}
