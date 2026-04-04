import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { models, type DeploymentConfig } from '../api/client';
import { SkeletonTable } from '../components/LoadingSkeleton';
import EmptyState from '../components/EmptyState';
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
  };
  return <span className={`badge ${map[status] ?? 'badge-neutral'}`}>{status}</span>;
}

export default function ModelsPage() {
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [showPublic, setShowPublic] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['models', showPublic],
    queryFn: () => models.list(showPublic ? { public: true } : undefined),
  });

  const deployMutation = useMutation({
    mutationFn: (id: string) => models.deploy(id, defaultDeployConfig),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['models'] });
      addToast('Model deployment started successfully', 'success');
    },
    onError: () => {
      addToast('Failed to deploy model', 'error');
    },
  });

  const undeployMutation = useMutation({
    mutationFn: (id: string) => models.undeploy(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['models'] });
      addToast('Model undeployed successfully', 'success');
    },
    onError: () => {
      addToast('Failed to undeploy model', 'error');
    },
  });

  const formatParams = (n: number) => {
    if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
    if (n >= 1e6) return `${(n / 1e6).toFixed(0)}M`;
    return n.toLocaleString();
  };

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Models</h1>
        <div className="actions-row">
          <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--text-secondary)' }}>
            <input
              type="checkbox"
              checked={showPublic}
              onChange={(e) => setShowPublic(e.target.checked)}
            />
            Show public models
          </label>
        </div>
      </div>

      <div className="card">
        <div className="table-wrapper">
          {isLoading ? (
            <SkeletonTable rows={5} cols={7} />
          ) : !data?.items?.length ? (
            <EmptyState
              icon="🤖"
              title="No models found"
              description="Upload or register a model to get started with inference."
            />
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Framework</th>
                  <th>Format</th>
                  <th>Parameters</th>
                  <th>Context</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((model) => (
                  <tr key={model.id}>
                    <td>
                      <div style={{ fontWeight: 500 }}>{model.name}</div>
                      {model.description && (
                        <div className="text-sm text-muted truncate" style={{ maxWidth: 240 }}>
                          {model.description}
                        </div>
                      )}
                    </td>
                    <td>{model.framework}</td>
                    <td><span className="text-mono">{model.format}</span></td>
                    <td>{formatParams(model.parameter_count)}</td>
                    <td>{model.context_length.toLocaleString()}</td>
                    <td>{statusBadge(model.status)}</td>
                    <td>
                      <div className="actions-row">
                        {model.status === 'undeployed' || model.status === 'failed' ? (
                          <button
                            className="btn btn-primary btn-sm"
                            onClick={() => deployMutation.mutate(model.id)}
                            disabled={deployMutation.isPending}
                          >
                            Deploy
                          </button>
                        ) : model.status === 'deployed' ? (
                          <button
                            className="btn btn-danger btn-sm"
                            onClick={() => undeployMutation.mutate(model.id)}
                            disabled={undeployMutation.isPending}
                          >
                            Undeploy
                          </button>
                        ) : (
                          <span className="text-sm text-muted">Deploying…</span>
                        )}
                      </div>
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
