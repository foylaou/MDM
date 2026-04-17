import { create } from "zustand";
import apiClient from "../lib/apiClient";
import { createClients, type Clients } from "../lib/client";

interface UserInfo {
  id: string;
  username: string;
  role: string;
  system_role: string;
  display_name: string;
}

export type ModulePermissions = Record<string, string>;

interface AuthStore {
  user: UserInfo | null;
  modulePermissions: ModulePermissions;
  clients: Clients | null;
  isLoading: boolean;
  isAuthenticated: boolean;

  checkAuth: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  modulePermissions: {},
  clients: null,
  isLoading: true,
  isAuthenticated: false,

  checkAuth: async () => {
    // Try cookie-based auth first
    try {
      const { data } = await apiClient.get("/api/me");
      set({
        user: {
          id: data.id,
          username: data.username,
          role: data.role,
          system_role: data.system_role || "",
          display_name: data.display_name || data.username,
        },
        modulePermissions: data.module_permissions || {},
        clients: createClients(),
        isAuthenticated: true,
        isLoading: false,
      });
      return;
    } catch {
      // Cookie auth failed, not logged in
    }

    set({ user: null, modulePermissions: {}, clients: null, isAuthenticated: false, isLoading: false });
  },

  login: async (username: string, password: string) => {
    // Use REST login (sets HttpOnly cookie)
    const { data } = await apiClient.post("/api/login", { username, password });
    set({
      user: data.user,
      modulePermissions: data.module_permissions || {},
      clients: createClients(),
      isAuthenticated: true,
    });
  },

  logout: async () => {
    try {
      await apiClient.post("/api/logout");
    } catch { /* ignore */ }
    set({ user: null, modulePermissions: {}, clients: null, isAuthenticated: false });
  },
}));
