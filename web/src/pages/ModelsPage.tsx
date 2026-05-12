import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { models } from '../api/client';
import { SkeletonTable } from '../components/LoadingSkeleton';
import EmptyState from '../components/EmptyState';
import { useToast } from '../components/Toast';

function statusBadge(status: string) {
  const map: Record<string, string> = {
    ready: 'badge-success',
    running: 'badge-success',
    deployed: 'badge-success',
    deploying: 'badge-warning',
    pending: 'badge-warning',
    undeployed: 'badge-neutral',
    uploading: 'badge-neutral',
    validating: 'badge-neutral',
    failed: 'badge-danger',
  };
  return <span className={`badge ${map[status] ?? 'badge-neutral'}`}>{status}</span>;
}

export default function ModelsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [showPublic, setShowPublic] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [sourceType, setSourceType] = useState<'huggingface' | 'nim' | 'custom'>('huggingface');
  const [createForm, setCreateForm] = useState({
    name: '',
    slug: '',
    description: '',
    sourceValue: '',
  });

  const buildStorageURI = () => {
    const value = createForm.sourceValue.trim();
    if (!value) return '';
    if (sourceType === 'nim') {
      return value.startsWith('nim://') ? value : `nim://${value}`;
    }
    return value;
  };

  const { data, isLoading } = useQuery({
    queryKey: ['models', showPublic],
    queryFn: () => models.list(showPublic ? { public: true } : undefined),
  });

  const createMutation = useMutation({
    mutationFn: () => models.create({
      name: createForm.name,
      slug: createForm.slug,
      description: createForm.description,
      storage_uri: buildStorageURI(),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['models'] });
      addToast('Model created', 'success');
      setShowCreate(false);
      setSourceType('huggingface');
      setCreateForm({ name: '', slug: '', description: '', sourceValue: '' });
    },
    onError: (err: any) => {
      addToast(`Failed to create model: ${err.message ?? 'unknown error'}`, 'error');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => models.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['models'] });
      addToast('Model deleted', 'success');
    },
    onError: (err: any) => {
      addToast(`Failed to delete model: ${err.message ?? 'unknown error'}`, 'error');
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
          <button className="btn btn-primary btn-sm" onClick={() => setShowCreate((v) => !v)}>
            {showCreate ? 'Cancel' : '+ Create model'}
          </button>
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

      {showCreate && (
        <div className="card" style={{ padding: 16, marginBottom: 16 }}>
          <h3 style={{ marginBottom: 12 }}>Create model</h3>
          <div style={{ display: 'grid', gridTemplateColumns: '2fr 2fr', gap: 8, marginBottom: 8 }}>
            <input
              className="form-input"
              placeholder="Model name"
              value={createForm.name}
              onChange={(e) => setCreateForm((p) => ({ ...p, name: e.target.value }))}
            />
            <input
              className="form-input"
              placeholder="Slug (lowercase-hyphen)"
              value={createForm.slug}
              onChange={(e) => setCreateForm((p) => ({ ...p, slug: e.target.value }))}
            />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: 8, marginBottom: 8 }}>
            <select
              className="form-input"
              value={sourceType}
              onChange={(e) => setSourceType(e.target.value as 'huggingface' | 'nim' | 'custom')}
            >
              <option value="huggingface">HuggingFace</option>
              <option value="nim">NIM</option>
              <option value="custom">Custom URI</option>
            </select>
            <input
              className="form-input"
              placeholder={
                sourceType === 'huggingface'
                  ? 'HF model id, e.g. Qwen/Qwen3-8B'
                  : sourceType === 'nim'
                    ? 'NIM model ref, e.g. meta/llama-3.1-8b-instruct'
                    : 'Storage URI, e.g. s3://bucket/model or gs://...'
              }
              value={createForm.sourceValue}
              onChange={(e) => setCreateForm((p) => ({ ...p, sourceValue: e.target.value }))}
            />
          </div>
          <textarea
            className="form-input"
            placeholder="Description (optional)"
            rows={2}
            value={createForm.description}
            onChange={(e) => setCreateForm((p) => ({ ...p, description: e.target.value }))}
            style={{ marginBottom: 8 }}
          />
          <button
            className="btn btn-primary btn-sm"
            disabled={createMutation.isPending || !createForm.name || !createForm.slug || !createForm.sourceValue.trim()}
            onClick={() => createMutation.mutate()}
          >
            {createMutation.isPending ? 'Creating…' : 'Create'}
          </button>
        </div>
      )}

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
                      <div style={{ fontWeight: 500 }}>
                        <Link to={`/models/${model.id}`} style={{ color: 'var(--primary)', textDecoration: 'none' }}>
                          {model.name}
                        </Link>
                      </div>
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
                        {['ready', 'undeployed', 'failed'].includes(model.status) ? (
                          <button
                            className="btn btn-primary btn-sm"
                            onClick={() => navigate(`/models/${model.id}`)}
                          >
                            Deploy…
                          </button>
                        ) : ['running', 'deployed'].includes(model.status) ? (
                          <button
                            className="btn btn-sm"
                            onClick={() => navigate(`/models/${model.id}`)}
                          >
                            View
                          </button>
                        ) : (
                          <span className="text-sm text-muted">Preparing…</span>
                        )}
                        <button
                          className="btn btn-danger btn-sm"
                          disabled={deleteMutation.isPending}
                          onClick={() => {
                            if (!window.confirm(`Delete model "${model.name}"? This cannot be undone.`)) return;
                            deleteMutation.mutate(model.id);
                          }}
                        >
                          Delete
                        </button>
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
