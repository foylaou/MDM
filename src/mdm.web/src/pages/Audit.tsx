import { useState, useEffect, useMemo } from "react";
import { useAuthStore } from "../stores/authStore";
import { useEventStore } from "../stores/eventStore";
import { useTranslation } from "react-i18next";
import { Search, RefreshCw, Download, ChevronDown, ChevronRight, Clock, CheckCircle, AlertCircle, Activity } from "lucide-react";
import type { ColDef, ICellRendererParams } from "ag-grid-enterprise";
import { ResponseViewer } from "../components/ResponseViewer";
import { DataGrid } from "../components/DataGrid";
import type { AuditLog } from "../gen/mdm/v1/audit_pb";

const PAGE_SIZE = 20;

const MODULE_OPTIONS = [
  { value: "", label: "全部模組" },
  { value: "system", label: "系統" },
  { value: "asset", label: "財產管理" },
  { value: "mdm", label: "裝置管理" },
  { value: "rental", label: "租借系統" },
];

const MODULE_BADGE: Record<string, string> = {
  system: "badge-ghost",
  asset: "badge-primary",
  mdm: "badge-info",
  rental: "badge-warning",
};

export function Audit() {
  const { t } = useTranslation();
  const { clients, user } = useAuthStore();
  const { trackedCommands, events } = useEventStore();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterAction, setFilterAction] = useState("");
  const [filterModule, setFilterModule] = useState("");
  const [expandedCmds, setExpandedCmds] = useState<Set<string>>(new Set());

  const loadLogs = async () => {
    if (!clients) return;
    setLoading(true);
    try {
      const resp = await clients.audit.listAuditLogs({ action: filterAction, module: filterModule, pageSize: PAGE_SIZE });
      setLogs(resp.logs);
    } catch (err) { console.error("Failed to load audit logs:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadLogs(); }, [clients, filterAction, filterModule]);

  const exportCSV = () => {
    if (logs.length === 0) return;
    const header = ["Time", "User", "Action", "Target", "Module", "IP", "Detail"];
    const rows = logs.map((log) => [
      log.timestamp ? new Date(log.timestamp.toDate()).toISOString() : "",
      log.username,
      log.action,
      log.target,
      log.module || "system",
      log.ipAddress || "",
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

  const toggleCmd = (id: string) => {
    setExpandedCmds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const findEventForCommand = (commandUuid?: string) => {
    if (!commandUuid) return null;
    return events.find((e) => e.commandUuid === commandUuid && e.rawPayload);
  };

  const cmdStatusIcon = (status: string) => {
    switch (status) {
      case "acknowledged": return <CheckCircle size={16} className="text-success" />;
      case "error": return <AlertCircle size={16} className="text-error" />;
      default: return <Clock size={16} className="text-warning animate-pulse" />;
    }
  };

  const cmdStatusBadge = (status: string) => {
    switch (status) {
      case "acknowledged": return "badge-success";
      case "error": return "badge-error";
      default: return "badge-warning";
    }
  };

  const columnDefs = useMemo<ColDef<AuditLog>[]>(() => [
    {
      headerName: t("audit.time"),
      field: "timestamp" as any,
      width: 180,
      cellClass: "text-sm opacity-70",
      valueGetter: (p) => p.data?.timestamp ? p.data.timestamp.toDate().getTime() : 0,
      valueFormatter: (p) => p.value ? new Date(p.value as number).toLocaleString() : "-",
      sort: "desc",
    },
    { headerName: t("audit.user"), field: "username", width: 140, cellClass: "font-medium" },
    {
      headerName: t("audit.module"),
      field: "module",
      width: 110,
      cellRenderer: (p: ICellRendererParams<AuditLog>) => {
        const mod = (p.value as string) || "system";
        const modBadge = MODULE_BADGE[mod] || "badge-ghost";
        return <span className={`badge badge-sm ${modBadge}`}>{mod}</span>;
      },
    },
    {
      headerName: t("audit.action"),
      field: "action",
      width: 160,
      cellRenderer: (p: ICellRendererParams<AuditLog>) =>
        <span className="badge badge-info badge-sm">{p.value}</span>,
    },
    {
      headerName: t("audit.target"),
      field: "target",
      minWidth: 200,
      cellClass: "font-mono text-xs opacity-70",
      valueFormatter: (p) => p.value || "-",
      tooltipField: "target",
    },
    {
      headerName: "IP",
      field: "ipAddress",
      width: 140,
      cellClass: "font-mono text-xs opacity-50",
      valueFormatter: (p) => p.value || "-",
    },
    {
      headerName: t("audit.detail"),
      field: "detail",
      minWidth: 240,
      cellClass: "text-sm opacity-70",
      valueFormatter: (p) => p.value || "-",
      tooltipField: "detail",
    },
  ], [t]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("audit.title")}</h1>
          <p className="text-sm text-base-content/60">{t("audit.records", { count: logs.length })}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <label className="input input-bordered input-sm flex items-center gap-2">
            <Search size={14} className="opacity-50" />
            <input type="text" placeholder={t("audit.filterAction")} value={filterAction} onChange={(e) => setFilterAction(e.target.value)} className="grow w-36" />
          </label>
          <select
            value={filterModule}
            onChange={(e) => setFilterModule(e.target.value)}
            className="select select-bordered select-sm"
          >
            {MODULE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
          <button onClick={loadLogs} className="btn btn-ghost btn-sm gap-1"><RefreshCw size={14} />{t("common.refresh")}</button>
          <button onClick={exportCSV} disabled={logs.length === 0} className="btn btn-outline btn-sm gap-1"><Download size={14} />CSV</button>
        </div>
      </div>

      {/* Real-time command tracking */}
      {trackedCommands.length > 0 && (
        <div className="card bg-base-100 shadow">
          <div className="card-body p-4">
            <div className="flex items-center gap-2 mb-3">
              <Activity size={18} className="text-primary" />
              <h2 className="card-title text-base">{t("audit.liveTracking")}</h2>
              <span className="badge badge-primary badge-sm">{trackedCommands.length}</span>
            </div>
            <div className="space-y-2">
              {trackedCommands.map((cmd) => {
                const isExpanded = expandedCmds.has(cmd.id);
                const matchedEvent = findEventForCommand(cmd.commandUuid);
                return (
                  <div key={cmd.id} className="border border-base-300 rounded-lg overflow-hidden">
                    <button
                      className="w-full flex items-center gap-3 p-3 hover:bg-base-200/50 transition-colors text-left"
                      onClick={() => toggleCmd(cmd.id)}
                    >
                      {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                      {cmdStatusIcon(cmd.status)}
                      <span className="font-medium text-sm flex-1">{cmd.label}</span>
                      <span className="text-xs text-base-content/50">{cmd.udids.length} {t("audit.deviceCount")}</span>
                      <span className={`badge badge-sm ${cmdStatusBadge(cmd.status)}`}>
                        {t(`audit.cmdStatus.${cmd.status}`)}
                      </span>
                    </button>
                    {isExpanded && (
                      <div className="border-t border-base-300 p-3 bg-base-200/30 space-y-2">
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-sm">
                          <div className="flex justify-between">
                            <span className="opacity-60">{t("audit.executor")}</span>
                            <span className="font-medium">{user?.display_name || user?.username || "-"}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="opacity-60">{t("audit.sentTime")}</span>
                            <span>{cmd.sentAt.toLocaleTimeString()}</span>
                          </div>
                          {cmd.responseAt && (
                            <div className="flex justify-between">
                              <span className="opacity-60">{t("audit.responseTime")}</span>
                              <span>{cmd.responseAt.toLocaleTimeString()}</span>
                            </div>
                          )}
                          <div className="flex justify-between">
                            <span className="opacity-60">{t("audit.targetDevices")}</span>
                            <span className="font-mono text-xs">{cmd.udids.map((u) => u.slice(0, 8) + "\u2026").join(", ")}</span>
                          </div>
                          {cmd.commandUuid && (
                            <div className="flex justify-between sm:col-span-2">
                              <span className="opacity-60">Command UUID</span>
                              <span className="font-mono text-xs opacity-70">{cmd.commandUuid}</span>
                            </div>
                          )}
                        </div>
                        {matchedEvent?.rawPayload && (
                          <div className="mt-2">
                            <div className="text-xs font-medium opacity-60 mb-1">{t("audit.responsePayload")}</div>
                            <ResponseViewer rawPayload={matchedEvent.rawPayload} />
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}

      <div className="card bg-base-100 shadow p-2">
        <DataGrid<AuditLog>
          rowData={logs}
          columnDefs={columnDefs}
          loading={loading}
          getRowId={(p) => p.data.id}
          overlayNoRowsTemplate={`<span class="opacity-50">${t("audit.noLogs")}</span>`}
        />
      </div>
    </div>
  );
}
