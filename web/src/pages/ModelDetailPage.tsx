import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { models, type DeploymentConfig } from '../api/client';
import { useToast } from '../components/Toast';

const defaultDeployConfig: DeploymentConfig = {
  replicas_min: 1,
  replicas_max: 3,
  gpu_type: 'a100',
  sla_tier: 'standard',
  max_batch_size: 32,
  max_sequence_length: 2048,
};

function statusBadge(status: string) {
  const map: Record<string, string> = {
    deployed: 'badge-success',
    deploying: 'badge-warning',
    undeployed: 'badge-neutral',
    failed: 'badge-danger',
    running: 'badge-success',
    stopped: 'badge-neutral',
    pending: 'badge-warning',
  };
  return <span className={`badge ${map[status] ?? 'badge-neutral'}`}>{status}</span>;
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
    mutationFn: () => models.deploy(id!, defaultDeployConfig),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model', id] });
      queryClient.invalidateQueries({ queryKey: ['model-deployments', id] });
      addToast('Model deployment started successfully', 'success');
      setShowDeployForm(false);
    },
    onError: () => {
      addToast('Failed to deploy model', 'error');
    },
  });

  const undeployMutation = useMutation({
    mutationFn: () => models.undeploy(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model', id] });
      queryClient.invalidateQueries({ queryKey: ['model-deployments', id] });
      addToast('Model undeployed successfully', 'success');
    },
    onError: () => {
      addToast('Failed to undeploy model', 'error');
    },
  });

  if (modelLoading) {
    return (
      <div>
        <div className="page-header">
          <h1 className="page-title">Model Details</h1>
        </div>
        <div className="card" style={{ padding: 24 }}>
          <p className="text-muted">Loading…</p>
        </div>
      </div>
    );
  }

  if (!model) {
    return (
      <div>
        <div className="page-header">
          <h1 className="page-title">Model Not Found</h1>
        </div>
        <div className="card" style={{ padding: 24 }}>
          <p>The requested model could not be found.</p>
          <Link to="/models" className="btn btn-primary" style={{ marginTop: 12 }}>
            ← Back to Models
          </Link>
        </div>
      </div>
    );
  }

  const deployments = deploymentsData?.deployments ?? [];

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
          {(model.status === 'undeployed' || model.status === 'failed') && (
            <button
              className="btn btn-primary"
              onClick={() => deployMutation.mutate()}
              disabled={deployMutation.isPending}
            >
              {deployMutation.isPending ? 'Deploying…' : '🚀 Deploy'}
            </button>
          )}
          {model.status === 'deployed' && (
            <button
              className="btn btn-danger"
              onClick={() => undeployMutation.mutate()}
              disabled={undeployMutation.isPending}
            >
              {undeployMutation.isPending ? 'Stopping…' : '⏹ Undeploy'}
            </button>
          )}
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
          <span className="text-muted">Format</span>
          <span className="text-mono">{model.format}</span>
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

      {/* Deployments */}
      <div className="card">
        <div style={{ padding: '16px 24px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>Deployments</h2>
        </div>
        <div className="table-wrapper">
          {deploymentsLoading ? (
            <div style={{ padding: 24 }}>
              <p className="text-muted">Loading deployments…</p>
            </div>
          ) : deployments.length === 0 ? (
            <div style={{ padding: 24, textAlign: 'center' }}>
              <p className="text-muted" style={{ marginBottom: 12 }}>No deployments for this model.</p>
              {(model.status === 'undeployed' || model.status === 'failed') && (
                <button
                  className="btn btn-primary btn-sm"
                  onClick={() => deployMutation.mutate()}
                  disabled={deployMutation.isPending}
                >
                  Deploy Now
                </button>
              )}
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Status</th>
                  <th>Replicas</th>
                  <th>GPU Type</th>
                  <th>Endpoint URL</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {deployments.map((dep) => (
                  <tr key={dep.id}>
                    <td className="text-mono" style={{ fontSize: 13 }}>{dep.id.slice(0, 8)}…</td>
                    <td>{statusBadge(dep.status)}</td>
                    <td>{dep.replicas}</td>
                    <td>{dep.gpu_type}</td>
                    <td>
                      {dep.endpoint_url ? (
                        <code style={{ fontSize: 12 }}>{dep.endpoint_url}</code>
                      ) : (
                        <span className="text-muted">—</span>
                      )}
                    </td>
                    <td>{new Date(dep.created_at).toLocaleDateString()}</td>
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
