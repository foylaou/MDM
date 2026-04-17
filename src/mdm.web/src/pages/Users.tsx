import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { UserPlus, Trash2, Edit3, X, Save, ShieldCheck, ShieldOff, ChevronDown, ChevronUp } from "lucide-react";
import apiClient from "../lib/apiClient";
import { useAuthStore } from "../stores/authStore";
import { useDialog } from "../components/DialogProvider";

interface UserRow {
  id: string;
  username: string;
  display_name: string;
  role: string;
  system_role: string;
  is_active: boolean;
  permissions: Record<string, string>;
}

const modules = ["asset", "mdm", "rental"] as const;
const moduleLabels: Record<string, string> = { asset: "財產管理", mdm: "裝置管理", rental: "租借系統" };
const levelOptions: Record<string, string[]> = {
  asset: ["none", "viewer", "operator", "manager"],
  mdm: ["none", "viewer", "operator", "manager"],
  rental: ["none", "requester", "approver", "manager"],
};
const levelLabels: Record<string, string> = {
  none: "無權限", viewer: "檢視者", requester: "申請者",
  operator: "操作員", approver: "核准者", manager: "管理者",
};
const levelBadge: Record<string, string> = {
  none: "", viewer: "badge-ghost", requester: "badge-info",
  operator: "badge-accent", approver: "badge-warning", manager: "badge-primary",
};

export function Users() {
  const { t } = useTranslation();
  const dialog = useDialog();
  const { clients } = useAuthStore();
  const [users, setUsers] = useState<UserRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [form, setForm] = useState({ username: "", password: "", role: "viewer", displayName: "" });
  const [saving, setSaving] = useState(false);

  // Inline edit state
  const [editForm, setEditForm] = useState({ display_name: "", password: "" });
  const [permForm, setPermForm] = useState<Record<string, string>>({});
  const [editSaving, setEditSaving] = useState(false);

  const loadUsers = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch (err) { console.error("Failed to load users:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadUsers(); }, []);

  const handleCreate = async () => {
    if (!clients) return;
    setSaving(true);
    try {
      await clients.user.createUser(form);
      setShowCreate(false);
      setForm({ username: "", password: "", role: "viewer", displayName: "" });
      loadUsers();
    } catch (err) {
      await dialog.error(t("users.createFailed") + ": " + (err instanceof Error ? err.message : ""));
    } finally { setSaving(false); }
  };

  const toggleExpand = (u: UserRow) => {
    if (expandedId === u.id) {
      setExpandedId(null);
    } else {
      setExpandedId(u.id);
      setEditForm({ display_name: u.display_name, password: "" });
      const perms: Record<string, string> = {};
      for (const m of modules) perms[m] = u.permissions[m] || "none";
      setPermForm(perms);
    }
  };

  const handleSaveUser = async (userId: string) => {
    setEditSaving(true);
    try {
      // Save user fields
      const userBody: Record<string, unknown> = {};
      if (editForm.display_name) userBody.display_name = editForm.display_name;
      if (editForm.password) userBody.password = editForm.password;
      if (Object.keys(userBody).length > 0) {
        await apiClient.put(`/api/users/${userId}`, userBody);
      }

      // Save permissions
      await apiClient.put(`/api/users-permissions/${userId}`, permForm);

      setExpandedId(null);
      loadUsers();
    } catch (err) {
      await dialog.error("儲存失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setEditSaving(false); }
  };

  const toggleActive = async (id: string, currentlyActive: boolean) => {
    try {
      await apiClient.put(`/api/users/${id}`, { is_active: !currentlyActive });
      loadUsers();
    } catch { await dialog.error("操作失敗"); }
  };

  const handleDelete = async (id: string, username: string) => {
    if (!(await dialog.confirm(t("users.deleteConfirm", { name: username })))) return;
    try {
      await apiClient.delete(`/api/users/${id}`);
      loadUsers();
    } catch (err) { await dialog.error(t("users.deleteFailed") + ": " + (err instanceof Error ? err.message : "")); }
  };

  const toggleSysAdmin = async (u: UserRow) => {
    const newRole = u.system_role === "sys_admin" ? "user" : "sys_admin";
    const msg = newRole === "sys_admin"
      ? `確定要將「${u.display_name || u.username}」設為系統管理員？`
      : `確定要取消「${u.display_name || u.username}」的系統管理員身份？`;
    if (!(await dialog.confirm(msg))) return;
    try {
      await apiClient.put(`/api/users/${u.id}`, { system_role: newRole });
      loadUsers();
    } catch (err) {
      await dialog.error("操作失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const permBadges = (perms: Record<string, string>) => {
    const entries = modules
      .map((m) => ({ module: m, level: perms[m] || "none" }))
      .filter((e) => e.level !== "none");
    if (entries.length === 0) return <span className="text-base-content/30 text-xs">無模組權限</span>;
    return (
      <div className="flex flex-wrap gap-1">
        {entries.map((e) => (
          <span key={e.module} className={`badge badge-sm ${levelBadge[e.level] || "badge-ghost"}`}>
            {moduleLabels[e.module]}：{levelLabels[e.level]}
          </span>
        ))}
      </div>
    );
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("users.title")}</h1>
          <p className="text-sm text-base-content/60">{t("users.subtitle")}</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm gap-1">
          <UserPlus size={16} />{t("users.createUser")}
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <h2 className="card-title text-base">{t("users.newUser")}</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-2">
              <div className="form-control">
                <label className="label"><span className="label-text">{t("users.username")}</span></label>
                <input type="text" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="input input-bordered input-sm" />
              </div>
              <div className="form-control">
                <label className="label"><span className="label-text">{t("users.password")}</span></label>
                <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} className="input input-bordered input-sm" />
              </div>
              <div className="form-control">
                <label className="label"><span className="label-text">{t("users.displayName")}</span></label>
                <input type="text" value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} className="input input-bordered input-sm" />
              </div>
              <div className="form-control">
                <label className="label"><span className="label-text">{t("users.role")}</span></label>
                <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })} className="select select-bordered select-sm">
                  <option value="viewer">{t("users.roles.viewer")}</option>
                  <option value="operator">{t("users.roles.operator")}</option>
                  <option value="admin">{t("users.roles.admin")}</option>
                </select>
              </div>
            </div>
            <div className="card-actions mt-4">
              <button onClick={handleCreate} disabled={saving} className="btn btn-success btn-sm gap-1">
                {saving && <span className="loading loading-spinner loading-xs"></span>}
                {t("common.create")}
              </button>
              <button onClick={() => setShowCreate(false)} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
            </div>
          </div>
        </div>
      )}

      {/* User list */}
      {loading ? (
        <div className="flex justify-center py-12"><span className="loading loading-spinner loading-lg"></span></div>
      ) : users.length === 0 ? (
        <div className="text-center py-12 text-base-content/50">{t("users.noUsers")}</div>
      ) : (
        <div className="space-y-2">
          {users.map((u) => {
            const isSysAdmin = u.system_role === "sys_admin";
            const isExpanded = expandedId === u.id;
            return (
              <div key={u.id} className={`card bg-base-100 shadow-sm ${!u.is_active ? "opacity-50" : ""}`}>
                <div className="card-body p-4">
                  {/* Summary row */}
                  <div className="flex items-center gap-3 cursor-pointer" onClick={() => toggleExpand(u)}>
                    {/* Avatar */}
                    <div className="avatar placeholder">
                      <div className={`w-10 rounded-full ${isSysAdmin ? "bg-primary text-primary-content" : "bg-base-300 text-base-content"}`}>
                        <span className="text-sm font-bold">{(u.display_name || u.username)[0].toUpperCase()}</span>
                      </div>
                    </div>

                    {/* Info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold">{u.display_name || u.username}</span>
                        {u.display_name && <span className="text-xs text-base-content/50">@{u.username}</span>}
                        {isSysAdmin && <span className="badge badge-primary badge-sm">系統管理員</span>}
                        {!u.is_active && <span className="badge badge-error badge-sm">已停用</span>}
                      </div>
                      <div className="mt-1">
                        {isSysAdmin
                          ? <span className="text-xs text-primary">擁有所有模組完整權限</span>
                          : permBadges(u.permissions)
                        }
                      </div>
                    </div>

                    {/* Quick actions */}
                    <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                      <button onClick={() => toggleActive(u.id, u.is_active)}
                        className="btn btn-ghost btn-xs" title={u.is_active ? "停用" : "啟用"}>
                        {u.is_active ? <ShieldCheck size={14} className="text-success" /> : <ShieldOff size={14} className="text-error" />}
                      </button>
                      <button onClick={() => handleDelete(u.id, u.username)} className="btn btn-ghost btn-xs text-error" title="刪除">
                        <Trash2 size={14} />
                      </button>
                      {isExpanded ? <ChevronUp size={16} className="opacity-40" /> : <ChevronDown size={16} className="opacity-40" />}
                    </div>
                  </div>

                  {/* Expanded edit panel */}
                  {isExpanded && (
                    <div className="border-t border-base-300 mt-3 pt-4 space-y-4">
                      {/* Basic info */}
                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                        <div className="form-control">
                          <label className="label py-1"><span className="label-text text-xs">{t("users.displayName")}</span></label>
                          <input type="text" value={editForm.display_name}
                            onChange={(e) => setEditForm({ ...editForm, display_name: e.target.value })}
                            className="input input-bordered input-sm" />
                        </div>
                        <div className="form-control">
                          <label className="label py-1"><span className="label-text text-xs">新密碼（留空不修改）</span></label>
                          <input type="password" value={editForm.password}
                            onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
                            className="input input-bordered input-sm" placeholder="••••••" />
                        </div>
                        <div className="form-control">
                          <label className="label py-1"><span className="label-text text-xs">系統角色</span></label>
                          <button onClick={() => toggleSysAdmin(u)}
                            className={`btn btn-sm ${isSysAdmin ? "btn-primary" : "btn-outline"}`}>
                            {isSysAdmin ? "系統管理員（點擊取消）" : "一般使用者（點擊升為管理員）"}
                          </button>
                        </div>
                      </div>

                      {/* Module permissions */}
                      {!isSysAdmin && (
                        <div>
                          <h4 className="font-medium text-sm mb-2">模組權限</h4>
                          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                            {modules.map((m) => (
                              <div key={m} className="form-control">
                                <label className="label py-1"><span className="label-text text-xs">{moduleLabels[m]}</span></label>
                                <div className="flex gap-1 flex-wrap">
                                  {levelOptions[m].map((level) => (
                                    <button key={level}
                                      onClick={() => setPermForm({ ...permForm, [m]: level })}
                                      className={`btn btn-xs ${permForm[m] === level
                                        ? (level === "none" ? "btn-ghost btn-active" : "btn-primary")
                                        : "btn-ghost"
                                      }`}
                                    >
                                      {levelLabels[level]}
                                    </button>
                                  ))}
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                      {isSysAdmin && (
                        <div className="alert alert-info py-2">
                          <span className="text-sm">系統管理員自動擁有所有模組的完整權限，無需個別設定。</span>
                        </div>
                      )}

                      {/* Save / Cancel */}
                      <div className="flex gap-2">
                        <button onClick={() => handleSaveUser(u.id)} disabled={editSaving} className="btn btn-success btn-sm gap-1">
                          {editSaving ? <span className="loading loading-spinner loading-xs"></span> : <Save size={14} />}
                          儲存變更
                        </button>
                        <button onClick={() => setExpandedId(null)} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
                      </div>
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
