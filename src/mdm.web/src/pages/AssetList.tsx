import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useDialog } from "../components/DialogProvider";
import { Link } from "react-router-dom";
import { Search, Plus, Download, Filter, X, Edit3, Trash2 } from "lucide-react";
import apiClient from "../lib/apiClient";
import { AssetForm } from "../components/AssetForm";

interface AssetRow {
  id: string;
  device_udid: string | null;
  asset_number: string;
  name: string;
  spec: string;
  quantity: number;
  unit: string;
  acquired_date: string | null;
  unit_price: number;
  purpose: string;
  assigned_date: string | null;
  custodian_id: string | null;
  custodian_name: string;
  current_holder_id: string | null;
  current_holder_name: string;
  location: string;
  asset_category: string;
  category_id: string | null;
  category_name: string;
  asset_status: string;
  device_name: string;
  device_serial: string;
  notes: string;
}

interface CategoryOption { id: string; name: string; level: number; }
interface UserOption { id: string; username: string; display_name: string; }

const ASSET_STATUS_CONFIG: Record<string, { label: string; badge: string }> = {
  available:    { label: "可用",   badge: "badge-success" },
  rented:       { label: "借出",   badge: "badge-warning" },
  faulty:       { label: "故障",   badge: "badge-error" },
  repairing:    { label: "維修中", badge: "badge-info" },
  lost:         { label: "遺失",   badge: "badge-error" },
  retired:      { label: "報廢",   badge: "badge-ghost" },
  transferred:  { label: "移撥",   badge: "badge-ghost" },
};

export function AssetList() {
  const { t } = useTranslation();
  const dialog = useDialog();
  const [assets, setAssets] = useState<AssetRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [categories, setCategories] = useState<CategoryOption[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [filterCategory, setFilterCategory] = useState("");
  const [filterCustodian, setFilterCustodian] = useState("");
  const [filterStatus, setFilterStatus] = useState("");
  const [filterLinked, setFilterLinked] = useState(""); // "mdm" | "standalone" | ""
  const [editingAssetId, setEditingAssetId] = useState<string | null>(null); // asset id or "new"

  const loadAssets = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/assets");
      setAssets(data.assets || []);
    } catch (err) { console.error("Load assets:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => {
    loadAssets();
    apiClient.get("/api/categories").then(({ data }) => setCategories(data.categories || [])).catch(() => {});
    apiClient.get("/api/users-list").then(({ data }) => setUsers(data.users || [])).catch(() => {});
  }, []);

  const deleteAsset = async (id: string) => {
    if (!(await dialog.confirm("確定要刪除此財產資訊？"))) return;
    try {
      await apiClient.delete(`/api/assets/${id}`);
      loadAssets();
    } catch { await dialog.error("刪除失敗"); }
  };

  const hasFilters = filterCategory || filterCustodian || filterStatus || filterLinked;
  const clearFilters = () => { setFilterCategory(""); setFilterCustodian(""); setFilterStatus(""); setFilterLinked(""); };

  // Client-side filtering
  const filtered = assets.filter((a) => {
    if (search) {
      const q = search.toLowerCase();
      if (
        !a.asset_number.toLowerCase().includes(q) &&
        !a.name.toLowerCase().includes(q) &&
        !a.custodian_name.toLowerCase().includes(q) &&
        !(a.device_name || "").toLowerCase().includes(q) &&
        !(a.device_serial || "").toLowerCase().includes(q) &&
        !a.spec.toLowerCase().includes(q)
      ) return false;
    }
    if (filterCategory && a.category_id !== filterCategory) return false;
    if (filterCustodian && a.custodian_id !== filterCustodian) return false;
    if (filterStatus && a.asset_status !== filterStatus) return false;
    if (filterLinked === "mdm" && !a.device_udid) return false;
    if (filterLinked === "standalone" && a.device_udid) return false;
    return true;
  });

  const categoryOptions = categories.map((c) => ({
    ...c,
    label: "\u00A0\u00A0".repeat(c.level) + c.name,
  }));

  // Editing inline
  if (editingAssetId) {
    const editingAsset = editingAssetId === "new" ? null : assets.find((a) => a.id === editingAssetId);
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">{t("nav.assets")}</h1>
          <button onClick={() => setEditingAssetId(null)} className="btn btn-ghost btn-sm gap-1">
            <X size={14} /> {t("common.cancel")}
          </button>
        </div>
        <div className="card bg-base-100 shadow p-6">
          <AssetForm
            assetId={editingAssetId === "new" ? undefined : editingAssetId}
            deviceUdid={editingAsset?.device_udid || undefined}
            onSaved={() => { setEditingAssetId(null); loadAssets(); }}
            onCancel={() => setEditingAssetId(null)}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("nav.assets")}</h1>
          <p className="text-sm text-base-content/60">{t("common.showing", { count: filtered.length })}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <label className="input input-bordered input-sm flex items-center gap-2">
            <Search size={14} className="opacity-50" />
            <input
              type="text"
              placeholder={t("common.search")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="grow w-32"
            />
          </label>
          <button onClick={() => setEditingAssetId("new")} className="btn btn-primary btn-sm gap-1">
            <Plus size={14} /> {t("assets.add")}
          </button>
          <button onClick={() => window.open("/api/assets-export", "_blank")} className="btn btn-outline btn-sm gap-1">
            <Download size={14} /> Excel
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-2 items-center">
        <Filter size={14} className="opacity-50" />
        <select value={filterCategory} onChange={(e) => setFilterCategory(e.target.value)} className="select select-bordered select-sm">
          <option value="">全部分類</option>
          {categoryOptions.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
        </select>
        <select value={filterCustodian} onChange={(e) => setFilterCustodian(e.target.value)} className="select select-bordered select-sm">
          <option value="">全部保管人</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.display_name || u.username}</option>)}
        </select>
        <select value={filterStatus} onChange={(e) => setFilterStatus(e.target.value)} className="select select-bordered select-sm">
          <option value="">全部狀態</option>
          {Object.entries(ASSET_STATUS_CONFIG).map(([val, cfg]) => (
            <option key={val} value={val}>{cfg.label}</option>
          ))}
        </select>
        <select value={filterLinked} onChange={(e) => setFilterLinked(e.target.value)} className="select select-bordered select-sm">
          <option value="">全部來源</option>
          <option value="mdm">MDM 裝置</option>
          <option value="standalone">獨立資產</option>
        </select>
        {hasFilters && (
          <button onClick={clearFilters} className="btn btn-ghost btn-sm gap-1">
            <X size={14} /> 清除篩選
          </button>
        )}
      </div>

      {/* Table */}
      <div className="card bg-base-100 shadow">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t("assets.assetNumber")}</th>
                <th>{t("assets.name")}</th>
                <th>{t("assets.spec")}</th>
                <th>分類</th>
                <th>{t("assets.custodian")}</th>
                <th>{t("assets.currentHolder")}</th>
                <th>狀態</th>
                <th>關聯裝置</th>
                <th>{t("assets.location")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={10} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : filtered.length === 0 ? (
                <tr><td colSpan={10} className="text-center py-8 text-base-content/50">{t("assets.noAsset")}</td></tr>
              ) : filtered.map((a) => {
                const st = ASSET_STATUS_CONFIG[a.asset_status] || ASSET_STATUS_CONFIG.available;
                return (
                  <tr key={a.id} className="hover">
                    <td className="font-mono text-xs">{a.asset_number || "-"}</td>
                    <td className="font-medium">{a.name || "-"}</td>
                    <td className="text-sm opacity-70">{a.spec || "-"}</td>
                    <td className="text-sm">
                      {a.category_name ? <span className="badge badge-ghost badge-sm">{a.category_name}</span> : <span className="opacity-30">-</span>}
                    </td>
                    <td className="text-sm">{a.custodian_name || <span className="opacity-30">-</span>}</td>
                    <td className="text-sm">
                      {a.current_holder_name
                        ? <span className="badge badge-warning badge-sm">{a.current_holder_name}</span>
                        : <span className="opacity-30">-</span>}
                    </td>
                    <td><span className={`badge badge-sm ${st.badge}`}>{st.label}</span></td>
                    <td className="text-sm">
                      {a.device_udid ? (
                        <Link to={`/mdm/devices/${a.device_udid}`} className="link link-primary text-xs">
                          {a.device_name || a.device_serial || a.device_udid.substring(0, 8)}
                        </Link>
                      ) : (
                        <span className="badge badge-outline badge-sm">獨立資產</span>
                      )}
                    </td>
                    <td className="text-sm opacity-70">{a.location || "-"}</td>
                    <td>
                      <div className="flex gap-1">
                        <button onClick={() => setEditingAssetId(a.id)} className="btn btn-ghost btn-xs"><Edit3 size={14} /></button>
                        <button onClick={() => deleteAsset(a.id)} className="btn btn-ghost btn-xs text-error"><Trash2 size={14} /></button>
                      </div>
                    </td>
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
