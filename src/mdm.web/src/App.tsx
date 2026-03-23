import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useAuthStore } from "./stores/authStore";
import { Layout } from "./components/Layout";
import { ToastContainer } from "./components/ToastContainer";
import { DialogProvider } from "./components/DialogProvider";
import { Login } from "./pages/Login";
import { Setup } from "./pages/Setup";
import { Dashboard } from "./pages/Dashboard";
import { Devices } from "./pages/Devices";
import { DeviceDetail } from "./pages/DeviceDetail";
import { Commands } from "./pages/Commands";
import { Apps } from "./pages/Apps";
import { Profiles } from "./pages/Profiles";
import { Events } from "./pages/Events";
import { Users } from "./pages/Users";
import { Audit } from "./pages/Audit";
import { Rentals } from "./pages/Rentals";
import { Categories } from "./pages/Categories";
import { useState, useEffect } from "react";
import apiClient from "./lib/apiClient";

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuthStore();
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-base-200">
        <span className="loading loading-spinner loading-lg text-primary"></span>
      </div>
    );
  }
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function AppRoutes() {
  const { isAuthenticated, isLoading, checkAuth } = useAuthStore();
  const [initialized, setInitialized] = useState<boolean | null>(null);

  useEffect(() => { checkAuth(); }, [checkAuth]);

  useEffect(() => {
    apiClient.get("/api/system-status")
      .then(({ data }) => setInitialized(data.initialized))
      .catch(() => setInitialized(true));
  }, []);

  if (initialized === null || isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-base-200">
        <span className="loading loading-spinner loading-lg text-primary"></span>
      </div>
    );
  }

  if (!initialized) {
    return (
      <Routes>
        <Route path="/setup" element={<Setup />} />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    );
  }

  return (
    <Routes>
      <Route path="/setup" element={<Navigate to="/login" replace />} />
      <Route path="/login" element={isAuthenticated ? <Navigate to="/dashboard" /> : <Login />} />
      <Route
        element={
          <ProtectedRoute>
            <Layout />
            <ToastContainer />
          </ProtectedRoute>
        }
      >
        <Route path="/dashboard" element={<Dashboard />} />
        <Route path="/devices" element={<Devices />} />
        <Route path="/devices/:udid" element={<DeviceDetail />} />
        <Route path="/commands" element={<Commands />} />
        <Route path="/apps" element={<Apps />} />
        <Route path="/profiles" element={<Profiles />} />
        <Route path="/events" element={<Events />} />
        <Route path="/rentals" element={<Rentals />} />
        <Route path="/categories" element={<Categories />} />
        <Route path="/users" element={<Users />} />
        <Route path="/audit" element={<Audit />} />
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
      </Route>
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <DialogProvider>
        <AppRoutes />
      </DialogProvider>
    </BrowserRouter>
  );
}
