import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { auth } from '../api/client';

const navItems = [
  { to: '/dashboard', icon: '📊', label: 'Dashboard' },
  { to: '/models', icon: '🤖', label: 'Models' },
  { to: '/tokens', icon: '🔑', label: 'API Tokens' },
  { to: '/usage', icon: '📈', label: 'Usage' },
];

export default function Layout() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: user } = useQuery({
    queryKey: ['me'],
    queryFn: auth.me,
    retry: false,
  });

  const handleLogout = () => {
    auth.logout().catch(() => {});
    localStorage.removeItem('taas_access_token');
    localStorage.removeItem('taas_refresh_token');
    queryClient.clear();
    navigate('/login');
  };

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span>T</span>aaS
        </div>
        <nav className="sidebar-nav">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`}
            >
              <span className="icon">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-footer">
          <button className="btn btn-ghost" style={{ width: '100%' }} onClick={handleLogout}>
            🚪 Logout
          </button>
        </div>
      </aside>
      <div className="main-area">
        <header className="header">
          <div className="header-title">Token as a Service</div>
          <div className="header-user">
            {user && (
              <>
                <span className="header-user-name">{user.name || user.email}</span>
                <span className="badge badge-info">{user.role}</span>
              </>
            )}
          </div>
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
