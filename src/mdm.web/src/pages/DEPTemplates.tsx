import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useDialog } from "../components/DialogProvider";
import { Save, RefreshCw } from "lucide-react";
import apiClient from "../lib/apiClient";

const FAMILIES = [
  { key: "mac",      label: "Mac" },
  { key: "ipad",     label: "iPad" },
  { key: "iphone",   label: "iPhone" },
  { key: "appletv",  label: "Apple TV" },
] as const;

type Family = (typeof FAMILIES)[number]["key"];

export function DEPTemplates() {
  const { t } = useTranslation();
  const dialog = useDialog();

  const [activeTab, setActiveTab] = useState<Family>("mac");
  const [contents, setContents] = useState<Record<Family, string>>({
    mac: "", ipad: "", iphone: "", appletv: "",
  });
  const [dirty, setDirty] = useState<Record<Family, boolean>>({
    mac: false, ipad: false, iphone: false, appletv: false,
  });
  const [loading, setLoading] = useState<Record<Family, boolean>>({
    mac: false, ipad: false, iphone: false, appletv: false,
  });
  const [saving, setSaving] = useState(false);
  const [exists, setExists] = useState<Record<Family, boolean>>({
    mac: false, ipad: false, iphone: false, appletv: false,
  });

  const load = async (family: Family) => {
    setLoading((p) => ({ ...p, [family]: true }));
    try {
      const { data } = await apiClient.get(`/api/dep/templates/${family}`);
      setContents((p) => ({ ...p, [family]: data.content ?? "" }));
      setExists((p) => ({ ...p, [family]: data.exists ?? false }));
      setDirty((p) => ({ ...p, [family]: false }));
    } catch (err) {
      await dialog.error("載入失敗：" + (err instanceof Error ? err.message : String(err)));
    } finally {
      setLoading((p) => ({ ...p, [family]: false }));
    }
  };

  // Load all families on mount.
  useEffect(() => {
    for (const f of FAMILIES) load(f.key);
  }, []);

  const handleChange = (value: string) => {
    setContents((p) => ({ ...p, [activeTab]: value }));
    setDirty((p) => ({ ...p, [activeTab]: true }));
  };

  const handleSave = async () => {
    const content = contents[activeTab].trim();
    // Basic JSON validation on the client too.
    try {
      JSON.parse(content);
    } catch {
      await dialog.error("JSON 格式錯誤，請確認後再儲存。");
      return;
    }
    setSaving(true);
    try {
      await apiClient.put(`/api/dep/templates/${activeTab}`, { content });
      setDirty((p) => ({ ...p, [activeTab]: false }));
      setExists((p) => ({ ...p, [activeTab]: true }));
      await dialog.success(`${activeTab}.json 已儲存`);
    } catch (err: unknown) {
      const resp = (err as { response?: { data?: { error?: string } } })?.response?.data;
      await dialog.error("儲存失敗：" + (resp?.error || (err instanceof Error ? err.message : String(err))));
    } finally {
      setSaving(false);
    }
  };

  const label = FAMILIES.find((f) => f.key === activeTab)?.label ?? activeTab;

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("nav.depTemplates")}</h1>
          <p className="text-sm text-base-content/60">
            管理各裝置類型的 DEP profile 模板（JSON），排程器套用時從此讀取。
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => load(activeTab)}
            disabled={loading[activeTab]}
            className="btn btn-ghost btn-sm gap-1"
            title="重新載入"
          >
            {loading[activeTab]
              ? <span className="loading loading-spinner loading-xs" />
              : <RefreshCw size={14} />}
            重新載入
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !dirty[activeTab]}
            className="btn btn-primary btn-sm gap-1"
          >
            {saving ? <span className="loading loading-spinner loading-xs" /> : <Save size={14} />}
            儲存 {label}
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div role="tablist" className="tabs tabs-bordered">
        {FAMILIES.map((f) => (
          <button
            key={f.key}
            role="tab"
            className={`tab gap-2 ${activeTab === f.key ? "tab-active" : ""}`}
            onClick={() => setActiveTab(f.key)}
          >
            {f.label}
            {dirty[f.key] && <span className="badge badge-warning badge-xs">未儲存</span>}
            {!exists[f.key] && !dirty[f.key] && (
              <span className="badge badge-ghost badge-xs">尚未建立</span>
            )}
          </button>
        ))}
      </div>

      {/* Editor */}
      <div className="card bg-base-100 shadow p-4 space-y-2">
        <div className="flex items-center gap-2 text-sm text-base-content/60">
          <span className="font-mono">{activeTab}.json</span>
          {exists[activeTab]
            ? <span className="badge badge-success badge-xs">已存在</span>
            : <span className="badge badge-ghost badge-xs">尚未儲存到磁碟</span>}
        </div>
        {loading[activeTab] ? (
          <div className="flex items-center justify-center h-64">
            <span className="loading loading-spinner loading-lg" />
          </div>
        ) : (
          <textarea
            className="textarea textarea-bordered font-mono text-xs w-full h-[60vh] resize-y"
            value={contents[activeTab]}
            onChange={(e) => handleChange(e.target.value)}
            spellCheck={false}
            placeholder={`輸入 ${label} 的 DEP profile JSON…`}
          />
        )}
        <p className="text-xs text-base-content/40">
          修改後按「儲存」寫入磁碟。排程器下次跑時（或按「立即套用 DEP profile」）即會使用新模板。
        </p>
      </div>
    </div>
  );
}
