import { useState } from 'react';

interface TokenModalProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: { name: string; scopes: string[]; rate_limit_rpm?: number; expires_at?: string }) => void;
  createdToken?: string | null;
  isLoading?: boolean;
}

export default function TokenModal({ open, onClose, onSubmit, createdToken, isLoading }: TokenModalProps) {
  const [name, setName] = useState('');
  const [scopes, setScopes] = useState('inference');
  const [rateLimit, setRateLimit] = useState('60');
  const [expiresAt, setExpiresAt] = useState('');
  const [copied, setCopied] = useState(false);

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit({
      name: name.trim(),
      scopes: scopes.split(',').map((s) => s.trim()).filter(Boolean),
      rate_limit_rpm: rateLimit ? Number(rateLimit) : undefined,
      expires_at: expiresAt || undefined,
    });
  };

  const handleCopy = () => {
    if (createdToken) {
      navigator.clipboard.writeText(createdToken);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleClose = () => {
    setName('');
    setScopes('inference');
    setRateLimit('60');
    setExpiresAt('');
    setCopied(false);
    onClose();
  };

  return (
    <div className="modal-overlay" onClick={handleClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3 className="modal-title">{createdToken ? 'Token Created' : 'Create API Token'}</h3>
          <button className="modal-close" onClick={handleClose}>
            ×
          </button>
        </div>

        {createdToken ? (
          <div className="modal-body">
            <p className="text-sm text-muted mb-16">
              Your new API token has been created. Copy it now — you won't be able to see it again.
            </p>
            <div className="token-box">
              <code>{createdToken}</code>
              <button className="token-copy-btn" onClick={handleCopy}>
                {copied ? '✓ Copied' : 'Copy'}
              </button>
            </div>
            <div className="token-warning">
              ⚠️ Store this token securely. It will not be shown again.
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="modal-body">
              <div className="form-group">
                <label className="form-label">Token Name</label>
                <input
                  className="form-input"
                  placeholder="e.g. production-backend"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">Scopes (comma-separated)</label>
                <input
                  className="form-input"
                  placeholder="inference, models:read"
                  value={scopes}
                  onChange={(e) => setScopes(e.target.value)}
                />
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label className="form-label">Rate Limit (RPM)</label>
                  <input
                    className="form-input"
                    type="number"
                    min="1"
                    value={rateLimit}
                    onChange={(e) => setRateLimit(e.target.value)}
                  />
                </div>
                <div className="form-group">
                  <label className="form-label">Expires At (optional)</label>
                  <input
                    className="form-input"
                    type="date"
                    value={expiresAt}
                    onChange={(e) => setExpiresAt(e.target.value)}
                  />
                </div>
              </div>
            </div>
            <div className="modal-footer">
              <button type="button" className="btn btn-secondary" onClick={handleClose}>
                Cancel
              </button>
              <button type="submit" className="btn btn-primary" disabled={!name.trim() || isLoading}>
                {isLoading ? 'Creating…' : 'Create Token'}
              </button>
            </div>
          </form>
        )}

        {createdToken && (
          <div className="modal-footer">
            <button className="btn btn-primary" onClick={handleClose}>
              Done
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
