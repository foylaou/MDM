import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Save, Trash2, Edit3, X } from "lucide-react";
import apiClient from "../lib/apiClient";
import { CategoryPicker } from "./CategoryPicker";

interface Asset {
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
  borrow_date: string | null;
  custodian_id: string | null;
  custodian_name: string;
  location: string;
  asset_category: string;
  notes: string;
  category_id: string | null;
  category_name: string;
}

interface UserOption {
  id: string;
  username: string;
  display_name: string;
}

const emptyAsset: Omit<Asset, "id"> = {
  device_udid: null,
  asset_number: "", name: "", spec: "", quantity: 1, unit: "台",
  acquired_date: null, unit_price: 0, purpose: "",
  borrow_date: null, custodian_id: null, custodian_name: "",
  location: "", asset_category: "", notes: "",
  category_id: null, category_name: "",
};

interface AssetFormProps {
  deviceUdid: string;
}

export function AssetForm({ deviceUdid }: AssetFormProps) {
  const { t } = useTranslation();
  const [assets, setAssets] = useState<Asset[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState(emptyAsset);
  const [saving, setSaving] = useState(false);

  const loadAssets = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get(`/api/assets?device_udid=${deviceUdid}`);
      setAssets(data.assets || []);
    } catch (err) { console.error("Load assets:", err); }
    finally { setLoading(false); }
  };

  const loadUsers = async () => {
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch { /* */ }
  };

  useEffect(() => { loadAssets(); loadUsers(); }, [deviceUdid]);

  const startEdit = (asset?: Asset) => {
    if (asset) {
      setForm({ ...asset, device_udid: deviceUdid });
      setEditing(asset.id);
    } else {
      setForm({ ...emptyAsset, device_udid: deviceUdid });
      setEditing("new");
    }
  };

  const cancelEdit = () => { setEditing(null); };

  const handleCustodianChange = (userId: string) => {
    const user = users.find((u) => u.id === userId);
    setForm((prev) => ({
      ...prev,
      custodian_id: userId || null,
      custodian_name: user ? (user.display_name || user.username) : "",
    }));
  };

  const saveAsset = async () => {
    setSaving(true);
    try {
      if (editing === "new") {
        await apiClient.post("/api/assets", form);
      } else {
        await apiClient.put(`/api/assets/${editing}`, form);
      }
      setEditing(null);
      loadAssets();
    } catch (err) {
      alert("儲存失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setSaving(false); }
  };

  const deleteAsset = async (id: string) => {
    if (!confirm("確定要刪除此財產資訊？")) return;
    try {
      await apiClient.delete(`/api/assets/${id}`);
      loadAssets();
    } catch { alert("刪除失敗"); }
  };

  const updateField = (key: string, value: unknown) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  if (loading) {
    return <div className="flex justify-center py-8"><span className="loading loading-spinner loading-md"></span></div>;
  }

  // Editing form
  if (editing) {
    const textFields: { key: string; label: string; type?: string }[] = [
      { key: "asset_number", label: t("assets.assetNumber") },
      { key: "name", label: t("assets.name") },
      { key: "spec", label: t("assets.spec") },
      { key: "quantity", label: t("assets.quantity"), type: "number" },
      { key: "unit", label: t("assets.unit") },
      { key: "acquired_date", label: t("assets.acquiredDate"), type: "date" },
      { key: "unit_price", label: t("assets.unitPrice"), type: "number" },
      { key: "purpose", label: t("assets.purpose") },
      { key: "borrow_date", label: t("assets.borrowDate"), type: "date" },
      { key: "location", label: t("assets.location") },
      { key: "asset_category", label: t("assets.category") },
      { key: "notes", label: t("assets.notes") },
    ];

    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold">{editing === "new" ? t("assets.add") : t("assets.edit")}</h3>
          <button onClick={cancelEdit} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {textFields.map((f) => (
            <div key={f.key} className="form-control">
              <label className="label py-1"><span className="label-text text-xs">{f.label}</span></label>
              <input
                type={f.type || "text"}
                value={(form as Record<string, unknown>)[f.key] as string ?? ""}
                onChange={(e) => updateField(f.key, f.type === "number" ? Number(e.target.value) : e.target.value)}
                className="input input-bordered input-sm"
              />
            </div>
          ))}
          {/* Custodian — user dropdown */}
          <div className="form-control">
            <label className="label py-1"><span className="label-text text-xs">{t("assets.custodian")}</span></label>
            <select
              value={form.custodian_id || ""}
              onChange={(e) => handleCustodianChange(e.target.value)}
              className="select select-bordered select-sm"
            >
              <option value="">-- 無 --</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
              ))}
            </select>
          </div>
          {/* Category — cascading picker */}
          <div className="form-control sm:col-span-2">
            <label className="label py-1"><span className="label-text text-xs">裝置分類</span></label>
            <CategoryPicker
              value={form.category_id}
              onChange={(catId, _path) => setForm((prev) => ({ ...prev, category_id: catId }))}
            />
          </div>
        </div>
        <div className="flex gap-2">
          <button onClick={saveAsset} disabled={saving} className="btn btn-success btn-sm gap-1">
            {saving ? <span className="loading loading-spinner loading-xs"></span> : <Save size={14} />}
            {t("common.save")}
          </button>
          <button onClick={cancelEdit} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
        </div>
      </div>
    );
  }

  // Display mode
  if (assets.length === 0) {
    return (
      <div className="text-center py-6">
        <p className="text-base-content/50 mb-3">{t("assets.noAsset")}</p>
        <button onClick={() => startEdit()} className="btn btn-primary btn-sm gap-1">
          <Plus size={14} /> {t("assets.add")}
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <button onClick={() => startEdit()} className="btn btn-primary btn-sm gap-1">
          <Plus size={14} /> {t("assets.add")}
        </button>
      </div>
      {assets.map((asset) => (
        <div key={asset.id} className="border border-base-300 rounded-lg p-3">
          <div className="flex items-start justify-between">
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-1 text-sm flex-1">
              {[
                [t("assets.assetNumber"), asset.asset_number],
                [t("assets.name"), asset.name],
                [t("assets.spec"), asset.spec],
                [t("assets.quantity"), `${asset.quantity} ${asset.unit}`],
                [t("assets.acquiredDate"), asset.acquired_date || "-"],
                [t("assets.unitPrice"), asset.unit_price ? `$${asset.unit_price.toLocaleString()}` : "-"],
                [t("assets.purpose"), asset.purpose],
                [t("assets.borrowDate"), asset.borrow_date || "-"],
                [t("assets.custodian"), asset.custodian_name || "-"],
                [t("assets.location"), asset.location],
                ["裝置分類", asset.category_name],
                [t("assets.category"), asset.asset_category],
                [t("assets.notes"), asset.notes],
              ].filter(([, v]) => v && v !== "-").map(([label, value]) => (
                <div key={label}>
                  <span className="text-base-content/50">{label}：</span>
                  <span className="font-medium">{value}</span>
                </div>
              ))}
            </div>
            <div className="flex gap-1 ml-2">
              <button onClick={() => startEdit(asset)} className="btn btn-ghost btn-xs"><Edit3 size={14} /></button>
              <button onClick={() => deleteAsset(asset.id)} className="btn btn-ghost btn-xs text-error"><Trash2 size={14} /></button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
