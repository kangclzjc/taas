/**
 * TaaS API Client
 * Type-safe wrapper around the TaaS REST API.
 *
 * Auth strategy:
 * - Access token stored in httpOnly cookie (set by backend via Set-Cookie)
 * - Refresh token stored in httpOnly cookie
 * - Credentials included via fetch({ credentials: 'same-origin' })
 * - Fallback: for programmatic/CLI usage, Bearer token header is still supported
 * - Auto-refresh: 401 responses trigger a transparent token refresh + retry
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

let isRefreshing = false;
let refreshQueue: Array<{ resolve: () => void; reject: (err: Error) => void }> = [];

function processRefreshQueue(error: Error | null) {
  refreshQueue.forEach(({ resolve, reject }) => {
    if (error) reject(error);
    else resolve();
  });
  refreshQueue = [];
}

async function attemptRefresh(): Promise<void> {
  const res = await fetch(`${BASE_URL}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({}),
  });
  if (!res.ok) {
    throw new APIError('REFRESH_FAILED', 'Token refresh failed', res.status);
  }
  // New tokens are set via Set-Cookie by the backend
}

async function request<T>(
  path: string,
  options: RequestInit = {},
  _isRetry = false,
): Promise<T> {
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: 'same-origin', // Send httpOnly cookies automatically
  });

  // Auto-refresh on 401 (unless this is already a retry or a refresh/login/register request)
  if (
    res.status === 401 &&
    !_isRetry &&
    !path.startsWith('/auth/login') &&
    !path.startsWith('/auth/register') &&
    !path.startsWith('/auth/refresh')
  ) {
    if (!isRefreshing) {
      isRefreshing = true;
      try {
        await attemptRefresh();
        processRefreshQueue(null);
      } catch (err) {
        processRefreshQueue(err as Error);
        // Refresh failed — redirect to login or throw
        throw new APIError('SESSION_EXPIRED', 'Session expired, please log in again', 401);
      } finally {
        isRefreshing = false;
      }
      // Retry the original request
      return request<T>(path, options, true);
    } else {
      // Another refresh is in progress — wait for it
      return new Promise<T>((resolve, reject) => {
        refreshQueue.push({
          resolve: () => resolve(request<T>(path, options, true)),
          reject,
        });
      });
    }
  }

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

  changePassword: (data: { current_password: string; new_password: string }) =>
    request('/auth/change-password', { method: 'POST', body: JSON.stringify(data) }),

  refresh: () =>
    request<LoginResponse>('/auth/refresh', {
      method: 'POST',
      body: JSON.stringify({}),
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
  name: string;
  deploy_mode: 'dgdr' | 'dgd';
  sla_tier?: 'standard' | 'professional' | 'enterprise';
  // Hardware
  gpu_type?: string;
  gpu_count_per_replica?: number;
  num_gpus_per_node?: number;
  vram_mb?: number;
  // Scaling
  replicas_min?: number;
  replicas_max?: number;
  // Engine
  backend?: 'vllm' | 'sglang' | 'trtllm';
  backend_image?: string;
  // Parallelism
  tensor_parallel_size?: number;
  pipeline_parallel_size?: number;
  // Workload (DGDR)
  input_sequence_length?: number;
  output_sequence_length?: number;
  // SLA (DGDR)
  target_ttft_ms?: number;
  target_itl_ms?: number;
  target_tpot_ms?: number;
  search_strategy?: 'rapid' | 'thorough';
  // Disaggregated
  disagg_enabled?: boolean;
  prefill_replicas?: number;
  decode_replicas?: number;
  // DGD-specific
  frontend_replicas?: number;
  worker_command?: string;
  dynamo_namespace?: string;
  router_mode?: 'random' | 'kv';
  env_vars?: Record<string, string>;
  // Advanced
  max_batch_size?: number;
  max_sequence_length?: number;
  dtype?: string;
  extra_args?: Record<string, string>;
}

export interface Deployment {
  id: string;
  model_id: string;
  name: string;
  status: string;
  deploy_mode: string;
  backend: string;
  gpu_type: string;
  gpu_count_per_replica: number;
  tensor_parallel_size: number;
  pipeline_parallel_size: number;
  disagg_enabled: boolean;
  prefill_replicas: number;
  decode_replicas: number;
  replicas_min: number;
  replicas_max: number;
  replicas_current: number;
  endpoint_url: string | null;
  error_message: string;
  created_at: string;
}

export const models = {
  list: (params?: { public?: boolean; framework?: string }) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return request<{ items: Model[]; total: number }>(`/models${qs}`);
  },
  get: (id: string) => request<Model>(`/models/${id}`),
  getDeployments: (modelId: string) =>
    request<{ deployments: Deployment[] }>(`/models/${modelId}/deployments`),
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
  list: (params?: { limit?: number; offset?: number }) => {
    const qs = params ? '?' + new URLSearchParams(
      Object.entries(params).reduce((acc, [k, v]) => { if (v != null) acc[k] = String(v); return acc; }, {} as Record<string, string>)
    ).toString() : '';
    return request<{ items: APIToken[]; limit: number; offset: number }>(`/tokens${qs}`);
  },
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
  timeseries: (params?: { start_date?: string; end_date?: string; granularity?: string }) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return request<any>(`/usage/timeseries${qs}`);
  },
  currentPeriod: () => request('/billing/current'),
  history: () => request('/billing/history'),
};

// ─── Organizations ────────────────────────────────────────────────────────

export interface Organization {
  id: string;
  slug: string;
  display_name: string;
  sla_tier: string;
  created_at: string;
}

export interface OrgMember {
  user_id: string;
  email?: string;
  role: string;
  joined_at: string;
}

export const organizations = {
  list: () =>
    request<{ organizations: Organization[] }>('/organizations'),
  create: (data: { slug: string; display_name: string }) =>
    request<Organization>('/organizations', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  get: (id: string) => request<Organization>(`/organizations/${id}`),
  update: (id: string, data: { display_name: string }) =>
    request<Organization>(`/organizations/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id: string) =>
    request(`/organizations/${id}`, { method: 'DELETE' }),
  listMembers: (id: string) =>
    request<{ members: OrgMember[] }>(`/organizations/${id}/members`),
  addMember: (id: string, data: { email: string; role: string }) =>
    request<OrgMember>(`/organizations/${id}/members`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  removeMember: (orgId: string, userId: string) =>
    request(`/organizations/${orgId}/members/${userId}`, {
      method: 'DELETE',
    }),
};
