import { useState, useEffect, useRef } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { useDialog } from "../components/DialogProvider";
import { Upload, Trash2, FileText, RefreshCw } from "lucide-react";

interface Profile {
  id: string;
  name: string;
  filename: string;
  size: number;
  uploaded_by: string;
  created_at: string;
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function Profiles() {
  const { t } = useTranslation();
  const dialog = useDialog();
  useAuthStore(); // ensure authenticated
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const baseUrl = import.meta.env.DEV ? "" : window.location.origin;

  const loadProfiles = async () => {
    setLoading(true);
    try {
      const resp = await fetch(`${baseUrl}/api/profiles`, {
        credentials: "include",
      });
      const data = await resp.json();
      setProfiles(data.profiles || []);
    } catch (err) {
      console.error("Failed to load profiles:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadProfiles(); }, []);

  // Triggered when file is selected — immediately upload
  const onFileSelected = async () => {
    const file = fileRef.current?.files?.[0];
    if (!file) return;

    if (!file.name.endsWith(".mobileconfig")) {
      await dialog.alert("請選擇 .mobileconfig 檔案");
      if (fileRef.current) fileRef.current.value = "";
      return;
    }

    setUploading(true);
    try {
      const form = new FormData();
      form.append("file", file);
      // Use filename without extension as default name
      form.append("name", file.name.replace(".mobileconfig", ""));

      const resp = await fetch(`${baseUrl}/api/profiles`, {
        method: "POST",
        credentials: "include",
        body: form,
      });

      if (!resp.ok) {
        const data = await resp.json().catch(() => ({ error: `HTTP ${resp.status}` }));
        throw new Error(data.error || "Upload failed");
      }

      if (fileRef.current) fileRef.current.value = "";
      loadProfiles();
    } catch (err) {
      await dialog.error(t("profiles.uploadFailed") + ": " + (err instanceof Error ? err.message : ""));
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (id: string, name: string) => {
    if (!(await dialog.confirm(t("profiles.deleteConfirm", { name })))) return;
    try {
      const resp = await fetch(`${baseUrl}/api/profiles/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!resp.ok) throw new Error("delete failed");
      loadProfiles();
    } catch (err) {
      await dialog.error(t("profiles.deleteFailed") + ": " + (err instanceof Error ? err.message : ""));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("profiles.title")}</h1>
          <p className="text-sm text-base-content/60">{t("profiles.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={loadProfiles} className="btn btn-ghost btn-sm gap-1">
            <RefreshCw size={14} />{t("common.refresh")}
          </button>
          <label className={`btn btn-primary btn-sm gap-1 ${uploading ? "btn-disabled" : ""}`}>
            {uploading ? <span className="loading loading-spinner loading-xs"></span> : <Upload size={14} />}
            {uploading ? t("profiles.uploading") : t("profiles.upload")}
            <input
              ref={fileRef}
              type="file"
              accept=".mobileconfig"
              className="hidden"
              onChange={onFileSelected}
              disabled={uploading}
            />
          </label>
        </div>
      </div>

      {/* Profile list */}
      <div className="card bg-base-100 shadow">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t("profiles.name")}</th>
                <th>{t("profiles.filename")}</th>
                <th>{t("profiles.size")}</th>
                <th>{t("profiles.uploadedBy")}</th>
                <th>{t("profiles.uploadedAt")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={6} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : profiles.length === 0 ? (
                <tr><td colSpan={6} className="text-center py-8 text-base-content/50">{t("profiles.noProfiles")}</td></tr>
              ) : profiles.map((p) => (
                <tr key={p.id} className="hover">
                  <td className="font-medium">
                    <div className="flex items-center gap-2">
                      <FileText size={16} className="opacity-40" />
                      {p.name}
                    </div>
                  </td>
                  <td className="font-mono text-xs opacity-70">{p.filename}</td>
                  <td className="text-sm">{formatSize(p.size)}</td>
                  <td className="text-sm">{p.uploaded_by}</td>
                  <td className="text-sm opacity-70">{new Date(p.created_at).toLocaleString()}</td>
                  <td>
                    <button onClick={() => handleDelete(p.id, p.name)} className="btn btn-ghost btn-xs text-error">
                      <Trash2 size={14} />
                    </button>
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
