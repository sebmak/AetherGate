import type {
  ProxyTarget,
  DDNSStatus,
  CloudflareConfig,
  StatsResponse,
  RequestLog,
  SystemLog,
  SystemSettings,
  CFZone,
  CertificateInfo,
} from '../types'

const API_BASE = '/api'

class ApiError extends Error {
  code?: string
  status: number

  constructor(message: string, status: number, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

function getAuthHeader(): Record<string, string> {
  const token = localStorage.getItem('proxyserver_token')
  if (token) {
    return { Authorization: `Bearer ${token}` }
  }
  return {}
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const url = `${API_BASE}${endpoint}`
  const headers = {
    'Content-Type': 'application/json',
    ...getAuthHeader(),
    ...options.headers,
  }

  const res = await fetch(url, {
    ...options,
    headers,
    credentials: 'include', // Includes HTTP-only session cookies
  })

  if (res.status === 401) {
    const errorData = await res.json().catch(() => ({}))
    if (errorData.code === 'SETUP_REQUIRED') {
      window.dispatchEvent(new CustomEvent('auth:setup-required'))
    } else {
      localStorage.removeItem('proxyserver_token')
      window.dispatchEvent(new CustomEvent('auth:unauthorized'))
    }
    throw new ApiError(errorData.error || 'Unauthorized', 401, errorData.code)
  }

  if (!res.ok) {
    const errorData = await res.json().catch(() => ({}))
    throw new ApiError(errorData.error || `Request failed (${res.status})`, res.status)
  }

  return res.json()
}

export const api = {
  // Setup & Auth
  async getSetupStatus(): Promise<{ is_configured: boolean }> {
    return request<{ is_configured: boolean }>('/setup/status')
  },

  async performSetup(username: string, password: string, legacyConfig?: string): Promise<{ success: boolean; token: string; username: string; imported_count?: number }> {
    const res = await request<{ success: boolean; token: string; username: string; imported_count?: number }>('/setup', {
      method: 'POST',
      body: JSON.stringify({
        username,
        password,
        legacy_config: legacyConfig ? legacyConfig.trim() : undefined,
      }),
    })
    if (res.token) {
      localStorage.setItem('proxyserver_token', res.token)
    }
    return res
  },

  async login(username: string, password: string): Promise<{ success: boolean; token: string; username: string }> {
    const res = await request<{ success: boolean; token: string; username: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
    if (res.token) {
      localStorage.setItem('proxyserver_token', res.token)
    }
    return res
  },

  async logout(): Promise<void> {
    try {
      await request('/auth/logout', { method: 'POST' })
    } finally {
      localStorage.removeItem('proxyserver_token')
    }
  },

  async getMe(): Promise<{ username: string }> {
    return request<{ username: string }>('/auth/me')
  },

  // Stats & Health
  async getStats(): Promise<StatsResponse> {
    return request<StatsResponse>('/stats')
  },

  async getHealth(): Promise<{ status: string; uptime_s: number; version: string }> {
    return request('/health')
  },

  // Proxies
  async getProxies(): Promise<ProxyTarget[]> {
    return request<ProxyTarget[]>('/proxies')
  },

  async createProxy(data: Partial<ProxyTarget>): Promise<ProxyTarget> {
    return request<ProxyTarget>('/proxies', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  },

  async updateProxy(id: string, data: Partial<ProxyTarget>): Promise<ProxyTarget> {
    return request<ProxyTarget>(`/proxies/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  },

  async deleteProxy(id: string): Promise<void> {
    await request(`/proxies/${id}`, {
      method: 'DELETE',
    })
  },

  async toggleProxy(id: string): Promise<ProxyTarget> {
    return request<ProxyTarget>(`/proxies/${id}/toggle`, {
      method: 'POST',
    })
  },

  async testUpstream(scheme: string, host: string): Promise<{ success: boolean; latency_ms: number; message: string }> {
    return request('/proxies/test-upstream', {
      method: 'POST',
      body: JSON.stringify({ scheme, host }),
    })
  },

  async renewCertificate(id: string): Promise<{ success: boolean; cert: CertificateInfo; error?: string }> {
    return request<{ success: boolean; cert: CertificateInfo; error?: string }>(`/proxies/${id}/renew-cert`, {
      method: 'POST',
    })
  },

  async renewAllCertificates(): Promise<{ success: boolean; certs: CertificateInfo[]; error?: string }> {
    return request<{ success: boolean; certs: CertificateInfo[]; error?: string }>('/settings/tls/renew-all', {
      method: 'POST',
    })
  },

  async getTLSCerts(): Promise<{ certs: CertificateInfo[] }> {
    return request<{ certs: CertificateInfo[] }>('/settings/tls/certs')
  },

  // Cloudflare DDNS
  async getDDNS(): Promise<{ status: DDNSStatus; config: CloudflareConfig }> {
    return request<{ status: DDNSStatus; config: CloudflareConfig }>('/ddns/status')
  },

  async updateDDNSConfig(data: Partial<CloudflareConfig>): Promise<{ success: boolean }> {
    return request('/ddns/config', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  },

  async triggerDDNSSync(): Promise<{ message: string }> {
    return request('/ddns/sync', {
      method: 'POST',
    })
  },

  async testCloudflareToken(api_token: string, api_key?: string, email?: string): Promise<{ valid: boolean; zones?: CFZone[]; error?: string; warning?: string }> {
    return request('/ddns/test-token', {
      method: 'POST',
      body: JSON.stringify({ api_token, api_key, email }),
    })
  },

  // Logs
  async getLogs(limit = 100): Promise<RequestLog[]> {
    return request<RequestLog[]>(`/logs?limit=${limit}`)
  },

  async clearLogs(): Promise<{ success: boolean }> {
    return request<{ success: boolean }>('/logs', {
      method: 'DELETE',
    })
  },

  async getSystemLogs(params?: { limit?: number; level?: string; source?: string; search?: string }): Promise<SystemLog[]> {
    const q = new URLSearchParams()
    if (params?.limit) q.set('limit', String(params.limit))
    if (params?.level) q.set('level', params.level)
    if (params?.source) q.set('source', params.source)
    if (params?.search) q.set('search', params.search)
    const qs = q.toString() ? `?${q.toString()}` : ''
    return request<SystemLog[]>(`/system-logs${qs}`)
  },

  async clearSystemLogs(): Promise<{ success: boolean }> {
    return request<{ success: boolean }>('/system-logs', {
      method: 'DELETE',
    })
  },

  // Settings
  async getSettings(): Promise<SystemSettings> {
    return request<SystemSettings>('/settings')
  },

  async updateSettings(data: Partial<SystemSettings>): Promise<{ success: boolean }> {
    return request('/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    })
  },

  async changePassword(current_password: string, new_password: string): Promise<{ success: boolean }> {
    return request('/settings/change-password', {
      method: 'POST',
      body: JSON.stringify({ current_password, new_password }),
    })
  },

  async importLegacy(payload: string): Promise<{ success: boolean; imported_count: number }> {
    return request('/settings/import-legacy', {
      method: 'POST',
      body: JSON.stringify({ payload }),
    })
  },
}
