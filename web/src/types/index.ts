export interface CertificateInfo {
  domain: string
  status: 'valid' | 'expiring_soon' | 'expired' | 'self_signed' | 'error' | 'renewing'
  is_valid: boolean
  is_self_signed: boolean
  issuer?: string
  subject?: string
  issued_at?: string
  expires_at?: string
  days_remaining: number
  last_error?: string
  mode: string
  serial?: string
  dns_names?: string[]
}

export interface ProxyTarget {
  id: string
  name: string
  domain: string
  host: string
  scheme: string
  force_ssl: boolean
  require_auth: boolean
  auth_resource?: string
  custom_headers?: Record<string, string>
  pass_host_header: boolean
  enabled: boolean
  cloudflare_sync: boolean
  created_at: string
  updated_at: string
  // Merged health and TLS status
  is_healthy?: boolean
  latency_ms?: number
  last_checked?: string
  last_error?: string
  tls_cert?: CertificateInfo
}

export interface DNSRecordStatus {
  domain: string
  record_id: string
  type: string
  content: string
  proxied: boolean
  status: 'synced' | 'updated' | 'error' | 'pending'
  message?: string
  last_updated: string
}

export interface SyncLogEntry {
  timestamp: string
  ip: string
  success: boolean
  message: string
  updated?: string[]
}

export interface DDNSStatus {
  enabled: boolean
  current_ip: string
  previous_ip: string
  last_check_time: string
  last_sync_time: string
  last_error?: string
  records: Record<string, DNSRecordStatus>
  history: SyncLogEntry[]
}

export interface CloudflareConfig {
  enabled: boolean
  api_token: string
  api_key: string
  email: string
  zone_id: string
  zone_name: string
  proxied: boolean
  ttl: number
  interval_seconds: number
  domains: string[]
  auto_sync_proxies: boolean
}

export interface TrafficMetrics {
  total_requests: number
  status_2xx: number
  status_3xx: number
  status_4xx: number
  status_5xx: number
  total_bytes: number
}

export interface StatsResponse {
  metrics: TrafficMetrics
  healthy_targets: number
  total_targets: number
  uptime_seconds: number
}

export interface RequestLog {
  id: string
  timestamp: string
  client_ip: string
  method: string
  host: string
  path: string
  target: string
  status_code: number
  duration_ms: number
  bytes_sent: number
  error?: string
}

export interface SystemLog {
  id: string
  timestamp: string
  level: 'INFO' | 'WARN' | 'ERROR' | 'DEBUG'
  source: 'TLS' | 'ACME' | 'DDNS' | 'PROXY' | 'AUTH' | 'SYSTEM' | string
  message: string
  details?: string
}

export interface TLSConfig {
  mode: 'autocert' | 'custom' | 'none'
  email: string
  cert_path?: string
  key_path?: string
}

export interface SystemSettings {
  admin_username: string
  admin_port: number
  http_port: number
  https_port: number
  data_dir: string
  tls: TLSConfig
}

export interface UserInfo {
  username: string
}

export interface CFZone {
  id: string
  name: string
}
