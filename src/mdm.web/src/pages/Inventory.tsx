import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useDialog } from "../components/DialogProvider";
import {
  Plus, RefreshCw, Play, CheckCircle, Trash2, Download,
  ClipboardCheck, ChevronDown, ChevronRight, Search,
} from "lucide-react";
import apiClient from "../lib/apiClient";

interface InventorySession {
  id: string;
  name: string;
  description: string;
  status: string;
  creator_name: string;
  created_at: string;
  started_at: string | null;
  completed_at: string | null;
  notes: string;
  total_count: number;
  checked_count: number;
  matched_count: number;
  missing_count: number;
}

interface InventoryItem {
  id: string;
  session_id: string;
  asset_id: string;
  device_udid: string;
  asset_number: string;
  asset_name: string;
  found: boolean | null;
  condition: string;
  checker_name: string;
  checked_at: string | null;
  notes: string;
}

const STATUS_CONFIG: Record<string, { label: string; badge: string }> = {
  draft:       { label: "草稿",   badge: "badge-ghost" },
  in_progress: { label: "盤點中", badge: "badge-warning" },
  completed:   { label: "已完成", badge: "badge-success" },
};

export function Inventory() {
  const { t } = useTranslation();
  const dialog = useDialog();
  const [sessions, setSessions] = useState<InventorySession[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createDesc, setCreateDesc] = useState("");
  const [expandedSession, setExpandedSession] = useState<string | null>(null);
  const [items, setItems] = useState<InventoryItem[]>([]);
  const [itemsLoading, setItemsLoading] = useState(false);
  const [itemFilter, setItemFilter] = useState("");

  const loadSessions = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/inventory-sessions");
      setSessions(data.sessions || []);
    } catch { /* ignore */ }
    finally { setLoading(false); }
  };

  useEffect(() => { loadSessions(); }, []);

  const loadItems = async (sessionId: string) => {
    setItemsLoading(true);
    try {
      const { data } = await apiClient.get(`/api/inventory-sessions/${sessionId}`);
      setItems(data.items || []);
    } catch { /* ignore */ }
    finally { setItemsLoading(false); }
  };

  const toggleSession = (id: string) => {
    if (expandedSession === id) {
      setExpandedSession(null);
      setItems([]);
    } else {
      setExpandedSession(id);
      loadItems(id);
    }
  };

  const handleCreate = async () => {
    if (!createName.trim()) return;
    try {
      const { data } = await apiClient.post("/api/inventory-sessions", {
        name: createName.trim(), description: createDesc.trim(),
      });
      await dialog.success(`盤點已建立，包含 ${data.item_count} 項資產`);
      setShowCreate(false);
      setCreateName("");
      setCreateDesc("");
      loadSessions();
    } catch (err) {
      await dialog.error("建立失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const handleStart = async (id: string) => {
    try {
      await apiClient.post(`/api/inventory-sessions/${id}/start`);
      loadSessions();
      if (expandedSession === id) loadItems(id);
    } catch (err) {
      await dialog.error("操作失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const handleComplete = async (id: string) => {
    if (!await dialog.confirm("確定要完成此盤點？完成後無法再修改。")) return;
    try {
      await apiClient.post(`/api/inventory-sessions/${id}/complete`);
      loadSessions();
      if (expandedSession === id) loadItems(id);
    } catch (err) {
      await dialog.error("操作失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const handleDelete = async (id: string) => {
    if (!await dialog.confirm("確定要刪除此盤點？")) return;
    try {
      await apiClient.delete(`/api/inventory-sessions/${id}`);
      if (expandedSession === id) {
        setExpandedSession(null);
        setItems([]);
      }
      loadSessions();
    } catch (err) {
      await dialog.error("刪除失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const handleCheckItem = async (itemId: string, found: boolean) => {
    try {
      await apiClient.put(`/api/inventory-items/${itemId}`, {
        found,
        condition: found ? "good" : "",
        notes: "",
      });
      if (expandedSession) {
        loadItems(expandedSession);
        loadSessions();
      }
    } catch { /* ignore */ }
  };

  const handleExport = (sessionId: string) => {
    window.open(`/api/inventory-export/${sessionId}`, "_blank");
  };

  const filteredItems = itemFilter
    ? items.filter((i) =>
        i.asset_number.toLowerCase().includes(itemFilter.toLowerCase()) ||
        i.asset_name.toLowerCase().includes(itemFilter.toLowerCase()))
    : items;

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("inventory.title")}</h1>
          <p className="text-sm text-base-content/60">{t("inventory.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={loadSessions} className="btn btn-ghost btn-sm gap-1">
            <RefreshCw size={14} />{t("common.refresh")}
          </button>
          <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm gap-1">
            <Plus size={14} />{t("inventory.create")}
          </button>
        </div>
      </div>

      {/* Create dialog */}
      {showCreate && (
        <div className="card bg-base-100 shadow border border-primary/20">
          <div className="card-body p-4 space-y-3">
            <h3 className="font-semibold">{t("inventory.create")}</h3>
            <input
              type="text" placeholder={t("inventory.nameLabel")}
              className="input input-bordered input-sm w-full"
              value={createName} onChange={(e) => setCreateName(e.target.value)}
            />
            <input
              type="text" placeholder={t("inventory.descLabel")}
              className="input input-bordered input-sm w-full"
              value={createDesc} onChange={(e) => setCreateDesc(e.target.value)}
            />
            <div className="flex gap-2 justify-end">
              <button onClick={() => setShowCreate(false)} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
              <button onClick={handleCreate} disabled={!createName.trim()} className="btn btn-primary btn-sm">{t("common.confirm")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Sessions list */}
      {loading ? (
        <div className="text-center py-8"><span className="loading loading-spinner loading-md"></span></div>
      ) : sessions.length === 0 ? (
        <div className="card bg-base-100 shadow">
          <div className="card-body text-center text-base-content/50">
            <ClipboardCheck size={48} className="mx-auto mb-2 opacity-30" />
            <p>{t("inventory.noSessions")}</p>
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {sessions.map((s) => {
            const st = STATUS_CONFIG[s.status] || STATUS_CONFIG.draft;
            const isExpanded = expandedSession === s.id;
            const progress = s.total_count > 0 ? Math.round((s.checked_count / s.total_count) * 100) : 0;

            return (
              <div key={s.id} className="card bg-base-100 shadow">
                <div className="card-body p-4">
                  {/* Session header */}
                  <div className="flex items-center gap-3 cursor-pointer" onClick={() => toggleSession(s.id)}>
                    {isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold">{s.name}</span>
                        <span className={`badge badge-sm ${st.badge}`}>{st.label}</span>
                      </div>
                      {s.description && <p className="text-sm text-base-content/60 truncate">{s.description}</p>}
                    </div>
                    <div className="text-right text-sm">
                      <div className="font-mono">{s.checked_count}/{s.total_count}</div>
                      <div className="text-xs text-base-content/50">{s.creator_name}</div>
                    </div>
                  </div>

                  {/* Progress bar */}
                  <div className="flex items-center gap-2 mt-1">
                    <progress className="progress progress-primary w-full" value={progress} max="100"></progress>
                    <span className="text-xs font-mono w-10 text-right">{progress}%</span>
                  </div>

                  {/* Stats */}
                  <div className="flex gap-4 text-sm mt-1">
                    <span>{t("inventory.total")}: <b>{s.total_count}</b></span>
                    <span>{t("inventory.checked")}: <b>{s.checked_count}</b></span>
                    <span className="text-success">{t("inventory.matched")}: <b>{s.matched_count}</b></span>
                    <span className="text-error">{t("inventory.missing")}: <b>{s.missing_count}</b></span>
                  </div>

                  {/* Actions */}
                  <div className="flex gap-2 mt-2">
                    {s.status === "draft" && (
                      <button onClick={(e) => { e.stopPropagation(); handleStart(s.id); }} className="btn btn-success btn-xs gap-1">
                        <Play size={12} />{t("inventory.start")}
                      </button>
                    )}
                    {s.status === "in_progress" && (
                      <button onClick={(e) => { e.stopPropagation(); handleComplete(s.id); }} className="btn btn-info btn-xs gap-1">
                        <CheckCircle size={12} />{t("inventory.complete")}
                      </button>
                    )}
                    <button onClick={(e) => { e.stopPropagation(); handleExport(s.id); }} className="btn btn-outline btn-xs gap-1">
                      <Download size={12} />{t("inventory.export")}
                    </button>
                    {s.status !== "completed" && (
                      <button onClick={(e) => { e.stopPropagation(); handleDelete(s.id); }} className="btn btn-ghost btn-xs text-error gap-1">
                        <Trash2 size={12} />{t("common.delete")}
                      </button>
                    )}
                  </div>

                  {/* Items table */}
                  {isExpanded && (
                    <div className="mt-3 border-t border-base-300 pt-3">
                      <div className="flex items-center gap-2 mb-2">
                        <label className="input input-bordered input-xs flex items-center gap-1">
                          <Search size={12} className="opacity-50" />
                          <input
                            type="text" placeholder={t("inventory.searchItem")}
                            value={itemFilter} onChange={(e) => setItemFilter(e.target.value)}
                            className="grow w-32"
                          />
                        </label>
                      </div>
                      {itemsLoading ? (
                        <div className="text-center py-4"><span className="loading loading-spinner loading-sm"></span></div>
                      ) : (
                        <div className="overflow-x-auto">
                          <table className="table table-xs">
                            <thead>
                              <tr>
                                <th>{t("inventory.assetNumber")}</th>
                                <th>{t("inventory.assetName")}</th>
                                <th>{t("inventory.result")}</th>
                                <th>{t("inventory.checker")}</th>
                                <th>{t("inventory.checkedAt")}</th>
                                {s.status === "in_progress" && <th>{t("common.actions")}</th>}
                              </tr>
                            </thead>
                            <tbody>
                              {filteredItems.map((item) => (
                                <tr key={item.id} className="hover">
                                  <td className="font-mono text-xs">{item.asset_number || "-"}</td>
                                  <td>{item.asset_name}</td>
                                  <td>
                                    {item.found === null ? (
                                      <span className="badge badge-ghost badge-xs">{t("inventory.unchecked")}</span>
                                    ) : item.found ? (
                                      <span className="badge badge-success badge-xs">{t("inventory.found")}</span>
                                    ) : (
                                      <span className="badge badge-error badge-xs">{t("inventory.notFound")}</span>
                                    )}
                                  </td>
                                  <td className="text-xs">{item.checker_name || "-"}</td>
                                  <td className="text-xs opacity-70">
                                    {item.checked_at ? new Date(item.checked_at).toLocaleString() : "-"}
                                  </td>
                                  {s.status === "in_progress" && (
                                    <td>
                                      <div className="flex gap-1">
                                        <button
                                          onClick={() => handleCheckItem(item.id, true)}
                                          className="btn btn-success btn-xs"
                                          disabled={item.found === true}
                                        >
                                          {t("inventory.markFound")}
                                        </button>
                                        <button
                                          onClick={() => handleCheckItem(item.id, false)}
                                          className="btn btn-error btn-xs"
                                          disabled={item.found === false}
                                        >
                                          {t("inventory.markMissing")}
                                        </button>
                                      </div>
                                    </td>
                                  )}
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
