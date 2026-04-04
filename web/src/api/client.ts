/**
 * TaaS API Client
 * Type-safe wrapper around the TaaS REST API.
 */

const BASE_URL = import.meta.env.VITE_API_URL ?? '/api';

export class APIError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly status: number,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = 'APIError';
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = localStorage.getItem('taas_access_token');
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...options.headers,
  };

  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers });

  if (!res.ok) {
    const body = await res.json().catch(() => ({ code: 'UNKNOWN', message: res.statusText }));
    throw new APIError(
      body.code ?? 'UNKNOWN',
      body.message ?? res.statusText,
      res.status,
      res.headers.get('x-request-id') ?? undefined,
    );
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

// ─── Auth ──────────────────────────────────────────────────────────────────

export interface LoginRequest { email: string; password: string }
export interface LoginResponse { access_token: string; refresh_token: string; expires_in: number }
export interface UserProfile { id: string; email: string; name: string; role: string; org_id: string }

export const auth = {
  login: (data: LoginRequest) =>
    request<LoginResponse>('/auth/login', { method: 'POST', body: JSON.stringify(data) }),

  register: (data: { email: string; password: string; name: string; org_name: string }) =>
    request<UserProfile>('/auth/register', { method: 'POST', body: JSON.stringify(data) }),

  me: () => request<UserProfile>('/auth/me'),

  refresh: (refreshToken: string) =>
    request<LoginResponse>('/auth/refresh', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  logout: () => request('/auth/logout', { method: 'POST' }),
};

// ─── Models ───────────────────────────────────────────────────────────────

export interface Model {
  id: string; name: string; description: string;
  framework: string; format: string; status: string;
  is_public: boolean; parameter_count: number;
  context_length: number; created_at: string;
}

export interface DeploymentConfig {
  replicas_min: number; replicas_max: number;
  gpu_type: string; sla_tier: 'standard' | 'professional' | 'enterprise';
  max_batch_size: number; max_sequence_length: number;
}

export const models = {
  list: (params?: { public?: boolean; framework?: string }) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return request<{ items: Model[]; total: number }>(`/models${qs}`);
  },
  get: (id: string) => request<Model>(`/models/${id}`),
  deploy: (id: string, config: DeploymentConfig) =>
    request(`/models/${id}/deploy`, { method: 'POST', body: JSON.stringify(config) }),
  undeploy: (id: string) =>
    request(`/models/${id}/undeploy`, { method: 'POST' }),
};

// ─── Tokens ───────────────────────────────────────────────────────────────

export interface APIToken {
  id: string; name: string; token_prefix: string;
  scopes: string[]; rate_limit_rpm: number;
  expires_at: string | null; is_active: boolean;
  created_at: string; last_used_at: string | null;
}

export const tokens = {
  list: () => request<{ items: APIToken[] }>('/tokens'),
  create: (data: {
    name: string; model_ids?: string[]; scopes: string[];
    rate_limit_rpm?: number; expires_at?: string; budget_limit_usd?: number;
  }) => request<APIToken & { token_value: string }>('/tokens', {
    method: 'POST', body: JSON.stringify(data),
  }),
  revoke: (id: string) => request(`/tokens/${id}`, { method: 'DELETE' }),
  rotate: (id: string) =>
    request<{ token_value: string; expires_at: string }>(`/tokens/${id}/rotate`, { method: 'POST' }),
};

// ─── Usage & Billing ──────────────────────────────────────────────────────

export interface UsageSummary {
  total_requests: number; total_tokens: number;
  prompt_tokens: number; completion_tokens: number;
  total_cost_usd: number; period_start: string; period_end: string;
}

export const billing = {
  usage: (params?: { start_date?: string; end_date?: string; model_id?: string }) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return request<UsageSummary>(`/usage${qs}`);
  },
  currentPeriod: () => request('/billing/current'),
  history: () => request('/billing/history'),
};
