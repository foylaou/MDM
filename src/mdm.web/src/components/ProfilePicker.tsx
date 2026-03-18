import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { FileText } from "lucide-react";

interface Profile {
  id: string;
  name: string;
  filename: string;
  size: number;
}

interface ProfilePickerProps {
  onSelect: (profileId: string, base64Content: string) => void;
  selectedId?: string;
}

export function ProfilePicker({ onSelect, selectedId }: ProfilePickerProps) {
  const { t } = useTranslation();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loading, setLoading] = useState(true);

  const baseUrl = import.meta.env.DEV ? "" : window.location.origin;

  useEffect(() => {
    setLoading(true);
    fetch(`${baseUrl}/api/profiles`, { credentials: "include" })
      .then((r) => r.json())
      .then((data) => setProfiles(data.profiles || []))
      .catch((err) => console.error("ProfilePicker:", err))
      .finally(() => setLoading(false));
  }, []);

  const handleChange = async (profileId: string) => {
    if (!profileId) { onSelect("", ""); return; }
    try {
      const resp = await fetch(`${baseUrl}/api/profiles/${profileId}`, { credentials: "include" });
      const data = await resp.json();
      onSelect(profileId, data.content_base64 || "");
    } catch (err) { console.error("Failed to load profile:", err); }
  };

  if (loading) {
    return <div className="flex items-center gap-2 text-sm text-base-content/50"><span className="loading loading-spinner loading-xs"></span>{t("common.loading")}</div>;
  }
  if (profiles.length === 0) {
    return <div className="text-sm text-base-content/50">{t("profiles.noProfiles")}</div>;
  }

  return (
    <select value={selectedId || ""} onChange={(e) => handleChange(e.target.value)} className="select select-bordered w-full">
      <option value="">{t("profiles.selectProfile")}</option>
      {profiles.map((p) => (
        <option key={p.id} value={p.id}><FileText size={14} /> {p.name} ({p.filename})</option>
      ))}
    </select>
  );
}
