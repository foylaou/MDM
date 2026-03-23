import { create } from "zustand";
import apiClient from "../lib/apiClient";

export interface DeviceRow {
  udid: string;
  serial_number: string;
  device_name: string;
  model: string;
  os_version: string;
  last_seen: string;
  enrollment_status: string;
  is_supervised: boolean;
  is_lost_mode: boolean;
  battery_level: number;
  custodian_name: string;
  category_name: string;
  category_id: string | null;
  custodian_id: string | null;
  asset_status: string;
}

interface DeviceFilters {
  search: string;
  categoryId: string;
  custodianId: string;
}

interface DeviceStore {
  devices: DeviceRow[];
  total: number;
  loading: boolean;
  filters: DeviceFilters;
  selected: Set<string>;

  setFilter: (key: keyof DeviceFilters, value: string) => void;
  clearFilters: () => void;
  loadDevices: () => Promise<void>;
  toggleSelect: (udid: string) => void;
  selectAll: () => void;
  clearSelection: () => void;
}

export const useDeviceStore = create<DeviceStore>((set, get) => ({
  devices: [],
  total: 0,
  loading: false,
  filters: { search: "", categoryId: "", custodianId: "" },
  selected: new Set<string>(),

  setFilter: (key, value) => {
    set((s) => ({ filters: { ...s.filters, [key]: value } }));
    get().loadDevices();
  },

  clearFilters: () => {
    set({ filters: { search: "", categoryId: "", custodianId: "" } });
    get().loadDevices();
  },

  loadDevices: async () => {
    set({ loading: true });
    try {
      const { filters } = get();
      const params: Record<string, string> = {};
      if (filters.search) params.filter = filters.search;
      if (filters.categoryId) params.category_id = filters.categoryId;
      if (filters.custodianId) params.custodian_id = filters.custodianId;
      const { data } = await apiClient.get("/api/devices-list", { params });
      set({ devices: data.devices || [], total: data.total || 0 });
    } catch (err) {
      console.error("Load devices:", err);
    } finally {
      set({ loading: false });
    }
  },

  toggleSelect: (udid) => set((s) => {
    const next = new Set(s.selected);
    if (next.has(udid)) next.delete(udid); else next.add(udid);
    return { selected: next };
  }),

  selectAll: () => set((s) => ({
    selected: s.selected.size === s.devices.length
      ? new Set<string>()
      : new Set(s.devices.map((d) => d.udid)),
  })),

  clearSelection: () => set({ selected: new Set<string>() }),
}));
