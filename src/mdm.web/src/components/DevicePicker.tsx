import { useState, useEffect, useMemo } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { Search, X, Tablet, Check, Hash } from "lucide-react";
import apiClient from "../lib/apiClient";
import type { Device } from "../gen/mdm/v1/device_pb";

interface CategoryOption { id: string; name: string; level: number; }

interface DevicePickerProps {
  selected: string[];
  onChange: (udids: string[]) => void;
  showFilters?: boolean; // show category + rental status filters
}

export function DevicePicker({ selected, onChange, showFilters }: DevicePickerProps) {
  const { t } = useTranslation();
  const { clients } = useAuthStore();
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [categories, setCategories] = useState<CategoryOption[]>([]);
  const [categoryFilter, setCategoryFilter] = useState("");
  const [quickQty, setQuickQty] = useState("");

  useEffect(() => {
    if (!clients) return;
    setLoading(true);
    clients.device.listDevices({ pageSize: 200 })
      .then((resp) => setDevices(resp.devices))
      .catch((err) => console.error("DevicePicker load:", err))
      .finally(() => setLoading(false));
  }, [clients]);

  useEffect(() => {
    if (showFilters) {
      apiClient.get("/api/categories").then(({ data }) => setCategories(data.categories || [])).catch(() => {});
    }
  }, [showFilters]);

  const filtered = useMemo(() => {
    let result = devices;
    if (search) {
      const q = search.toLowerCase();
      result = result.filter(
        (d) =>
          d.deviceName.toLowerCase().includes(q) ||
          d.serialNumber.toLowerCase().includes(q) ||
          d.udid.toLowerCase().includes(q) ||
          d.model.toLowerCase().includes(q)
      );
    }
    return result;
  }, [devices, search]);

  const selectedSet = useMemo(() => new Set(selected), [selected]);

  const toggle = (udid: string) => {
    if (selectedSet.has(udid)) {
      onChange(selected.filter((u) => u !== udid));
    } else {
      onChange([...selected, udid]);
    }
  };

  const selectAll = () => {
    const allFilteredUdids = filtered.map((d) => d.udid);
    const allSelected = allFilteredUdids.every((u) => selectedSet.has(u));
    if (allSelected) {
      onChange(selected.filter((u) => !allFilteredUdids.includes(u)));
    } else {
      const merged = new Set([...selected, ...allFilteredUdids]);
      onChange([...merged]);
    }
  };

  const clearAll = () => onChange([]);

  // Quick quantity: select N random unselected devices
  const handleQuickQty = () => {
    const qty = parseInt(quickQty);
    if (isNaN(qty) || qty <= 0) return;
    const unselected = filtered.filter((d) => !selectedSet.has(d.udid));
    const toSelect = unselected.slice(0, qty).map((d) => d.udid);
    onChange([...selected, ...toSelect]);
    setQuickQty("");
  };

  const selectedDevices = devices.filter((d) => selectedSet.has(d.udid));

  return (
    <div className="space-y-3">
      {/* Selected tags */}
      {selectedDevices.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {selectedDevices.map((d) => (
            <span key={d.udid} className="badge badge-primary gap-1">
              {d.deviceName || d.serialNumber}
              <button onClick={() => toggle(d.udid)} className="hover:opacity-70"><X size={12} /></button>
            </span>
          ))}
          <button onClick={clearAll} className="badge badge-ghost gap-1 cursor-pointer hover:badge-error">
            {t("devicePicker.clearAll")}
          </button>
        </div>
      )}

      {/* Search + quick qty + filters */}
      <div className="flex gap-2 flex-wrap">
        <label className="input input-bordered input-sm flex items-center gap-2 flex-1 min-w-48">
          <Search size={14} className="opacity-50" />
          <input type="text" placeholder={t("devicePicker.placeholder")} value={search} onChange={(e) => setSearch(e.target.value)} className="grow" />
        </label>
        <div className="join">
          <input
            type="number"
            min="1"
            placeholder="數量"
            value={quickQty}
            onChange={(e) => setQuickQty(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleQuickQty()}
            className="input input-bordered input-sm join-item w-20"
          />
          <button onClick={handleQuickQty} disabled={!quickQty} className="btn btn-sm join-item gap-1">
            <Hash size={12} /> 快選
          </button>
        </div>
        <button onClick={selectAll} className="btn btn-ghost btn-sm">{t("devicePicker.selectAll")}</button>
      </div>

      {/* Category filter */}
      {showFilters && categories.length > 0 && (
        <select value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)} className="select select-bordered select-sm w-full">
          <option value="">全部分類</option>
          {categories.map((c) => (
            <option key={c.id} value={c.id}>{"　".repeat(c.level)}{c.name}</option>
          ))}
        </select>
      )}

      {/* Summary */}
      {selected.length > 0 && (
        <div className="text-sm font-medium text-primary">
          {t("devicePicker.selected", { count: selected.length })}
        </div>
      )}

      {/* Device list */}
      <div className="border border-base-300 rounded-lg max-h-56 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-6">
            <span className="loading loading-spinner loading-sm mr-2"></span>
            {t("devicePicker.loading")}
          </div>
        ) : filtered.length === 0 ? (
          <div className="text-center py-6 text-base-content/50 text-sm">{t("devicePicker.noResults")}</div>
        ) : (
          <ul className="divide-y divide-base-200">
            {filtered.map((d) => {
              const isSelected = selectedSet.has(d.udid);
              return (
                <li
                  key={d.udid}
                  onClick={() => toggle(d.udid)}
                  className={`flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-base-200 transition-colors ${isSelected ? "bg-primary/5" : ""}`}
                >
                  <div className={`w-5 h-5 rounded border flex items-center justify-center flex-shrink-0 ${isSelected ? "bg-primary border-primary text-primary-content" : "border-base-300"}`}>
                    {isSelected && <Check size={12} />}
                  </div>
                  <Tablet size={16} className="opacity-40 flex-shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium truncate">{d.deviceName || d.serialNumber || d.udid}</div>
                    <div className="text-xs opacity-50 truncate">
                      {d.serialNumber} {d.model ? `· ${d.model}` : ""} {d.osVersion ? `· ${d.osVersion}` : ""}
                    </div>
                  </div>
                  <span className={`badge badge-xs flex-shrink-0 ${d.enrollmentStatus === "enrolled" ? "badge-success" : "badge-ghost"}`}>
                    {d.enrollmentStatus}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
