import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { models, type Deployment, type DeploymentConfig, type Model } from '../api/client';
import { useToast } from '../components/Toast';
import DeployForm from '../components/DeployForm';

// LiteLLM public base shown in usage examples. Resolution order:
//  1) localStorage `taas.litellm_base_url` (lets each user pick the URL that actually reaches LiteLLM
//     from where they curl: dev box uses 14000, Mac via SSH tunnel uses 4001, prod uses real URL)
//  2) build-time env VITE_LITELLM_PUBLIC_URL
//  3) heuristic: same protocol/hostname as the TaaS UI, with port 4001 (the dev SSH-tunnel default)
const LITELLM_BASE_STORAGE_KEY = 'taas.litellm_base_url';
const MODEL_DETAIL_REFRESH_MS = 5000;

function defaultLitellmBase(): string {
  if (typeof window === 'undefined') return 'http://127.0.0.1:4001';
  const envBase = import.meta.env.VITE_LITELLM_PUBLIC_URL;
  if (envBase) return envBase;
  const stored = window.localStorage.getItem(LITELLM_BASE_STORAGE_KEY);
  if (stored) return stored;
  return `${window.location.protocol}//${window.location.hostname}:4001`;
}

function statusBadge(status: string) {
  const map: Record<string, string> = {
    deployed: 'badge-success', deploying: 'badge-warning', undeployed: 'badge-neutral',
    failed: 'badge-danger', running: 'badge-success', stopped: 'badge-neutral',
    pending: 'badge-warning',
  };
  return <span className={`badge ${map[status] ?? 'badge-neutral'}`}>{status}</span>;
}

function modeBadge(mode: string) {
  if (mode === 'dgdr') return <span className="badge badge-warning" title="Auto-profiling, SLA-driven">🔬 DGDR</span>;
  return <span className="badge badge-info" title="Direct deploy, no profiling">⚡ DGD</span>;
}

function formatParams(n: number) {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(0)}M`;
  return n.toLocaleString();
}

function parseModelSource(storageURI?: string): { kind: 'huggingface' | 'nim' | 'custom' | 'unknown'; label: string; value: string } {
  const value = (storageURI ?? '').trim();
  if (!value) return { kind: 'unknown', label: 'Unknown', value: '—' };
  if (value.startsWith('nim://')) return { kind: 'nim', label: 'NIM', value: value.replace(/^nim:\/\//, '') };
  if (!value.includes('://') && value.includes('/')) return { kind: 'huggingface', label: 'HuggingFace', value };
  return { kind: 'custom', label: 'Custom URI', value };
}

function modelSlug(m: Model): string {
  return (m.slug && m.slug.length > 0) ? m.slug : m.name.toLowerCase().replace(/\s+/g, '-');
}

interface CopyableProps {
  text: string;
  label?: string;
  multiline?: boolean;
}

function Copyable({ text, label = 'Copy', multiline = false }: CopyableProps) {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // best-effort, no toast spam
    }
  };
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}>
      <pre style={{
        flex: 1,
        margin: 0,
        padding: '10px 12px',
        background: 'var(--bg-subtle, #f6f8fa)',
        border: '1px solid var(--border, #e5e7eb)',
        borderRadius: 6,
        fontSize: 12,
        whiteSpace: multiline ? 'pre' : 'pre-wrap',
        wordBreak: multiline ? 'normal' : 'break-all',
        overflowX: 'auto',
      }}>{text}</pre>
      <button
        type="button"
        className="btn btn-sm"
        style={{ flexShrink: 0 }}
        onClick={onCopy}
      >
        {copied ? '✓ Copied' : label}
      </button>
    </div>
  );
}

export default function ModelDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [showDeployForm, setShowDeployForm] = useState(false);
  const [deletingDeploymentId, setDeletingDeploymentId] = useState<string | null>(null);
  const [litellmBase, setLitellmBase] = useState<string>(() => defaultLitellmBase());

  const saveLitellmBase = (next: string) => {
    const trimmed = next.trim().replace(/\/$/, '');
    setLitellmBase(trimmed);
    try {
      window.localStorage.setItem(LITELLM_BASE_STORAGE_KEY, trimmed);
    } catch {
      // ignore quota / private mode
    }
  };

  const { data: model, isLoading: modelLoading } = useQuery({
    queryKey: ['model', id],
    queryFn: () => models.get(id!),
    enabled: !!id,
    refetchInterval: MODEL_DETAIL_REFRESH_MS,
    refetchIntervalInBackground: false,
  });

  const { data: deploymentsData, isLoading: deploymentsLoading } = useQuery({
    queryKey: ['model-deployments', id],
    queryFn: () => models.getDeployments(id!),
    enabled: !!id,
    refetchInterval: MODEL_DETAIL_REFRESH_MS,
    refetchIntervalInBackground: false,
  });

  const deployMutation = useMutation({
    mutationFn: (config: DeploymentConfig) => models.deploy(id!, config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model', id] });
      queryClient.invalidateQueries({ queryKey: ['model-deployments', id] });
      addToast('Deployment started successfully!', 'success');
      setShowDeployForm(false);
    },
    onError: (err: any) => {
      addToast(`Failed to deploy: ${err.message ?? 'unknown error'}`, 'error');
    },
  });

  const deleteDeploymentMutation = useMutation({
    mutationFn: (deploymentId: string) => models.deleteDeployment(id!, deploymentId),
    onMutate: (deploymentId: string) => {
      setDeletingDeploymentId(deploymentId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model', id] });
      queryClient.invalidateQueries({ queryKey: ['model-deployments', id] });
      addToast('Deployment deleted', 'success');
    },
    onError: (err: any) => {
      addToast(`Failed to delete deployment: ${err.message ?? 'unknown error'}`, 'error');
    },
    onSettled: () => {
      setDeletingDeploymentId(null);
    },
  });

  if (modelLoading) {
    return (
      <div>
        <div className="page-header"><h1 className="page-title">Model Details</h1></div>
        <div className="card" style={{ padding: 24 }}><p className="text-muted">Loading…</p></div>
      </div>
    );
  }

  if (!model) {
    return (
      <div>
        <div className="page-header"><h1 className="page-title">Model Not Found</h1></div>
        <div className="card" style={{ padding: 24 }}>
          <p>The requested model could not be found.</p>
          <Link to="/models" className="btn btn-primary" style={{ marginTop: 12 }}>← Back to Models</Link>
        </div>
      </div>
    );
  }

  const deployments = (deploymentsData?.deployments ?? []).filter(
    (d: Deployment) => d.status !== 'stopped',
  );
  const source = parseModelSource(model.storage_uri);
  const nimNotSupported = source.kind === 'nim';
  const slug = modelSlug(model);
  const runningDeployment: Deployment | undefined = deployments.find(
    (d: Deployment) => d.status === 'running' || d.status === 'deployed',
  );
  const apiBase = `${litellmBase.replace(/\/$/, '')}/v1`;
  const curlExample = [
    `curl ${apiBase}/chat/completions \\`,
    `  -H "Authorization: Bearer <YOUR_TAAS_TOKEN>" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '${JSON.stringify({
      model: slug,
      messages: [{ role: 'user', content: 'Hello!' }],
    })}'`,
  ].join('\n');
  const pythonExample = `from openai import OpenAI

client = OpenAI(
    base_url="${apiBase}",
    api_key="<YOUR_TAAS_TOKEN>",
)

resp = client.chat.completions.create(
    model="${slug}",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)`;

  // Show deploy form
  if (showDeployForm) {
    return (
      <div>
        <div className="page-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <button className="btn btn-sm" onClick={() => setShowDeployForm(false)}>← Back</button>
            <h1 className="page-title" style={{ margin: 0 }}>Deploy {model.name}</h1>
          </div>
        </div>
        <DeployForm
          modelName={model.name}
          modelSlug={(model as any).slug ?? model.name.toLowerCase().replace(/\s+/g, '-')}
          onSubmit={(config) => deployMutation.mutate(config)}
          onCancel={() => setShowDeployForm(false)}
          isSubmitting={deployMutation.isPending}
        />
      </div>
    );
  }

  return (
    <div>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Link to="/models" style={{ color: 'var(--text-secondary)', textDecoration: 'none', fontSize: 14 }}>
            ← Models
          </Link>
          <h1 className="page-title" style={{ margin: 0 }}>{model.name}</h1>
          {statusBadge(model.status)}
        </div>
        <div className="actions-row">
          <button
            className="btn btn-primary"
            onClick={() => {
              if (nimNotSupported) {
                addToast('NIM source deployment is not wired yet. Please use HuggingFace or Custom URI model for now.', 'error');
                return;
              }
              setShowDeployForm(true);
            }}
          >
            🚀 New Deployment
          </button>
        </div>
      </div>

      {/* Model Info */}
      <div className="card" style={{ padding: 24, marginBottom: 24 }}>
        <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Model Information</h2>
        <div style={{ display: 'grid', gridTemplateColumns: '140px 1fr', gap: '12px 16px', fontSize: 14 }}>
          <span className="text-muted">Name</span>
          <span style={{ fontWeight: 500 }}>{model.name}</span>
          <span className="text-muted">Description</span>
          <span>{model.description || '—'}</span>
          <span className="text-muted">Framework</span>
          <span>{model.framework}</span>
          <span className="text-muted">Source</span>
          <span>{source.label}</span>
          <span className="text-muted">Source Value</span>
          <span style={{ wordBreak: 'break-all' }}>{source.value}</span>
          <span className="text-muted">Parameters</span>
          <span>{formatParams(model.parameter_count)}</span>
          <span className="text-muted">Context Length</span>
          <span>{model.context_length.toLocaleString()}</span>
          <span className="text-muted">Status</span>
          <span>{statusBadge(model.status)}</span>
          <span className="text-muted">Public</span>
          <span>{model.is_public ? 'Yes' : 'No'}</span>
          <span className="text-muted">Created</span>
          <span>{new Date(model.created_at).toLocaleDateString()}</span>
        </div>
      </div>

      {/* How to call */}
      {runningDeployment ? (
        <div className="card" style={{ padding: 24, marginBottom: 24 }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 4 }}>How to call this model</h2>
          <p className="text-muted" style={{ fontSize: 13, marginBottom: 16 }}>
            Always go through the LiteLLM gateway — it handles auth, quotas, rate-limits and usage metering.
            Direct access to the internal Dynamo endpoint bypasses all of that and should only be used for debugging.
          </p>

          <div style={{ display: 'grid', gridTemplateColumns: '140px 1fr', gap: '12px 16px', fontSize: 14, marginBottom: 20 }}>
            <span className="text-muted">LiteLLM Base</span>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <input
                className="form-input"
                style={{ flex: 1, minWidth: 280, fontFamily: 'var(--font-mono)', fontSize: 13 }}
                value={litellmBase}
                onChange={(e) => saveLitellmBase(e.target.value)}
                placeholder="http://127.0.0.1:4001"
              />
              <span className="text-muted" style={{ fontSize: 12 }}>
                saved per browser
              </span>
            </div>
            <span className="text-muted">API Base (full)</span>
            <span className="text-mono" style={{ wordBreak: 'break-all' }}>{apiBase}</span>
            <span className="text-muted">Model</span>
            <span className="text-mono">{slug}</span>
            <span className="text-muted">Auth</span>
            <span>Bearer token from <Link to="/tokens" style={{ color: 'var(--primary)' }}>API Tokens</Link></span>
          </div>

          <details style={{ marginBottom: 16, fontSize: 12 }}>
            <summary className="text-muted" style={{ cursor: 'pointer' }}>How to choose the right base URL?</summary>
            <ul style={{ marginTop: 8, paddingLeft: 18, lineHeight: 1.6 }}>
              <li>
                <b>Curl from the same dev box that runs <code>kubectl port-forward</code></b>:
                use <code>http://127.0.0.1:14000</code>.
              </li>
              <li>
                <b>From your Mac</b> (after <code>ssh -L 4001:127.0.0.1:14000 root@&lt;dev-box&gt;</code>):
                use <code>http://127.0.0.1:4001</code>.
              </li>
              <li>
                <b>From inside the cluster</b>:
                use <code>http://taas-local-litellm.taas-local.svc.cluster.local:4000</code>.
              </li>
            </ul>
          </details>

          <div style={{ marginBottom: 16 }}>
            <div className="text-muted" style={{ fontSize: 12, marginBottom: 6, textTransform: 'uppercase', letterSpacing: 0.5 }}>curl</div>
            <Copyable text={curlExample} multiline />
          </div>

          <div>
            <div className="text-muted" style={{ fontSize: 12, marginBottom: 6, textTransform: 'uppercase', letterSpacing: 0.5 }}>Python (OpenAI SDK)</div>
            <Copyable text={pythonExample} multiline />
          </div>
        </div>
      ) : null}

      {/* Deployments Table */}
      <div className="card">
        <div style={{ padding: '16px 24px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>Deployments</h2>
          <button
            className="btn btn-primary btn-sm"
            onClick={() => {
              if (nimNotSupported) {
                addToast('NIM source deployment is not wired yet. Please use HuggingFace or Custom URI model for now.', 'error');
                return;
              }
              setShowDeployForm(true);
            }}
          >
            + Deploy
          </button>
        </div>
        <div className="table-wrapper">
          {deploymentsLoading ? (
            <div style={{ padding: 24 }}><p className="text-muted">Loading deployments…</p></div>
          ) : deployments.length === 0 ? (
            <div style={{ padding: 24, textAlign: 'center' }}>
              <p className="text-muted" style={{ marginBottom: 12 }}>No deployments yet.</p>
              <button
                className="btn btn-primary btn-sm"
                onClick={() => {
                  if (nimNotSupported) {
                    addToast('NIM source deployment is not wired yet. Please use HuggingFace or Custom URI model for now.', 'error');
                    return;
                  }
                  setShowDeployForm(true);
                }}
              >
                Create First Deployment
              </button>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Mode</th>
                  <th>Backend</th>
                  <th>Status</th>
                  <th>GPU</th>
                  <th>Topology</th>
                  <th>Internal Endpoint</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {deployments.map((dep: any) => (
                  <tr key={dep.id}>
                    <td style={{ fontWeight: 500 }}>{dep.name || dep.id.slice(0, 8)}</td>
                    <td>{modeBadge(dep.deploy_mode || 'dgd')}</td>
                    <td><span className="badge badge-neutral">{dep.backend || 'vllm'}</span></td>
                    <td>{statusBadge(dep.status)}</td>
                    <td style={{ fontSize: 13 }}>
                      {dep.gpu_type || '—'}
                      {dep.gpu_count_per_replica > 0 && ` ×${dep.gpu_count_per_replica}`}
                    </td>
                    <td style={{ fontSize: 13 }}>
                      {dep.disagg_enabled ? (
                        <span title="Disaggregated: prefill/decode separation">
                          P{dep.prefill_replicas || 1}/D{dep.decode_replicas || 1}
                          {dep.tensor_parallel_size > 1 && ` TP${dep.tensor_parallel_size}`}
                        </span>
                      ) : (
                        <span>
                          ×{dep.replicas_current || dep.replicas_min || 1}
                          {dep.tensor_parallel_size > 1 && ` TP${dep.tensor_parallel_size}`}
                        </span>
                      )}
                    </td>
                    <td>
                      {dep.endpoint_url ? (
                        <span
                          className="badge badge-neutral"
                          style={{ fontSize: 11, cursor: 'help' }}
                          title={`${dep.endpoint_url}\n\nThis is the in-cluster Dynamo Frontend URL used by LiteLLM and the gateway. Do not call it directly from clients.`}
                        >
                          🔧 internal — see "How to call" above
                        </span>
                      ) : dep.error_message ? (
                        <span className="text-danger" style={{ fontSize: 12 }} title={dep.error_message}>
                          ⚠ {dep.error_message.slice(0, 30)}…
                        </span>
                      ) : (
                        <span className="text-muted">—</span>
                      )}
                    </td>
                    <td style={{ fontSize: 13 }}>{new Date(dep.created_at).toLocaleDateString()}</td>
                    <td>
                      <button
                        type="button"
                        className="btn btn-sm btn-danger"
                        disabled={deleteDeploymentMutation.isPending}
                        title="Delete this deployment and clean up Dynamo resources"
                        onClick={() => {
                          const label = dep.name || dep.id.slice(0, 8);
                          if (!window.confirm(`Delete deployment "${label}"? This will remove the Dynamo deployment resources.`)) return;
                          deleteDeploymentMutation.mutate(dep.id);
                        }}
                      >
                        {deletingDeploymentId === dep.id ? 'Deleting…' : 'Delete'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
