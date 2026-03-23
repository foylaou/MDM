import { useEffect, useState } from "react";
import { useAuthStore } from "../stores/authStore";
import { useDeviceStore } from "../stores/deviceStore";
import { useTranslation } from "react-i18next";
import { useDialog } from "../components/DialogProvider";
import { Link, useNavigate } from "react-router-dom";
import { Search, RefreshCw, Send, Info, X, Filter, Columns } from "lucide-react";
import apiClient from "../lib/apiClient";

const ASSET_STATUS_CONFIG: Record<string, { label: string; badge: string }> = {
  available:  { label: "可用",   badge: "badge-success" },
  rented:     { label: "借出",   badge: "badge-warning" },
  faulty:     { label: "故障",   badge: "badge-error" },
  repairing:  { label: "維修中", badge: "badge-info" },
  lost:       { label: "遺失",   badge: "badge-error" },
  retired:    { label: "報廢",   badge: "badge-ghost" },
};

interface ColumnDef {
  key: string;
  label: string;
  defaultVisible: boolean;
}

const ALL_COLUMNS: ColumnDef[] = [
  { key: "device_name",       label: "裝置名稱",   defaultVisible: true },
  { key: "serial_number",     label: "序號",       defaultVisible: true },
  { key: "category_name",     label: "分類",       defaultVisible: true },
  { key: "custodian_name",    label: "保管人",     defaultVisible: true },
  { key: "asset_status",      label: "裝置狀態",   defaultVisible: true },
  { key: "model",             label: "型號",       defaultVisible: true },
  { key: "os_version",        label: "系統版本",   defaultVisible: true },
  { key: "last_seen",         label: "最後上線",   defaultVisible: true },
  { key: "enrollment_status", label: "註冊狀態",   defaultVisible: true },
];

function loadVisibleColumns(): Set<string> {
  try {
    const saved = localStorage.getItem("mdm_device_columns");
    if (saved) return new Set(JSON.parse(saved));
  } catch { /* ignore */ }
  return new Set(ALL_COLUMNS.filter((c) => c.defaultVisible).map((c) => c.key));
}

function saveVisibleColumns(cols: Set<string>) {
  localStorage.setItem("mdm_device_columns", JSON.stringify([...cols]));
}

interface CategoryOption { id: string; name: string; level: number; parent_id: string | null; }
interface UserOption { id: string; username: string; display_name: string; }

export function Devices() {
  const { t } = useTranslation();
  const dialog = useDialog();
  const { clients } = useAuthStore();
  const {
    devices, total, loading, filters,
    setFilter, clearFilters, loadDevices,
    selected, toggleSelect, selectAll,
  } = useDeviceStore();
  const navigate = useNavigate();
  const [syncing, setSyncing] = useState(false);
  const [syncingInfo, setSyncingInfo] = useState(false);
  const [categories, setCategories] = useState<CategoryOption[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [visibleCols, setVisibleCols] = useState<Set<string>>(loadVisibleColumns);
  const [showColPicker, setShowColPicker] = useState(false);

  const toggleColumn = (key: string) => {
    setVisibleCols((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      saveVisibleColumns(next);
      return next;
    });
  };

  const isColVisible = (key: string) => visibleCols.has(key);
  const visibleCount = ALL_COLUMNS.filter((c) => visibleCols.has(c.key)).length + 1; // +1 for checkbox column

  // Load filter options
  useEffect(() => {
    apiClient.get("/api/categories").then(({ data }) => setCategories(data.categories || [])).catch(() => {});
    apiClient.get("/api/users-list").then(({ data }) => setUsers(data.users || [])).catch(() => {});
  }, []);

  // Initial load
  useEffect(() => { loadDevices(); }, []);

  const handleSync = async () => {
    if (!clients) return;
    setSyncing(true);
    try {
      const resp = await clients.device.syncDevices({});
      await dialog.success(t("devices.syncSuccess", { count: resp.syncedCount }));
      loadDevices();
    } catch (err) {
      await dialog.error(t("devices.syncFailed") + ": " + (err instanceof Error ? err.message : ""));
    } finally { setSyncing(false); }
  };

  const handleSyncInfo = async () => {
    setSyncingInfo(true);
    try {
      const { data } = await apiClient.post("/api/sync-device-info");
      await dialog.success(t("assets.syncInfo") + `: ${data.count} devices`);
    } catch (err) {
      await dialog.error("Sync failed: " + (err instanceof Error ? err.message : ""));
    } finally { setSyncingInfo(false); }
  };

  // Build category options with indentation
  const categoryOptions = categories.map((c) => ({
    ...c,
    label: "\u00A0\u00A0".repeat(c.level) + c.name,
  }));

  const hasFilters = filters.categoryId || filters.custodianId;

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("devices.title")}</h1>
          <p className="text-sm text-base-content/60">{t("devices.count", { count: total })}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <label className="input input-bordered input-sm flex items-center gap-2" data-tour="device-search">
            <Search size={14} className="opacity-50" />
            <input
              type="text"
              placeholder={t("common.search")}
              value={filters.search}
              onChange={(e) => setFilter("search", e.target.value)}
              className="grow w-32"
            />
          </label>
          <button onClick={handleSync} disabled={syncing} className="btn btn-success btn-sm gap-1">
            {syncing ? <span className="loading loading-spinner loading-xs"></span> : <RefreshCw size={14} />}
            {t("devices.sync")}
          </button>
          <button onClick={handleSyncInfo} disabled={syncingInfo} className="btn btn-info btn-sm gap-1">
            {syncingInfo ? <span className="loading loading-spinner loading-xs"></span> : <Info size={14} />}
            {t("assets.syncInfo")}
          </button>
          <div className="dropdown dropdown-end">
            <button tabIndex={0} onClick={() => setShowColPicker(!showColPicker)} className="btn btn-ghost btn-sm gap-1">
              <Columns size={14} /> 欄位
            </button>
            {showColPicker && (
              <ul tabIndex={0} className="dropdown-content menu bg-base-100 rounded-box shadow-lg z-20 w-52 p-2"
                onMouseLeave={() => setShowColPicker(false)}>
                {ALL_COLUMNS.map((col) => (
                  <li key={col.key}>
                    <label className="flex items-center gap-2 cursor-pointer">
                      <input type="checkbox" className="checkbox checkbox-sm"
                        checked={visibleCols.has(col.key)}
                        onChange={() => toggleColumn(col.key)} />
                      <span className="text-sm">{col.label}</span>
                    </label>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-2 items-center">
        <Filter size={14} className="opacity-50" />
        <select
          value={filters.categoryId}
          onChange={(e) => setFilter("categoryId", e.target.value)}
          className="select select-bordered select-sm"
        >
          <option value="">全部分類</option>
          {categoryOptions.map((c) => (
            <option key={c.id} value={c.id}>{c.label}</option>
          ))}
        </select>
        <select
          value={filters.custodianId}
          onChange={(e) => setFilter("custodianId", e.target.value)}
          className="select select-bordered select-sm"
        >
          <option value="">全部保管人</option>
          {users.map((u) => (
            <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
          ))}
        </select>
        {hasFilters && (
          <button onClick={clearFilters} className="btn btn-ghost btn-sm gap-1">
            <X size={14} /> 清除篩選
          </button>
        )}
      </div>

      {/* Selection bar */}
      {selected.size > 0 && (
        <div role="alert" className="alert alert-info">
          <span className="font-medium">{t("common.selected", { count: selected.size })}</span>
          <Link to={`/commands?udids=${Array.from(selected).join(",")}`} className="btn btn-sm btn-primary gap-1">
            <Send size={14} />{t("devices.sendCommand")}
          </Link>
        </div>
      )}

      {/* Table */}
      <div className="card bg-base-100 shadow" data-tour="device-table">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>
                  <label>
                    <input type="checkbox" className="checkbox checkbox-sm"
                      checked={selected.size === devices.length && devices.length > 0} onChange={selectAll} />
                  </label>
                </th>
                {isColVisible("device_name") && <th>{t("devices.name")}</th>}
                {isColVisible("serial_number") && <th>{t("devices.serial")}</th>}
                {isColVisible("category_name") && <th>分類</th>}
                {isColVisible("custodian_name") && <th>保管人</th>}
                {isColVisible("asset_status") && <th>裝置狀態</th>}
                {isColVisible("model") && <th>{t("devices.model")}</th>}
                {isColVisible("os_version") && <th>{t("devices.os")}</th>}
                {isColVisible("last_seen") && <th>{t("devices.lastSeen")}</th>}
                {isColVisible("enrollment_status") && <th>{t("common.status")}</th>}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={visibleCount} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : devices.length === 0 ? (
                <tr><td colSpan={visibleCount} className="text-center py-8 text-base-content/50">{t("devices.noDevices")}</td></tr>
              ) : devices.map((d) => {
                const st = ASSET_STATUS_CONFIG[d.asset_status] || ASSET_STATUS_CONFIG.available;
                return (
                  <tr key={d.udid} className="hover cursor-pointer" onClick={() => navigate(`/devices/${d.udid}`)}>
                    <th onClick={(e) => e.stopPropagation()}>
                      <label>
                        <input type="checkbox" className="checkbox checkbox-sm"
                          checked={selected.has(d.udid)} onChange={() => toggleSelect(d.udid)} />
                      </label>
                    </th>
                    {isColVisible("device_name") && <td><div className="font-medium text-primary">{d.device_name || "-"}</div></td>}
                    {isColVisible("serial_number") && <td className="font-mono text-xs">{d.serial_number}</td>}
                    {isColVisible("category_name") && (
                      <td className="text-sm">
                        {d.category_name ? <span className="badge badge-ghost badge-sm">{d.category_name}</span> : <span className="opacity-30">-</span>}
                      </td>
                    )}
                    {isColVisible("custodian_name") && <td className="text-sm">{d.custodian_name || <span className="opacity-30">-</span>}</td>}
                    {isColVisible("asset_status") && (
                      <td>
                        <span className={`badge badge-sm ${st.badge}`}>{st.label}</span>
                      </td>
                    )}
                    {isColVisible("model") && <td className="text-sm opacity-70">{d.model || "-"}</td>}
                    {isColVisible("os_version") && <td className="text-sm">{d.os_version || "-"}</td>}
                    {isColVisible("last_seen") && <td className="text-sm opacity-70">{d.last_seen ? new Date(d.last_seen).toLocaleString() : "-"}</td>}
                    {isColVisible("enrollment_status") && (
                      <td>
                        <span className={`badge badge-sm ${d.enrollment_status === "enrolled" ? "badge-success" : "badge-ghost"}`}>
                          {d.enrollment_status}
                        </span>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
