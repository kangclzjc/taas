import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { tokens } from '../api/client';
import TokenModal from '../components/TokenModal';
import { SkeletonTable } from '../components/LoadingSkeleton';
import EmptyState from '../components/EmptyState';
import { useToast } from '../components/Toast';

export default function TokensPage() {
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [modalOpen, setModalOpen] = useState(false);
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['tokens'],
    queryFn: () => tokens.list(),
  });

  const createMutation = useMutation({
    mutationFn: tokens.create,
    onSuccess: (res) => {
      setCreatedToken(res.token_value);
      queryClient.invalidateQueries({ queryKey: ['tokens'] });
      addToast('Token created successfully', 'success');
    },
    onError: () => {
      addToast('Failed to create token', 'error');
    },
  });

  const revokeMutation = useMutation({
    mutationFn: tokens.revoke,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tokens'] });
      addToast('Token revoked successfully', 'success');
    },
    onError: () => {
      addToast('Failed to revoke token', 'error');
    },
  });

  const rotateMutation = useMutation({
    mutationFn: tokens.rotate,
    onSuccess: (res) => {
      setCreatedToken(res.token_value);
      setModalOpen(true);
      queryClient.invalidateQueries({ queryKey: ['tokens'] });
      addToast('Token rotated successfully', 'success');
    },
    onError: () => {
      addToast('Failed to rotate token', 'error');
    },
  });

  const handleCreate = (formData: { name: string; scopes: string[]; rate_limit_rpm?: number; expires_at?: string }) => {
    createMutation.mutate(formData);
  };

  const handleCloseModal = () => {
    setModalOpen(false);
    setCreatedToken(null);
    createMutation.reset();
  };

  const formatDate = (d: string | null) => {
    if (!d) return '—';
    return new Date(d).toLocaleDateString();
  };

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">API Tokens</h1>
        <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
          + Create Token
        </button>
      </div>

      <div className="card">
        <div className="table-wrapper">
          {isLoading ? (
            <SkeletonTable rows={4} cols={8} />
          ) : !data?.items?.length ? (
            <EmptyState
              icon="🔑"
              title="No tokens yet"
              description="Create an API token to start making requests."
              action={
                <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
                  + Create Token
                </button>
              }
            />
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Prefix</th>
                  <th>Scopes</th>
                  <th>Rate Limit</th>
                  <th>Status</th>
                  <th>Expires</th>
                  <th>Last Used</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((token) => (
                  <tr key={token.id}>
                    <td style={{ fontWeight: 500 }}>{token.name}</td>
                    <td><code className="text-mono">{token.token_prefix}…</code></td>
                    <td>
                      {token.scopes.map((s) => (
                        <span key={s} className="badge badge-info" style={{ marginRight: 4 }}>
                          {s}
                        </span>
                      ))}
                    </td>
                    <td>{token.rate_limit_rpm} RPM</td>
                    <td>
                      <span className={`badge ${token.is_active ? 'badge-success' : 'badge-danger'}`}>
                        {token.is_active ? 'Active' : 'Revoked'}
                      </span>
                    </td>
                    <td>{formatDate(token.expires_at)}</td>
                    <td>{formatDate(token.last_used_at)}</td>
                    <td>
                      <div className="actions-row">
                        {token.is_active && (
                          <>
                            <button
                              className="btn btn-secondary btn-sm"
                              onClick={() => rotateMutation.mutate(token.id)}
                              disabled={rotateMutation.isPending}
                            >
                              Rotate
                            </button>
                            <button
                              className="btn btn-danger btn-sm"
                              onClick={() => {
                                if (confirm(`Revoke token "${token.name}"?`)) {
                                  revokeMutation.mutate(token.id);
                                }
                              }}
                              disabled={revokeMutation.isPending}
                            >
                              Revoke
                            </button>
                          </>
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

      <TokenModal
        open={modalOpen}
        onClose={handleCloseModal}
        onSubmit={handleCreate}
        createdToken={createdToken}
        isLoading={createMutation.isPending}
      />
    </div>
  );
}
