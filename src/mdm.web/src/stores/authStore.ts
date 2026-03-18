import { create } from "zustand";
import apiClient from "../lib/apiClient";
import { createClients, type Clients } from "../lib/client";

interface UserInfo {
  id: string;
  username: string;
  role: string;
  display_name: string;
}

interface AuthStore {
  user: UserInfo | null;
  clients: Clients | null;
  isLoading: boolean;
  isAuthenticated: boolean;

  checkAuth: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  clients: null,
  isLoading: true,
  isAuthenticated: false,

  checkAuth: async () => {
    // Try cookie-based auth first
    try {
      const { data } = await apiClient.get("/api/me");
      set({
        user: data,
        clients: createClients(),
        isAuthenticated: true,
        isLoading: false,
      });
      return;
    } catch {
      // Cookie auth failed, not logged in
    }

    set({ user: null, clients: null, isAuthenticated: false, isLoading: false });
  },

  login: async (username: string, password: string) => {
    // Use REST login (sets HttpOnly cookie)
    const { data } = await apiClient.post("/api/login", { username, password });
    set({
      user: data.user,
      clients: createClients(),
      isAuthenticated: true,
    });
  },

  logout: async () => {
    try {
      await apiClient.post("/api/logout");
    } catch { /* ignore */ }
    set({ user: null, clients: null, isAuthenticated: false });
  },
}));
