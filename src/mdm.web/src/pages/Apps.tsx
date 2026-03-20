import { useState, useEffect, useCallback, useRef } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import apiClient from "../lib/apiClient";
import {
  Package, Plus, Pencil, Trash2, Save, Building2, Download, Search,
} from "lucide-react";

interface ManagedApp {
  id: string;
  name: string;
  bundle_id: string;
  app_type: "vpp" | "enterprise";
  itunes_store_id: string;
  manifest_url: string;
  purchased_qty: number;
  notes: string;
  installed_count: number;
  icon_url: string;
  created_at: string;
  updated_at: string;
}

const emptyForm = {
  name: "",
  bundle_id: "",
  app_type: "vpp" as "vpp" | "enterprise",
  itunes_store_id: "",
  manifest_url: "",
  purchased_qty: 0,
  notes: "",
  icon_url: "",
};

export function Apps() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const userRole = user?.role || "viewer";

  const [apps, setApps] = useState<ManagedApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState(emptyForm);
  const [saving, setSaving] = useState(false);
  const [lookingUp, setLookingUp] = useState(false);
  const lookupTimer = useRef<ReturnType<typeof setTimeout>>();

  // App Store search state
  interface SearchResult {
    trackId: number;
    trackName: string;
    bundleId: string;
    artworkUrl60: string;
    artworkUrl100: string;
    artworkUrl512: string;
    sellerName: string;
  }
  const [searchTerm, setSearchTerm] = useState("");
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const searchTimer = useRef<ReturnType<typeof setTimeout>>();

  const doSearch = useCallback(async (term: string) => {
    if (!term || term.length < 2) { setSearchResults([]); return; }
    setSearching(true);
    try {
      const { data } = await apiClient.get("/api/itunes-search", { params: { term, limit: "8" } });
      setSearchResults(data.results || []);
    } catch { setSearchResults([]); }
    finally { setSearching(false); }
  }, []);

  const handleSearchInput = (val: string) => {
    setSearchTerm(val);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => doSearch(val), 400);
  };

  const selectSearchResult = (r: SearchResult) => {
    setForm((prev) => ({
      ...prev,
      name: r.trackName,
      bundle_id: r.bundleId,
      itunes_store_id: String(r.trackId),
      icon_url: r.artworkUrl512 || r.artworkUrl100 || r.artworkUrl60 || "",
    }));
    setSearchTerm("");
    setSearchResults([]);
  };

  const loadApps = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/managed-apps");
      setApps(data.apps || []);
    } catch (err) {
      console.error("Load apps:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadApps(); }, [loadApps]);

  // Auto-lookup from iTunes when bundle_id or itunes_store_id changes (VPP only)
  const doLookup = useCallback(async (bundleId: string, itunesId: string) => {
    if (!bundleId && !itunesId) return;
    setLookingUp(true);
    try {
      const params: Record<string, string> = {};
      if (bundleId) params.bundleId = bundleId;
      else if (itunesId) params.id = itunesId;
      const { data } = await apiClient.get("/api/itunes-lookup", { params });
      if (data.resultCount > 0) {
        const result = data.results[0];
        setForm((prev) => ({
          ...prev,
          name: prev.name || result.trackName || "",
          bundle_id: prev.bundle_id || result.bundleId || "",
          itunes_store_id: prev.itunes_store_id || String(result.trackId || ""),
          icon_url: result.artworkUrl512 || result.artworkUrl100 || result.artworkUrl60 || "",
        }));
      }
    } catch { /* ignore */ }
    finally { setLookingUp(false); }
  }, []);

  const scheduleLookup = (bundleId: string, itunesId: string) => {
    if (lookupTimer.current) clearTimeout(lookupTimer.current);
    lookupTimer.current = setTimeout(() => doLookup(bundleId, itunesId), 600);
  };

  const handleBundleIdChange = (val: string) => {
    setForm((prev) => ({ ...prev, bundle_id: val }));
    if (form.app_type === "vpp" && val.includes(".")) {
      scheduleLookup(val, "");
    }
  };

  const handleItunesIdChange = (val: string) => {
    // parse from URL if pasted
    const match = val.match(/id(\d+)/);
    const parsed = match ? match[1] : val.trim();
    setForm((prev) => ({ ...prev, itunes_store_id: parsed }));
    if (form.app_type === "vpp" && /^\d+$/.test(parsed) && parsed.length >= 6) {
      scheduleLookup("", parsed);
    }
  };

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm);
    setSearchTerm("");
    setSearchResults([]);
    setShowModal(true);
  };

  const openEdit = (app: ManagedApp) => {
    setEditingId(app.id);
    setForm({
      name: app.name,
      bundle_id: app.bundle_id,
      app_type: app.app_type,
      itunes_store_id: app.itunes_store_id,
      manifest_url: app.manifest_url,
      purchased_qty: app.purchased_qty,
      notes: app.notes,
      icon_url: app.icon_url,
    });
    setShowModal(true);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      if (editingId) {
        await apiClient.put(`/api/managed-apps/${editingId}`, form);
      } else {
        await apiClient.post("/api/managed-apps", form);
      }
      setShowModal(false);
      loadApps();
    } catch (err) {
      console.error("Save app:", err);
      alert("儲存失敗");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (app: ManagedApp) => {
    if (!confirm(`確定要刪除「${app.name}」嗎？已安裝的裝置記錄也會一併刪除。`)) return;
    try {
      await apiClient.delete(`/api/managed-apps/${app.id}`);
      loadApps();
    } catch (err) {
      console.error("Delete app:", err);
    }
  };

  const canEdit = userRole === "admin" || userRole === "operator";

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("nav.apps") || "App 管理"}</h1>
          <p className="text-sm text-base-content/60">登記可安裝的 App，管理採購數量與安裝狀態</p>
        </div>
        {canEdit && (
          <button onClick={openCreate} className="btn btn-primary gap-1">
            <Plus size={16} /> 新增 App
          </button>
        )}
      </div>

      <div className="card bg-base-100 shadow">
        <div className="card-body p-0">
          {loading ? (
            <div className="flex justify-center py-12">
              <span className="loading loading-spinner loading-lg"></span>
            </div>
          ) : apps.length === 0 ? (
            <div className="text-center py-12 text-base-content/50">
              <Package size={48} className="mx-auto mb-3 opacity-30" />
              <p>尚未登記任何 App</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="table table-zebra">
                <thead>
                  <tr>
                    <th>App</th>
                    <th>Bundle ID</th>
                    <th>類型</th>
                    <th>ID / URL</th>
                    <th>採購數量</th>
                    <th>已安裝</th>
                    <th>可用</th>
                    <th>備註</th>
                    {canEdit && <th>操作</th>}
                  </tr>
                </thead>
                <tbody>
                  {apps.map((app) => {
                    const available = app.purchased_qty > 0
                      ? app.purchased_qty - app.installed_count
                      : "—";
                    return (
                      <tr key={app.id}>
                        <td>
                          <div className="flex items-center gap-2">
                            {app.icon_url ? (
                              <img src={app.icon_url} alt="" className="w-8 h-8 rounded-lg" />
                            ) : (
                              <div className="w-8 h-8 rounded-lg bg-base-300 flex items-center justify-center">
                                <Package size={16} className="opacity-40" />
                              </div>
                            )}
                            <span className="font-medium">{app.name}</span>
                          </div>
                        </td>
                        <td className="font-mono text-xs">{app.bundle_id}</td>
                        <td>
                          <span className={`badge badge-sm ${app.app_type === "vpp" ? "badge-primary" : "badge-secondary"}`}>
                            {app.app_type === "vpp" ? "VPP" : "企業"}
                          </span>
                        </td>
                        <td className="text-xs max-w-48 truncate">
                          {app.app_type === "vpp" ? app.itunes_store_id : app.manifest_url}
                        </td>
                        <td className="text-center">{app.purchased_qty || "—"}</td>
                        <td className="text-center">{app.installed_count}</td>
                        <td className="text-center">
                          {typeof available === "number" ? (
                            <span className={available <= 0 ? "text-error font-bold" : "text-success"}>
                              {available}
                            </span>
                          ) : available}
                        </td>
                        <td className="text-xs max-w-32 truncate">{app.notes}</td>
                        {canEdit && (
                          <td>
                            <div className="flex gap-1">
                              <button onClick={() => openEdit(app)} className="btn btn-ghost btn-xs">
                                <Pencil size={14} />
                              </button>
                              <button onClick={() => handleDelete(app)} className="btn btn-ghost btn-xs text-error">
                                <Trash2 size={14} />
                              </button>
                            </div>
                          </td>
                        )}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {/* Create / Edit Modal */}
      <dialog className={`modal ${showModal ? "modal-open" : ""}`}>
        <div className="modal-box max-w-lg">
          <h3 className="font-bold text-lg flex items-center gap-2">
            {editingId ? <Pencil size={18} /> : <Plus size={18} />}
            {editingId ? "編輯 App" : "新增 App"}
          </h3>
          <div className="space-y-3 py-4">
            {/* Type selector */}
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">類型</span></label>
              <div className="flex gap-2">
                <label className={`btn btn-sm gap-1 ${form.app_type === "vpp" ? "btn-primary" : "btn-outline"}`}>
                  <input type="radio" className="hidden" checked={form.app_type === "vpp"} onChange={() => setForm({ ...form, app_type: "vpp" })} />
                  <Download size={14} /> VPP App
                </label>
                <label className={`btn btn-sm gap-1 ${form.app_type === "enterprise" ? "btn-secondary" : "btn-outline"}`}>
                  <input type="radio" className="hidden" checked={form.app_type === "enterprise"} onChange={() => setForm({ ...form, app_type: "enterprise" })} />
                  <Building2 size={14} /> 企業 App
                </label>
              </div>
            </div>

            {/* VPP: App Store search + selected preview */}
            {form.app_type === "vpp" && !editingId && (
              <div className="form-control">
                <label className="label"><span className="label-text font-medium">搜尋 App Store</span></label>
                <label className="input input-bordered input-sm flex items-center gap-2">
                  <Search size={14} className="opacity-50" />
                  <input
                    type="text"
                    value={searchTerm}
                    onChange={(e) => handleSearchInput(e.target.value)}
                    className="grow"
                    placeholder="輸入 App 名稱搜尋，例如 Teams、Drive..."
                  />
                  {searching && <span className="loading loading-spinner loading-xs"></span>}
                </label>
                {/* Search results */}
                {searchResults.length > 0 && (
                  <div className="border border-base-300 rounded-lg mt-2 max-h-48 overflow-y-auto divide-y divide-base-200">
                    {searchResults.map((r) => (
                      <div
                        key={r.trackId}
                        onClick={() => selectSearchResult(r)}
                        className="flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-base-200 transition-colors"
                      >
                        <img src={r.artworkUrl60} alt="" className="w-8 h-8 rounded-lg flex-shrink-0" />
                        <div className="flex-1 min-w-0">
                          <div className="text-sm font-medium truncate">{r.trackName}</div>
                          <div className="text-xs opacity-50 truncate">{r.sellerName} · {r.bundleId}</div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Selected app preview */}
            {form.icon_url && (
              <div className="flex items-center gap-3 p-3 bg-base-200 rounded-lg">
                <img src={form.icon_url} alt="" className="w-12 h-12 rounded-xl shadow" />
                <div className="flex-1 min-w-0">
                  {form.name && <div className="font-medium">{form.name}</div>}
                  {form.bundle_id && <div className="text-xs font-mono opacity-60 truncate">{form.bundle_id}</div>}
                  {form.itunes_store_id && <div className="text-xs opacity-40">ID: {form.itunes_store_id}</div>}
                </div>
                {lookingUp && <span className="loading loading-spinner loading-sm"></span>}
              </div>
            )}

            {/* VPP: manual input fallback */}
            {form.app_type === "vpp" && (
              <div className="grid grid-cols-2 gap-2">
                <div className="form-control">
                  <label className="label"><span className="label-text text-xs">Bundle ID</span></label>
                  <input type="text" value={form.bundle_id} onChange={(e) => handleBundleIdChange(e.target.value)}
                    className="input input-bordered input-xs font-mono" placeholder="com.example.app" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text text-xs">iTunes Store ID</span></label>
                  <input type="text" value={form.itunes_store_id} onChange={(e) => handleItunesIdChange(e.target.value)}
                    className="input input-bordered input-xs" placeholder="ID 或 URL" />
                </div>
              </div>
            )}

            {/* Enterprise: manual input */}
            {form.app_type === "enterprise" && (
              <>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">Bundle ID</span></label>
                  <input type="text" value={form.bundle_id} onChange={(e) => setForm({ ...form, bundle_id: e.target.value })}
                    className="input input-bordered input-sm font-mono" placeholder="com.example.app" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">Manifest URL</span></label>
                  <input type="text" value={form.manifest_url} onChange={(e) => setForm({ ...form, manifest_url: e.target.value })}
                    className="input input-bordered input-sm" placeholder="https://example.com/app/manifest.plist" />
                </div>
              </>
            )}

            <div className="form-control">
              <label className="label"><span className="label-text font-medium">App 名稱 <span className="text-error">*</span></span></label>
              <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input input-bordered input-sm" placeholder="搜尋後自動帶入或手動輸入" />
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">採購數量</span></label>
              <input type="number" min="0" value={form.purchased_qty} onChange={(e) => setForm({ ...form, purchased_qty: parseInt(e.target.value) || 0 })}
                className="input input-bordered input-sm w-32" />
              <label className="label"><span className="label-text-alt">VPP App 需先在 Apple Business Manager 採購對應數量</span></label>
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">備註</span></label>
              <input type="text" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })}
                className="input input-bordered input-sm" placeholder="選填" />
            </div>
          </div>
          <div className="modal-action">
            <button className="btn" onClick={() => setShowModal(false)}>取消</button>
            <button className="btn btn-primary gap-1" disabled={!form.name || saving} onClick={handleSave}>
              {saving ? <span className="loading loading-spinner loading-xs"></span> : <Save size={14} />}
              {editingId ? "儲存" : "新增"}
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setShowModal(false)}>close</button>
        </form>
      </dialog>
    </div>
  );
}
