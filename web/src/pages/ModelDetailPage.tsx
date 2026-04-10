import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { models, type DeploymentConfig } from '../api/client';
import { useToast } from '../components/Toast';
import DeployForm from '../components/DeployForm';

function statusBadge(status: string) {
  const map: Record<string, string> = {
    deployed: 'badge-success', deploying: 'badge-warning', undeployed: 'badge-neutral',
    failed: 'badge-danger', running: 'badge-success', stopped: 'badge-neutral',
    pending: 'badge-warning',
  };
  return <span className={`badge ${map[status] ?? 'badge-neutral'}`}>{status}</span>;
}

function modeBadge(mode: string) {
  if (mode === 'dgd') return <span className="badge badge-info" title="Direct deploy, no profiling">⚡ DGD</span>;
  return <span className="badge badge-warning" title="Auto-profiling, SLA-driven">🔬 DGDR</span>;
}

function formatParams(n: number) {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(0)}M`;
  return n.toLocaleString();
}

export default function ModelDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [showDeployForm, setShowDeployForm] = useState(false);

  const { data: model, isLoading: modelLoading } = useQuery({
    queryKey: ['model', id],
    queryFn: () => models.get(id!),
    enabled: !!id,
  });

  const { data: deploymentsData, isLoading: deploymentsLoading } = useQuery({
    queryKey: ['model-deployments', id],
    queryFn: () => models.getDeployments(id!),
    enabled: !!id,
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

  const deployments = deploymentsData?.deployments ?? [];

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
          <button className="btn btn-primary" onClick={() => setShowDeployForm(true)}>
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

      {/* Deployments Table */}
      <div className="card">
        <div style={{ padding: '16px 24px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>Deployments</h2>
          <button className="btn btn-primary btn-sm" onClick={() => setShowDeployForm(true)}>
            + Deploy
          </button>
        </div>
        <div className="table-wrapper">
          {deploymentsLoading ? (
            <div style={{ padding: 24 }}><p className="text-muted">Loading deployments…</p></div>
          ) : deployments.length === 0 ? (
            <div style={{ padding: 24, textAlign: 'center' }}>
              <p className="text-muted" style={{ marginBottom: 12 }}>No deployments yet.</p>
              <button className="btn btn-primary btn-sm" onClick={() => setShowDeployForm(true)}>
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
                  <th>Endpoint</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {deployments.map((dep: any) => (
                  <tr key={dep.id}>
                    <td style={{ fontWeight: 500 }}>{dep.name || dep.id.slice(0, 8)}</td>
                    <td>{modeBadge(dep.deploy_mode || 'dgdr')}</td>
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
                        <code style={{ fontSize: 11 }}>{dep.endpoint_url}</code>
                      ) : dep.error_message ? (
                        <span className="text-danger" style={{ fontSize: 12 }} title={dep.error_message}>
                          ⚠ {dep.error_message.slice(0, 30)}…
                        </span>
                      ) : (
                        <span className="text-muted">—</span>
                      )}
                    </td>
                    <td style={{ fontSize: 13 }}>{new Date(dep.created_at).toLocaleDateString()}</td>
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
