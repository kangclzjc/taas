import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { auth, APIError } from '../api/client';

export default function LoginPage() {
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const res = await auth.login({ email, password });
      localStorage.setItem('taas_access_token', res.access_token);
      localStorage.setItem('taas_refresh_token', res.refresh_token);
      navigate('/dashboard');
    } catch (err) {
      if (err instanceof APIError) {
        setError(err.message);
      } else {
        setError('An unexpected error occurred. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-wrapper">
      <div className="login-card">
        <h1 className="login-title">Welcome to TaaS</h1>
        <p className="login-subtitle">Sign in to manage your tokens and models</p>

        {error && <div className="login-error">{error}</div>}

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label className="form-label">Email</label>
            <input
              className="form-input"
              type="email"
              placeholder="you@company.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
            />
          </div>
          <div className="form-group">
            <label className="form-label">Password</label>
            <input
              className="form-input"
              type="password"
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            style={{ width: '100%', justifyContent: 'center', padding: '10px 16px', marginTop: 8 }}
            disabled={loading}
          >
            {loading ? 'Signing in…' : 'Sign In'}
          </button>
        </form>
        <p className="text-sm text-muted" style={{ marginTop: 16, textAlign: 'center' }}>
          No account yet?{' '}
          <Link to="/register" style={{ color: 'var(--primary)' }}>
            Create one
          </Link>
        </p>
      </div>
    </div>
  );
}
