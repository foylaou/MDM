import { useEffect, useState } from "react";
import { useAuthStore } from "../stores/authStore";
import { useDeviceStore } from "../stores/deviceStore";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { Search, RefreshCw, Send, Info, X, Filter } from "lucide-react";
import apiClient from "../lib/apiClient";

interface CategoryOption { id: string; name: string; level: number; parent_id: string | null; }
interface UserOption { id: string; username: string; display_name: string; }

export function Devices() {
  const { t } = useTranslation();
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
      alert(t("devices.syncSuccess", { count: resp.syncedCount }));
      loadDevices();
    } catch (err) {
      alert(t("devices.syncFailed") + ": " + (err instanceof Error ? err.message : ""));
    } finally { setSyncing(false); }
  };

  const handleSyncInfo = async () => {
    setSyncingInfo(true);
    try {
      const { data } = await apiClient.post("/api/sync-device-info");
      alert(t("assets.syncInfo") + `: ${data.count} devices`);
    } catch (err) {
      alert("Sync failed: " + (err instanceof Error ? err.message : ""));
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
          <label className="input input-bordered input-sm flex items-center gap-2">
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
      <div className="card bg-base-100 shadow">
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
                <th>{t("devices.name")}</th>
                <th>{t("devices.serial")}</th>
                <th>分類</th>
                <th>保管人</th>
                <th>{t("devices.model")}</th>
                <th>{t("devices.os")}</th>
                <th>{t("devices.lastSeen")}</th>
                <th>{t("common.status")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={9} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : devices.length === 0 ? (
                <tr><td colSpan={9} className="text-center py-8 text-base-content/50">{t("devices.noDevices")}</td></tr>
              ) : devices.map((d) => (
                <tr key={d.udid} className="hover cursor-pointer" onClick={() => navigate(`/devices/${d.udid}`)}>
                  <th onClick={(e) => e.stopPropagation()}>
                    <label>
                      <input type="checkbox" className="checkbox checkbox-sm"
                        checked={selected.has(d.udid)} onChange={() => toggleSelect(d.udid)} />
                    </label>
                  </th>
                  <td><div className="font-medium text-primary">{d.device_name || "-"}</div></td>
                  <td className="font-mono text-xs">{d.serial_number}</td>
                  <td className="text-sm">
                    {d.category_name ? <span className="badge badge-ghost badge-sm">{d.category_name}</span> : <span className="opacity-30">-</span>}
                  </td>
                  <td className="text-sm">{d.custodian_name || <span className="opacity-30">-</span>}</td>
                  <td className="text-sm opacity-70">{d.model || "-"}</td>
                  <td className="text-sm">{d.os_version || "-"}</td>
                  <td className="text-sm opacity-70">{d.last_seen ? new Date(d.last_seen).toLocaleString() : "-"}</td>
                  <td>
                    <div className="flex gap-1 flex-wrap">
                      <span className={`badge badge-sm ${d.enrollment_status === "enrolled" ? "badge-success" : "badge-ghost"}`}>
                        {d.enrollment_status}
                      </span>
                      {d.is_lost_mode && <span className="badge badge-error badge-sm">遺失</span>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
