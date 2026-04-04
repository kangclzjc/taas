import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { organizations, type Organization, type OrgMember, APIError } from '../api/client';
import { useToast } from '../components/Toast';
import EmptyState from '../components/EmptyState';
import { SkeletonTable } from '../components/LoadingSkeleton';

function CreateOrgModal({
  open,
  onClose,
  onSubmit,
  isLoading,
}: {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: { slug: string; display_name: string }) => void;
  isLoading: boolean;
}) {
  const [slug, setSlug] = useState('');
  const [displayName, setDisplayName] = useState('');

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit({ slug, display_name: displayName });
  };

  const handleClose = () => {
    setSlug('');
    setDisplayName('');
    onClose();
  };

  return (
    <div className="modal-backdrop" onClick={handleClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2 className="modal-title">Create Organization</h2>
          <button className="modal-close" onClick={handleClose}>×</button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="modal-body">
            <div className="form-group">
              <label className="form-label">Display Name</label>
              <input
                className="form-input"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="My Organization"
                required
              />
            </div>
            <div className="form-group">
              <label className="form-label">Slug</label>
              <input
                className="form-input"
                value={slug}
                onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '-'))}
                placeholder="my-organization"
                required
                pattern="[a-z0-9][a-z0-9-]*[a-z0-9]"
                title="Lowercase letters, numbers, and hyphens only"
              />
              <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                URL-safe identifier (lowercase, hyphens allowed)
              </span>
            </div>
          </div>
          <div className="modal-footer">
            <button type="button" className="btn btn-secondary" onClick={handleClose}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={isLoading || !slug || !displayName}>
              {isLoading ? 'Creating…' : 'Create Organization'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function AddMemberForm({
  orgId,
  onSuccess,
}: {
  orgId: string;
  onSuccess: () => void;
}) {
  const { addToast } = useToast();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('member');

  const addMutation = useMutation({
    mutationFn: (data: { email: string; role: string }) =>
      organizations.addMember(orgId, data),
    onSuccess: () => {
      addToast('Member added successfully', 'success');
      setEmail('');
      setRole('member');
      onSuccess();
    },
    onError: (err) => {
      const msg = err instanceof APIError ? err.message : 'Failed to add member';
      addToast(msg, 'error');
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    addMutation.mutate({ email, role });
  };

  return (
    <form onSubmit={handleSubmit} style={{ display: 'flex', gap: 8, marginTop: 12, alignItems: 'center' }}>
      <input
        className="form-input"
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="user@example.com"
        required
        style={{ flex: 1 }}
      />
      <select
        className="form-input"
        value={role}
        onChange={(e) => setRole(e.target.value)}
        style={{ width: 120 }}
      >
        <option value="member">Member</option>
        <option value="admin">Admin</option>
        <option value="viewer">Viewer</option>
      </select>
      <button type="submit" className="btn btn-primary btn-sm" disabled={addMutation.isPending}>
        {addMutation.isPending ? 'Adding…' : '+ Add'}
      </button>
    </form>
  );
}

function MembersList({ orgId }: { orgId: string }) {
  const queryClient = useQueryClient();
  const { addToast } = useToast();

  const { data, isLoading } = useQuery({
    queryKey: ['org-members', orgId],
    queryFn: () => organizations.listMembers(orgId),
  });

  const removeMutation = useMutation({
    mutationFn: (userId: string) => organizations.removeMember(orgId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['org-members', orgId] });
      queryClient.invalidateQueries({ queryKey: ['organizations'] });
      addToast('Member removed', 'success');
    },
    onError: (err) => {
      const msg = err instanceof APIError ? err.message : 'Failed to remove member';
      addToast(msg, 'error');
    },
  });

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['org-members', orgId] });
    queryClient.invalidateQueries({ queryKey: ['organizations'] });
  };

  if (isLoading) {
    return <div style={{ padding: '8px 0', color: 'var(--text-muted)' }}>Loading members…</div>;
  }

  const members = data?.members ?? [];

  return (
    <div className="org-members">
      <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 8 }}>
        Members ({members.length})
      </div>
      {members.length === 0 ? (
        <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>No members yet</div>
      ) : (
        members.map((m) => (
          <div key={m.user_id} className="member-row">
            <div>
              <span style={{ fontWeight: 500 }}>{m.email || m.user_id}</span>
              <span
                className="badge badge-info"
                style={{ marginLeft: 8, fontSize: 11 }}
              >
                {m.role}
              </span>
            </div>
            <button
              className="btn btn-danger btn-sm"
              style={{ fontSize: 11, padding: '2px 8px' }}
              onClick={() => {
                if (confirm(`Remove ${m.email || m.user_id} from this organization?`)) {
                  removeMutation.mutate(m.user_id);
                }
              }}
              disabled={removeMutation.isPending}
            >
              Remove
            </button>
          </div>
        ))
      )}
      <AddMemberForm orgId={orgId} onSuccess={handleRefresh} />
    </div>
  );
}

function OrgCard({ org }: { org: Organization }) {
  const [expanded, setExpanded] = useState(false);
  const queryClient = useQueryClient();
  const { addToast } = useToast();

  const deleteMutation = useMutation({
    mutationFn: () => organizations.delete(org.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] });
      addToast('Organization deleted', 'success');
    },
    onError: (err) => {
      const msg = err instanceof APIError ? err.message : 'Failed to delete organization';
      addToast(msg, 'error');
    },
  });

  const formatDate = (d: string) => {
    return new Date(d).toLocaleDateString();
  };

  const slaTierColor = (tier: string) => {
    switch (tier) {
      case 'enterprise': return 'badge-success';
      case 'professional': return 'badge-warning';
      default: return 'badge-info';
    }
  };

  return (
    <div className="org-card">
      <div className="org-card-header">
        <div>
          <div className="org-card-name">{org.display_name}</div>
          <div className="org-card-slug">{org.slug}</div>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button
            className="btn btn-secondary btn-sm"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? '▲ Collapse' : '▼ Members'}
          </button>
          <button
            className="btn btn-danger btn-sm"
            onClick={() => {
              if (confirm(`Delete organization "${org.display_name}"? This cannot be undone.`)) {
                deleteMutation.mutate();
              }
            }}
            disabled={deleteMutation.isPending}
          >
            Delete
          </button>
        </div>
      </div>
      <div className="org-card-meta">
        <span className={`badge ${slaTierColor(org.sla_tier)}`}>
          {org.sla_tier || 'standard'}
        </span>
        <span>Created {formatDate(org.created_at)}</span>
      </div>
      {expanded && <MembersList orgId={org.id} />}
    </div>
  );
}

export default function OrganizationsPage() {
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [modalOpen, setModalOpen] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => organizations.list(),
  });

  const createMutation = useMutation({
    mutationFn: organizations.create,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] });
      addToast('Organization created successfully', 'success');
      setModalOpen(false);
    },
    onError: (err) => {
      const msg = err instanceof APIError ? err.message : 'Failed to create organization';
      addToast(msg, 'error');
    },
  });

  const orgs = data?.organizations ?? [];

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Organizations</h1>
        <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
          + Create Organization
        </button>
      </div>

      {isLoading ? (
        <div className="card">
          <SkeletonTable rows={3} cols={4} />
        </div>
      ) : orgs.length === 0 ? (
        <div className="card" style={{ padding: 40 }}>
          <EmptyState
            icon="🏢"
            title="No organizations yet"
            description="Create an organization to manage teams, billing, and access control."
            action={
              <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
                + Create Organization
              </button>
            }
          />
        </div>
      ) : (
        <div className="org-cards">
          {orgs.map((org) => (
            <OrgCard key={org.id} org={org} />
          ))}
        </div>
      )}

      <CreateOrgModal
        open={modalOpen}
        onClose={() => {
          setModalOpen(false);
          createMutation.reset();
        }}
        onSubmit={(data) => createMutation.mutate(data)}
        isLoading={createMutation.isPending}
      />
    </div>
  );
}
