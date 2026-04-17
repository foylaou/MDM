import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { History } from "lucide-react";
import apiClient from "../lib/apiClient";

interface CustodyLog {
  id: string;
  asset_id: string;
  action: string;
  from_user_id: string | null;
  from_user_name: string;
  to_user_id: string | null;
  to_user_name: string;
  reason: string;
  operated_by: string | null;
  operator_name: string;
  created_at: string;
}

interface CustodyHistoryProps {
  assetId: string;
  /** Bumped by parent to trigger reload (e.g. after a transfer). */
  refreshKey?: number;
}

const ACTION_LABELS: Record<string, { label: string; badge: string }> = {
  assign:   { label: "assign",   badge: "badge-success" },
  transfer: { label: "transfer", badge: "badge-info" },
  revoke:   { label: "revoke",   badge: "badge-warning" },
};

export function CustodyHistory({ assetId, refreshKey = 0 }: CustodyHistoryProps) {
  const { t } = useTranslation();
  const [logs, setLogs] = useState<CustodyLog[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!assetId) return;
    setLoading(true);
    apiClient.get(`/api/assets-custody/${assetId}`)
      .then(({ data }) => setLogs(data.logs || []))
      .catch(() => setLogs([]))
      .finally(() => setLoading(false));
  }, [assetId, refreshKey]);

  if (loading) {
    return <div className="flex justify-center py-4"><span className="loading loading-spinner loading-sm"></span></div>;
  }
  if (logs.length === 0) {
    return <p className="text-sm text-base-content/50 py-2">{t("custody.noHistory")}</p>;
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1 text-sm font-medium">
        <History size={14} /> {t("custody.historyTitle")}
      </div>
      <div className="overflow-x-auto">
        <table className="table table-sm">
          <thead>
            <tr>
              <th>{t("custody.col.time")}</th>
              <th>{t("custody.col.action")}</th>
              <th>{t("custody.col.from")}</th>
              <th>{t("custody.col.to")}</th>
              <th>{t("custody.col.reason")}</th>
              <th>{t("custody.col.operator")}</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((l) => {
              const a = ACTION_LABELS[l.action] || { label: l.action, badge: "badge-ghost" };
              const label = t(`custody.action.${l.action}`, { defaultValue: a.label });
              return (
                <tr key={l.id}>
                  <td className="whitespace-nowrap text-xs">{new Date(l.created_at).toLocaleString()}</td>
                  <td><span className={`badge badge-sm ${a.badge}`}>{label}</span></td>
                  <td className="text-sm">{l.from_user_name || <span className="opacity-30">-</span>}</td>
                  <td className="text-sm">{l.to_user_name || <span className="opacity-30">-</span>}</td>
                  <td className="text-sm">{l.reason || <span className="opacity-30">-</span>}</td>
                  <td className="text-sm">{l.operator_name || <span className="opacity-30">-</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
