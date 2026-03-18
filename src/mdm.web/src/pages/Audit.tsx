import { useState, useEffect } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { Search, RefreshCw, Download } from "lucide-react";
import { Pagination } from "../components/Pagination";
import type { AuditLog } from "../gen/mdm/v1/audit_pb";

const PAGE_SIZE = 20;

export function Audit() {
  const { t } = useTranslation();
  const { clients } = useAuthStore();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterAction, setFilterAction] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  const loadLogs = async () => {
    if (!clients) return;
    setLoading(true);
    try {
      const resp = await clients.audit.listAuditLogs({ action: filterAction, pageSize: PAGE_SIZE });
      setLogs(resp.logs);
      setTotal(resp.logs.length < PAGE_SIZE ? resp.logs.length : resp.logs.length * 3); // estimate
    } catch (err) { console.error("Failed to load audit logs:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadLogs(); }, [clients, filterAction]);
  useEffect(() => { setPage(1); }, [filterAction]);

  const exportCSV = () => {
    if (logs.length === 0) return;
    const header = ["Time", "User", "Action", "Target", "Detail"];
    const rows = logs.map((log) => [
      log.timestamp ? new Date(log.timestamp.toDate()).toISOString() : "",
      log.username,
      log.action,
      log.target,
      log.detail,
    ]);
    const csv = [header, ...rows].map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(",")).join("\n");
    const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `audit_logs_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("audit.title")}</h1>
          <p className="text-sm text-base-content/60">{t("audit.records", { count: logs.length })}</p>
        </div>
        <div className="flex gap-2">
          <label className="input input-bordered input-sm flex items-center gap-2">
            <Search size={14} className="opacity-50" />
            <input type="text" placeholder={t("audit.filterAction")} value={filterAction} onChange={(e) => setFilterAction(e.target.value)} className="grow w-36" />
          </label>
          <button onClick={loadLogs} className="btn btn-ghost btn-sm gap-1"><RefreshCw size={14} />{t("common.refresh")}</button>
          <button onClick={exportCSV} disabled={logs.length === 0} className="btn btn-outline btn-sm gap-1"><Download size={14} />CSV</button>
        </div>
      </div>

      <div className="card bg-base-100 shadow">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t("audit.time")}</th>
                <th>{t("audit.user")}</th>
                <th>{t("audit.action")}</th>
                <th>{t("audit.target")}</th>
                <th>{t("audit.detail")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : logs.length === 0 ? (
                <tr><td colSpan={5} className="text-center py-8 text-base-content/50">{t("audit.noLogs")}</td></tr>
              ) : logs.map((log) => (
                <tr key={log.id} className="hover">
                  <td className="text-sm opacity-70">{log.timestamp ? new Date(log.timestamp.toDate()).toLocaleString() : "-"}</td>
                  <td className="font-medium">{log.username}</td>
                  <td><span className="badge badge-info badge-sm">{log.action}</span></td>
                  <td className="font-mono text-xs opacity-70 max-w-xs truncate">{log.target || "-"}</td>
                  <td className="text-sm opacity-70 max-w-xs truncate">{log.detail || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="px-4 pb-4">
          <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
        </div>
      </div>
    </div>
  );
}
