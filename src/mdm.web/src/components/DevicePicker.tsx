import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Search, X, Tablet, Check, Hash } from "lucide-react";
import apiClient from "../lib/apiClient";

interface CategoryOption { id: string; name: string; level: number; }

interface DeviceItem {
  udid: string;
  serial_number: string;
  device_name: string;
  model: string;
  os_version: string;
  enrollment_status: string;
  asset_status: string;
}

interface DevicePickerProps {
  selected: string[];
  onChange: (udids: string[]) => void;
  showFilters?: boolean;
}

const statusConfig: Record<string, { label: string; color: string }> = {
  available:  { label: "可用",   color: "badge-success" },
  rented:     { label: "借出",   color: "badge-warning" },
  broken:     { label: "故障",   color: "badge-error" },
  repairing:  { label: "維修中", color: "badge-info" },
  lost:       { label: "遺失",   color: "badge-error" },
  retired:    { label: "報廢",   color: "badge-ghost" },
};

export function DevicePicker({ selected, onChange, showFilters }: DevicePickerProps) {
  const { t } = useTranslation();
  const [devices, setDevices] = useState<DeviceItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [categories, setCategories] = useState<CategoryOption[]>([]);
  const [categoryFilter, setCategoryFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState(showFilters ? "available" : "");
  const [quickQty, setQuickQty] = useState("");

  useEffect(() => {
    setLoading(true);
    apiClient.get("/api/devices-available")
      .then(({ data }) => setDevices(data.devices || []))
      .catch((err) => console.error("DevicePicker load:", err))
      .finally(() => setLoading(false));
  }, []);

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
          d.device_name.toLowerCase().includes(q) ||
          d.serial_number.toLowerCase().includes(q) ||
          d.udid.toLowerCase().includes(q) ||
          d.model.toLowerCase().includes(q)
      );
    }
    if (statusFilter) {
      result = result.filter((d) => d.asset_status === statusFilter);
    }
    return result;
  }, [devices, search, statusFilter]);

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

  const handleQuickQty = () => {
    const qty = parseInt(quickQty);
    if (isNaN(qty) || qty <= 0) return;
    const unselected = filtered.filter((d) => !selectedSet.has(d.udid));
    const toSelect = unselected.slice(0, qty).map((d) => d.udid);
    onChange([...selected, ...toSelect]);
    setQuickQty("");
  };

  const selectedDevices = devices.filter((d) => selectedSet.has(d.udid));

  // Count by status for filter badges
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const d of devices) {
      counts[d.asset_status] = (counts[d.asset_status] || 0) + 1;
    }
    return counts;
  }, [devices]);

  return (
    <div className="space-y-3">
      {/* Selected tags */}
      {selectedDevices.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {selectedDevices.map((d) => (
            <span key={d.udid} className="badge badge-primary gap-1">
              {d.device_name || d.serial_number}
              <button onClick={() => toggle(d.udid)} className="hover:opacity-70"><X size={12} /></button>
            </span>
          ))}
          <button onClick={clearAll} className="badge badge-ghost gap-1 cursor-pointer hover:badge-error">
            {t("devicePicker.clearAll")}
          </button>
        </div>
      )}

      {/* Search + quick qty */}
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

      {/* Status filter pills + Category */}
      {showFilters && (
        <div className="space-y-2">
          <div className="flex gap-1 flex-wrap">
            <button
              onClick={() => setStatusFilter("")}
              className={`btn btn-xs gap-1 ${statusFilter === "" ? "btn-neutral" : "btn-ghost"}`}
            >
              全部 <span className="badge badge-xs">{devices.length}</span>
            </button>
            {Object.entries(statusConfig).map(([key, cfg]) => {
              const count = statusCounts[key] || 0;
              if (count === 0 && key !== "available") return null;
              return (
                <button
                  key={key}
                  onClick={() => setStatusFilter(statusFilter === key ? "" : key)}
                  className={`btn btn-xs gap-1 ${statusFilter === key ? "btn-neutral" : "btn-ghost"}`}
                >
                  <span className={`badge badge-xs ${cfg.color}`}></span>
                  {cfg.label}
                  <span className="badge badge-xs">{count}</span>
                </button>
              );
            })}
          </div>
          {categories.length > 0 && (
            <select value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)} className="select select-bordered select-sm w-full">
              <option value="">全部分類</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{"　".repeat(c.level)}{c.name}</option>
              ))}
            </select>
          )}
        </div>
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
              const sc = statusConfig[d.asset_status] || statusConfig.available;
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
                    <div className="text-sm font-medium truncate">{d.device_name || d.serial_number || d.udid}</div>
                    <div className="text-xs opacity-50 truncate">
                      {d.serial_number} {d.model ? `· ${d.model}` : ""} {d.os_version ? `· ${d.os_version}` : ""}
                    </div>
                  </div>
                  <span className={`badge badge-xs flex-shrink-0 ${sc.color}`}>
                    {sc.label}
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
