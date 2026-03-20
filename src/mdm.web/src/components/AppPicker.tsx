import { useState, useEffect } from "react";
import apiClient from "../lib/apiClient";
import { Package } from "lucide-react";

interface ManagedApp {
  id: string;
  name: string;
  bundle_id: string;
  app_type: "vpp" | "enterprise";
  itunes_store_id: string;
  manifest_url: string;
  purchased_qty: number;
  installed_count: number;
  icon_url: string;
}

interface AppPickerProps {
  /** Filter by app type, or show all if undefined */
  appType?: "vpp" | "enterprise";
  /** Currently selected app ID */
  selectedId: string;
  /** Called when user selects an app */
  onSelect: (app: ManagedApp | null) => void;
}

export function AppPicker({ appType, selectedId, onSelect }: AppPickerProps) {
  const [apps, setApps] = useState<ManagedApp[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    apiClient.get("/api/managed-apps")
      .then(({ data }) => {
        let list: ManagedApp[] = data.apps || [];
        if (appType) list = list.filter((a) => a.app_type === appType);
        setApps(list);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [appType]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-base-content/50">
        <span className="loading loading-spinner loading-sm"></span>
        載入 App 列表...
      </div>
    );
  }

  if (apps.length === 0) {
    return (
      <div className="text-center py-4 text-base-content/50 text-sm">
        尚未登記任何{appType === "enterprise" ? "企業" : ""} App
      </div>
    );
  }

  return (
    <div className="border border-base-300 rounded-lg max-h-56 overflow-y-auto divide-y divide-base-200">
      {apps.map((a) => {
        const selected = selectedId === a.id;
        return (
          <div
            key={a.id}
            onClick={() => onSelect(selected ? null : a)}
            className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors hover:bg-base-200
              ${selected ? "bg-primary/10 border-l-2 border-primary" : ""}`}
          >
            {a.icon_url ? (
              <img src={a.icon_url} alt="" className="w-8 h-8 rounded-lg flex-shrink-0" />
            ) : (
              <div className="w-8 h-8 rounded-lg bg-base-300 flex items-center justify-center flex-shrink-0">
                <Package size={14} className="opacity-40" />
              </div>
            )}
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium">{a.name}</div>
              <div className="text-xs opacity-50 font-mono truncate">{a.bundle_id}</div>
            </div>
            <span className={`badge badge-xs ${a.app_type === "vpp" ? "badge-primary" : "badge-secondary"}`}>
              {a.app_type === "vpp" ? "VPP" : "企業"}
            </span>
          </div>
        );
      })}
    </div>
  );
}
