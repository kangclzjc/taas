import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import Layout from "./components/Layout";
import ErrorBoundary from "./components/ErrorBoundary";
import { ToastProvider } from "./components/Toast";
import DashboardPage from "./pages/DashboardPage";
import ModelsPage from "./pages/ModelsPage";
import ModelDetailPage from "./pages/ModelDetailPage";
import TokensPage from "./pages/TokensPage";
import UsagePage from "./pages/UsagePage";
import ReportsPage from "./pages/ReportsPage";
import ProfilePage from "./pages/ProfilePage";
import OrganizationsPage from "./pages/OrganizationsPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

export default function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <BrowserRouter>
            <Routes>
              <Route
                path="/login"
                element={<Navigate to="/reports" replace />}
              />
              <Route
                path="/register"
                element={<Navigate to="/reports" replace />}
              />
              <Route element={<Layout />}>
                <Route path="/" element={<Navigate to="/reports" replace />} />
                <Route path="/dashboard" element={<DashboardPage />} />
                <Route path="/models" element={<ModelsPage />} />
                <Route path="/models/:id" element={<ModelDetailPage />} />
                <Route path="/tokens" element={<TokensPage />} />
                <Route path="/usage" element={<UsagePage />} />
                <Route path="/reports" element={<ReportsPage />} />
                <Route path="/organizations" element={<OrganizationsPage />} />
                <Route path="/profile" element={<ProfilePage />} />
              </Route>
              <Route path="*" element={<Navigate to="/reports" replace />} />
            </Routes>
          </BrowserRouter>
        </ToastProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
