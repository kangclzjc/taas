import { NavLink, Outlet } from "react-router-dom";

const LITELLM_URL =
  import.meta.env.VITE_LITELLM_URL ?? "http://localhost:4000/ui";

const navItems = [
  { to: "/dashboard", icon: "📊", label: "Dashboard" },
  { to: "/models", icon: "🧠", label: "Models & Deploy" },
  { to: "/tokens", icon: "🔑", label: "API Keys" },
  { to: "/usage", icon: "📈", label: "Usage" },
  { to: "/reports", icon: "📑", label: "Reports" },
  { to: "/organizations", icon: "🏢", label: "Organizations" },
];

export default function Layout() {
  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span style={{ color: "var(--accent)" }}>⬡</span> TaaS
        </div>
        <nav className="sidebar-nav">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `sidebar-link${isActive ? " active" : ""}`
              }
            >
              <span className="icon">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
          <a
            href={LITELLM_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="sidebar-link"
            style={{
              marginTop: 8,
              borderTop: "1px solid rgba(255,255,255,0.08)",
              paddingTop: 12,
            }}
          >
            <span className="icon">🌐</span>
            API Gateway
            <span style={{ fontSize: 10, opacity: 0.5, marginLeft: "auto" }}>
              ↗
            </span>
          </a>
        </nav>
        <div className="sidebar-footer">
          <span
            className="badge badge-info"
            style={{ width: "100%", justifyContent: "center" }}
          >
            Demo Mode
          </span>
        </div>
      </aside>
      <div className="main-area">
        <header className="header">
          <div className="header-title">
            <span style={{ fontWeight: 700 }}>Token as a Service</span>
            <span
              style={{
                fontSize: 11,
                color: "var(--text-muted)",
                marginLeft: 8,
                fontWeight: 400,
              }}
            >
              Powered by NVIDIA Dynamo
            </span>
          </div>
          <div className="header-user">
            <span className="header-user-name">Demo workspace</span>
            <span className="badge badge-info">auth bypass</span>
          </div>
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
