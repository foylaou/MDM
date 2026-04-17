import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Save, Trash2, Edit3, X, ArrowRightLeft } from "lucide-react";
import apiClient from "../lib/apiClient";
import { useDialog } from "./DialogProvider";
import { CategoryPicker } from "./CategoryPicker";
import { CustodyTransferDialog } from "./CustodyTransferDialog";
import { CustodyHistory } from "./CustodyHistory";

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
  assigned_date: string | null;
  custodian_id: string | null;
  custodian_name: string;
  current_holder_id: string | null;
  current_holder_name: string;
  current_holder_since: string | null;
  location: string;
  asset_category: string;
  notes: string;
  category_id: string | null;
  category_name: string;
  asset_status: string;
}

const emptyAsset: Omit<Asset, "id"> = {
  device_udid: null,
  asset_number: "", name: "", spec: "", quantity: 1, unit: "台",
  acquired_date: null, unit_price: 0, purpose: "",
  assigned_date: null, custodian_id: null, custodian_name: "",
  current_holder_id: null, current_holder_name: "", current_holder_since: null,
  location: "", asset_category: "", notes: "",
  category_id: null, category_name: "",
  asset_status: "available",
};

const ASSET_STATUS_OPTIONS = [
  { value: "available",    label: "可用",   badge: "badge-success" },
  { value: "faulty",       label: "故障",   badge: "badge-error" },
  { value: "repairing",    label: "維修中", badge: "badge-info" },
  { value: "retired",      label: "報廢",   badge: "badge-ghost" },
  { value: "transferred",  label: "移撥",   badge: "badge-ghost" },
];

interface AssetFormProps {
  deviceUdid?: string;
  assetId?: string;
  onSaved?: () => void;
  onCancel?: () => void;
}

export function AssetForm({ deviceUdid, assetId, onSaved, onCancel }: AssetFormProps) {
  const isStandalone = !!(onSaved && onCancel);

  if (isStandalone) {
    return <StandaloneAssetForm assetId={assetId} deviceUdid={deviceUdid} onSaved={onSaved} onCancel={onCancel} />;
  }

  return <DeviceAssetList deviceUdid={deviceUdid || ""} />;
}

// ─── Standalone edit form (used by AssetList page) ───────────────────────────

function StandaloneAssetForm({
  assetId, deviceUdid, onSaved, onCancel,
}: {
  assetId?: string; deviceUdid?: string; onSaved: () => void; onCancel: () => void;
}) {
  const { t } = useTranslation();
  const dialog = useDialog();
  const [form, setForm] = useState<Omit<Asset, "id">>({ ...emptyAsset, device_udid: deviceUdid || null });
  const [saving, setSaving] = useState(false);
  const [loadingAsset, setLoadingAsset] = useState(!!assetId);
  const [transferOpen, setTransferOpen] = useState(false);
  const [historyRefresh, setHistoryRefresh] = useState(0);

  const loadAsset = () => {
    if (!assetId) return;
    setLoadingAsset(true);
    apiClient.get(`/api/assets?device_udid=`).then(({ data }) => {
      const assets: Asset[] = data.assets || [];
      const found = assets.find((a) => a.id === assetId);
      if (found) {
        setForm({ ...found, device_udid: found.device_udid ?? null });
      }
    }).catch(() => {}).finally(() => setLoadingAsset(false));
  };

  useEffect(loadAsset, [assetId]);

  const saveAsset = async () => {
    setSaving(true);
    try {
      if (assetId) {
        await apiClient.put(`/api/assets/${assetId}`, form);
      } else {
        await apiClient.post("/api/assets", form);
      }
      onSaved();
    } catch (err) {
      await dialog.error("儲存失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setSaving(false); }
  };

  const updateField = (key: string, value: unknown) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  if (loadingAsset) {
    return <div className="flex justify-center py-8"><span className="loading loading-spinner loading-md"></span></div>;
  }

  return (
    <>
      <AssetEditForm
        form={form} saving={saving}
        isRented={!!form.current_holder_id}
        title={assetId ? t("assets.edit") : t("assets.add")}
        updateField={updateField}
        setForm={setForm}
        onSave={saveAsset}
        onCancel={onCancel}
        onRequestTransfer={assetId ? () => setTransferOpen(true) : undefined}
      />
      {assetId && (
        <div className="mt-6 border-t border-base-300 pt-4">
          <CustodyHistory assetId={assetId} refreshKey={historyRefresh} />
        </div>
      )}
      {assetId && (
        <CustodyTransferDialog
          open={transferOpen}
          assetId={assetId}
          assetName={form.name}
          assetNumber={form.asset_number}
          currentCustodianId={form.custodian_id}
          currentCustodianName={form.custodian_name}
          onClose={() => setTransferOpen(false)}
          onDone={() => { loadAsset(); setHistoryRefresh((n) => n + 1); }}
        />
      )}
    </>
  );
}

// ─── Device-embedded list mode (used by DeviceDetail page) ───────────────────

function DeviceAssetList({ deviceUdid }: { deviceUdid: string }) {
  const { t } = useTranslation();
  const dialog = useDialog();
  const [assets, setAssets] = useState<Asset[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState(emptyAsset);
  const [saving, setSaving] = useState(false);
  const [transferAssetId, setTransferAssetId] = useState<string | null>(null);
  const [historyRefresh, setHistoryRefresh] = useState(0);

  const loadAssets = async () => {
    setLoading(true);
    try {
      const params = deviceUdid ? `?device_udid=${deviceUdid}` : "";
      const { data } = await apiClient.get(`/api/assets${params}`);
      setAssets(data.assets || []);
    } catch (err) { console.error("Load assets:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadAssets(); }, [deviceUdid]);

  const startEdit = (asset?: Asset) => {
    if (asset) {
      setForm({ ...asset, device_udid: deviceUdid || null });
      setEditing(asset.id);
    } else {
      setForm({ ...emptyAsset, device_udid: deviceUdid || null });
      setEditing("new");
    }
  };

  const cancelEdit = () => { setEditing(null); };

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
      await dialog.error("儲存失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setSaving(false); }
  };

  const deleteAsset = async (id: string) => {
    if (!(await dialog.confirm("確定要刪除此財產資訊？"))) return;
    try {
      await apiClient.delete(`/api/assets/${id}`);
      loadAssets();
    } catch { await dialog.error("刪除失敗"); }
  };

  const disposeAsset = async (id: string) => {
    const reason = window.prompt("請輸入報廢原因：");
    if (reason === null) return;
    try {
      await apiClient.post("/api/assets-lifecycle", { action: "dispose", asset_id: id, reason });
      await dialog.success("已報廢");
      loadAssets();
    } catch (err) {
      await dialog.error("報廢失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const transferAsset = async (id: string) => {
    const target = window.prompt("請輸入移撥對象（單位/部門）：");
    if (!target) return;
    try {
      await apiClient.post("/api/assets-lifecycle", { action: "transfer", asset_id: id, transferred_to: target });
      await dialog.success("已移撥");
      loadAssets();
    } catch (err) {
      await dialog.error("移撥失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const updateField = (key: string, value: unknown) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  if (loading) {
    return <div className="flex justify-center py-8"><span className="loading loading-spinner loading-md"></span></div>;
  }

  // Editing form
  if (editing) {
    const editingAsset = editing === "new" ? null : assets.find((a) => a.id === editing);
    return (
      <>
        <AssetEditForm
          form={form} saving={saving}
          isRented={!!form.current_holder_id}
          title={editing === "new" ? t("assets.add") : t("assets.edit")}
          updateField={updateField}
          setForm={setForm}
          onSave={saveAsset}
          onCancel={cancelEdit}
          onRequestTransfer={editingAsset ? () => setTransferAssetId(editingAsset.id) : undefined}
        />
        {editingAsset && (
          <div className="mt-6 border-t border-base-300 pt-4">
            <CustodyHistory assetId={editingAsset.id} refreshKey={historyRefresh} />
          </div>
        )}
        {editingAsset && (
          <CustodyTransferDialog
            open={transferAssetId === editingAsset.id}
            assetId={editingAsset.id}
            assetName={form.name}
            assetNumber={form.asset_number}
            currentCustodianId={form.custodian_id}
            currentCustodianName={form.custodian_name}
            onClose={() => setTransferAssetId(null)}
            onDone={() => { loadAssets(); setHistoryRefresh((n) => n + 1); }}
          />
        )}
      </>
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
              {(() => {
                const st = ASSET_STATUS_OPTIONS.find((s) => s.value === asset.asset_status) || ASSET_STATUS_OPTIONS[0];
                return (
                  <div>
                    <span className="text-base-content/50">裝置狀態：</span>
                    <span className={`badge badge-sm ${st.badge}`}>{st.label}</span>
                  </div>
                );
              })()}
              {[
                [t("assets.assetNumber"), asset.asset_number],
                [t("assets.name"), asset.name],
                [t("assets.spec"), asset.spec],
                [t("assets.quantity"), `${asset.quantity} ${asset.unit}`],
                [t("assets.acquiredDate"), asset.acquired_date || "-"],
                [t("assets.unitPrice"), asset.unit_price ? `$${asset.unit_price.toLocaleString()}` : "-"],
                [t("assets.purpose"), asset.purpose],
                [t("assets.assignedDate"), asset.assigned_date || "-"],
                [t("assets.custodian"), asset.custodian_name || "-"],
                [t("assets.currentHolder"), asset.current_holder_name || "-"],
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
            <div className="flex flex-col gap-1 ml-2">
              <button onClick={() => startEdit(asset)} className="btn btn-ghost btn-xs"><Edit3 size={14} /></button>
              {asset.asset_status !== "retired" && asset.asset_status !== "transferred" && (
                <>
                  <button onClick={() => disposeAsset(asset.id)} className="btn btn-ghost btn-xs text-warning" title="報廢">報廢</button>
                  <button onClick={() => transferAsset(asset.id)} className="btn btn-ghost btn-xs text-info" title="移撥">移撥</button>
                </>
              )}
              <button onClick={() => deleteAsset(asset.id)} className="btn btn-ghost btn-xs text-error"><Trash2 size={14} /></button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ─── Shared edit form UI ─────────────────────────────────────────────────────

function AssetEditForm({
  form, saving, isRented, title,
  updateField, setForm,
  onSave, onCancel, onRequestTransfer,
}: {
  form: Omit<Asset, "id">;
  saving: boolean;
  isRented: boolean;
  title: string;
  updateField: (key: string, value: unknown) => void;
  setForm: React.Dispatch<React.SetStateAction<Omit<Asset, "id">>>;
  onSave: () => void;
  onCancel: () => void;
  onRequestTransfer?: () => void;
}) {
  const { t } = useTranslation();

  const textFields: { key: string; label: string; type?: string }[] = [
    { key: "asset_number", label: t("assets.assetNumber") },
    { key: "name", label: t("assets.name") },
    { key: "spec", label: t("assets.spec") },
    { key: "quantity", label: t("assets.quantity"), type: "number" },
    { key: "unit", label: t("assets.unit") },
    { key: "acquired_date", label: t("assets.acquiredDate"), type: "date" },
    { key: "unit_price", label: t("assets.unitPrice"), type: "number" },
    { key: "purpose", label: t("assets.purpose") },
    { key: "location", label: t("assets.location") },
    { key: "asset_category", label: t("assets.category") },
    { key: "notes", label: t("assets.notes") },
  ];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold">{title}</h3>
        <button onClick={onCancel} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
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

        {/* Custodian — read-only + transfer button */}
        <div className="form-control">
          <label className="label py-1"><span className="label-text text-xs">{t("assets.custodian")}</span></label>
          <div className="flex gap-2">
            <div className="input input-bordered input-sm flex-1 flex items-center">
              {form.custodian_name || <span className="opacity-40">{t("custody.none")}</span>}
            </div>
            {onRequestTransfer && (
              <button
                type="button"
                onClick={onRequestTransfer}
                disabled={isRented}
                title={isRented ? t("custody.blockedRented") : undefined}
                className="btn btn-sm btn-outline gap-1"
              >
                <ArrowRightLeft size={14} />
                {form.custodian_id ? t("custody.transfer") : t("custody.assign")}
              </button>
            )}
          </div>
          {isRented && <label className="label py-0"><span className="label-text-alt text-warning">{t("custody.blockedRented")}</span></label>}
        </div>

        {/* Assigned date — informational, auto-filled by custody API but still editable */}
        <div className="form-control">
          <label className="label py-1"><span className="label-text text-xs">{t("assets.assignedDate")}</span></label>
          <div className="input input-bordered input-sm flex items-center">
            {form.assigned_date || <span className="opacity-40">-</span>}
          </div>
        </div>

        {/* Current holder — read-only, managed by rental system */}
        <div className="form-control">
          <label className="label py-1"><span className="label-text text-xs">{t("assets.currentHolder")}</span></label>
          <div className="input input-bordered input-sm flex items-center">
            {form.current_holder_name ? (
              <span>
                {form.current_holder_name}
                {form.current_holder_since && (
                  <span className="opacity-50 ml-1 text-xs">
                    ({new Date(form.current_holder_since).toLocaleDateString()})
                  </span>
                )}
              </span>
            ) : (
              <span className="opacity-40">{t("custody.holderNone")}</span>
            )}
          </div>
        </div>

        {/* Asset status */}
        <div className="form-control">
          <label className="label py-1"><span className="label-text text-xs">裝置狀態</span></label>
          <select
            value={form.asset_status || "available"}
            onChange={(e) => updateField("asset_status", e.target.value)}
            className="select select-bordered select-sm"
          >
            {ASSET_STATUS_OPTIONS.map((s) => (
              <option key={s.value} value={s.value}>{s.label}</option>
            ))}
          </select>
        </div>
        {/* Category — cascading picker */}
        <div className="form-control">
          <label className="label py-1"><span className="label-text text-xs">裝置分類</span></label>
          <CategoryPicker
            value={form.category_id}
            onChange={(catId, _path) => setForm((prev) => ({ ...prev, category_id: catId }))}
          />
        </div>
      </div>
      <div className="flex gap-2">
        <button onClick={onSave} disabled={saving} className="btn btn-success btn-sm gap-1">
          {saving ? <span className="loading loading-spinner loading-xs"></span> : <Save size={14} />}
          {t("common.save")}
        </button>
        <button onClick={onCancel} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
      </div>
    </div>
  );
}
