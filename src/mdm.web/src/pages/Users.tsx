import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { UserPlus, Trash2, Edit3, X, Save, ShieldCheck, ShieldOff } from "lucide-react";
import apiClient from "../lib/apiClient";
import { useAuthStore } from "../stores/authStore";
import { useDialog } from "../components/DialogProvider";

interface UserRow {
  id: string;
  username: string;
  display_name: string;
  role: string;
  is_active: boolean;
}

export function Users() {
  const { t } = useTranslation();
  const dialog = useDialog();
  const { clients } = useAuthStore();
  const [users, setUsers] = useState<UserRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState({ username: "", password: "", role: "viewer", displayName: "" });
  const [editForm, setEditForm] = useState({ role: "", display_name: "", password: "" });
  const [saving, setSaving] = useState(false);

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

  const startEdit = (u: UserRow) => {
    setEditingId(u.id);
    setEditForm({ role: u.role, display_name: u.display_name, password: "" });
  };

  const handleUpdate = async () => {
    if (!editingId) return;
    setSaving(true);
    try {
      const body: Record<string, unknown> = {};
      if (editForm.role) body.role = editForm.role;
      if (editForm.display_name) body.display_name = editForm.display_name;
      if (editForm.password) body.password = editForm.password;
      await apiClient.put(`/api/users/${editingId}`, body);
      setEditingId(null);
      loadUsers();
    } catch (err) {
      await dialog.error("更新失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setSaving(false); }
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

  const roleBadge = (role: string) => {
    switch (role) {
      case "admin": return "badge-primary";
      case "operator": return "badge-accent";
      default: return "badge-ghost";
    }
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

      <div className="card bg-base-100 shadow">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t("users.username")}</th>
                <th>{t("users.displayName")}</th>
                <th>{t("users.role")}</th>
                <th>{t("common.status")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : users.length === 0 ? (
                <tr><td colSpan={5} className="text-center py-8 text-base-content/50">{t("users.noUsers")}</td></tr>
              ) : users.map((u) => (
                <tr key={u.id} className={`hover ${!u.is_active ? "opacity-50" : ""}`}>
                  <td className="font-medium">{u.username}</td>
                  <td>
                    {editingId === u.id ? (
                      <input type="text" value={editForm.display_name} onChange={(e) => setEditForm({ ...editForm, display_name: e.target.value })}
                        className="input input-bordered input-xs w-32" />
                    ) : (u.display_name || "-")}
                  </td>
                  <td>
                    {editingId === u.id ? (
                      <select value={editForm.role} onChange={(e) => setEditForm({ ...editForm, role: e.target.value })} className="select select-bordered select-xs">
                        <option value="viewer">{t("users.roles.viewer")}</option>
                        <option value="operator">{t("users.roles.operator")}</option>
                        <option value="admin">{t("users.roles.admin")}</option>
                      </select>
                    ) : (
                      <span className={`badge badge-sm ${roleBadge(u.role)}`}>{t(`users.roles.${u.role}`)}</span>
                    )}
                  </td>
                  <td>
                    <button onClick={() => toggleActive(u.id, u.is_active)}>
                      {u.is_active ? (
                        <span className="badge badge-success badge-sm gap-1"><ShieldCheck size={10} /> 啟用</span>
                      ) : (
                        <span className="badge badge-error badge-sm gap-1"><ShieldOff size={10} /> 停用</span>
                      )}
                    </button>
                  </td>
                  <td>
                    {editingId === u.id ? (
                      <div className="flex gap-1 items-center">
                        <input type="password" placeholder="新密碼(選填)" value={editForm.password}
                          onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
                          className="input input-bordered input-xs w-28" />
                        <button onClick={handleUpdate} disabled={saving} className="btn btn-success btn-xs gap-1"><Save size={12} /></button>
                        <button onClick={() => setEditingId(null)} className="btn btn-ghost btn-xs"><X size={12} /></button>
                      </div>
                    ) : (
                      <div className="flex gap-1">
                        <button onClick={() => startEdit(u)} className="btn btn-ghost btn-xs"><Edit3 size={14} /></button>
                        <button onClick={() => handleDelete(u.id, u.username)} className="btn btn-ghost btn-xs text-error"><Trash2 size={14} /></button>
                      </div>
                    )}
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
