import { useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { auth, type APIError } from '../api/client';
import { useToast } from '../components/Toast';

export default function ProfilePage() {
  const { addToast } = useToast();

  const { data: user, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: auth.me,
  });

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  const changePasswordMutation = useMutation({
    mutationFn: (data: { current_password: string; new_password: string }) =>
      auth.changePassword(data),
    onSuccess: () => {
      addToast('Password changed successfully', 'success');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    },
    onError: (err: APIError) => {
      addToast(err.message || 'Failed to change password', 'error');
    },
  });

  const handleChangePassword = (e: React.FormEvent) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      addToast('New passwords do not match', 'error');
      return;
    }
    if (newPassword.length < 8) {
      addToast('New password must be at least 8 characters', 'error');
      return;
    }
    changePasswordMutation.mutate({
      current_password: currentPassword,
      new_password: newPassword,
    });
  };

  if (isLoading) {
    return (
      <div>
        <div className="page-header">
          <h1 className="page-title">Profile</h1>
        </div>
        <div className="card" style={{ padding: 24 }}>
          <p className="text-muted">Loading…</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Profile</h1>
      </div>

      {/* User Info */}
      <div className="card" style={{ padding: 24, marginBottom: 24 }}>
        <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Account Information</h2>
        <div style={{ display: 'grid', gridTemplateColumns: '120px 1fr', gap: '12px 16px', fontSize: 14 }}>
          <span className="text-muted">Email</span>
          <span>{user?.email}</span>
          <span className="text-muted">Name</span>
          <span>{user?.name || '—'}</span>
          <span className="text-muted">Role</span>
          <span><span className="badge badge-info">{user?.role}</span></span>
          <span className="text-muted">Organization ID</span>
          <span className="text-mono" style={{ fontSize: 13 }}>{user?.org_id}</span>
        </div>
      </div>

      {/* Change Password */}
      <div className="card" style={{ padding: 24 }}>
        <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Change Password</h2>
        <form onSubmit={handleChangePassword} style={{ maxWidth: 400 }}>
          <div className="form-group" style={{ marginBottom: 16 }}>
            <label className="form-label" htmlFor="current-password">Current Password</label>
            <input
              id="current-password"
              type="password"
              className="form-input"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
          </div>
          <div className="form-group" style={{ marginBottom: 16 }}>
            <label className="form-label" htmlFor="new-password">New Password</label>
            <input
              id="new-password"
              type="password"
              className="form-input"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              minLength={8}
              autoComplete="new-password"
            />
          </div>
          <div className="form-group" style={{ marginBottom: 20 }}>
            <label className="form-label" htmlFor="confirm-password">Confirm New Password</label>
            <input
              id="confirm-password"
              type="password"
              className="form-input"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              minLength={8}
              autoComplete="new-password"
            />
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={changePasswordMutation.isPending}
          >
            {changePasswordMutation.isPending ? 'Changing…' : 'Change Password'}
          </button>
        </form>
      </div>
    </div>
  );
}
