import { useState, useEffect, useMemo } from "react";
import { useAuthStore } from "../stores/authStore";
import { useModulePermission } from "../hooks/useModulePermission";
import { AssetPicker } from "../components/AssetPicker";
import apiClient from "../lib/apiClient";
import { useDialog } from "../components/DialogProvider";
import {
  Check, X, Clock, CheckCircle, AlertCircle, FileDown, Trash2, Eye,
} from "lucide-react";
import type { ColDef, ICellRendererParams } from "ag-grid-enterprise";
import { DataGrid } from "../components/DataGrid";

interface DisposalItem {
  line_no: number;
  asset_id: string;
  asset_name: string;
  asset_number: string;
  dispose_date: string | null;
  dispose_reason: string;
  data_wipe_checked: boolean;
}

interface DisposalRequest {
  id: string;
  request_number: number;
  applicant_id: string;
  applicant_name: string;
  status: string;
  approver_id?: string;
  approver_name: string;
  reject_reason: string;
  is_archived: boolean;
  items: DisposalItem[];
}

interface UserOption {
  id: string;
  username: string;
  display_name: string;
}

interface AssetOption {
  asset_id: string;
  asset_number: string;
  name: string;
}

const statusConfig: Record<string, { label: string; badge: string; icon: React.ReactNode }> = {
  pending:  { label: "待核准", badge: "badge-warning", icon: <Clock size={14} /> },
  approved: { label: "已報廢", badge: "badge-ghost",   icon: <CheckCircle size={14} /> },
  rejected: { label: "已拒絕", badge: "badge-error",   icon: <X size={14} /> },
};

async function downloadExportExcel(status: string) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const resp = await apiClient.get(`/api/disposal-requests-export?${params}`, { responseType: "blob" });
  const url = URL.createObjectURL(resp.data);
  const a = document.createElement("a");
  a.href = url;
  const disposition = resp.headers["content-disposition"] || "";
  const match = disposition.match(/filename="?([^"]+)"?/);
  a.download = match?.[1] || `資訊資產報廢申請_${new Date().toISOString().slice(0, 10)}.xlsx`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

interface ItemDraft {
  disposeDate: string;
  disposeReason: string;
  dataWipeChecked: boolean;
}

export function DisposalRequests() {
  const { user } = useAuthStore();
  const perm = useModulePermission("disposal");
  const dialog = useDialog();

  const [requests, setRequests] = useState<DisposalRequest[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [assets, setAssets] = useState<AssetOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [detailTarget, setDetailTarget] = useState<DisposalRequest | null>(null);
  const [rejectTarget, setRejectTarget] = useState<DisposalRequest | null>(null);
  const [rejectReason, setRejectReason] = useState("");

  // Create form
  const [applicantId, setApplicantId] = useState("");
  const [selectedAssets, setSelectedAssets] = useState<string[]>([]);
  const [drafts, setDrafts] = useState<Record<string, ItemDraft>>({});

  const load = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/disposal-requests", {
        params: { status: statusFilter, show_archived: showArchived ? "true" : "" },
      });
      setRequests(data.requests || []);
    } catch (err) { console.error("Load disposal requests:", err); }
    finally { setLoading(false); }
  };

  const loadUsers = async () => {
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch { /* */ }
  };

  const loadAssets = async () => {
    try {
      const { data } = await apiClient.get("/api/pickable-assets");
      setAssets(data.assets || []);
    } catch { /* */ }
  };

  useEffect(() => { load(); }, [statusFilter, showArchived]);
  useEffect(() => { loadUsers(); loadAssets(); }, []);

  useEffect(() => {
    if (!perm.canApprove && user?.id && !applicantId) {
      setApplicantId(user.id);
    }
  }, [perm.canApprove, user, applicantId]);

  const assetById = useMemo(() => new Map(assets.map((a) => [a.asset_id, a])), [assets]);

  // Keep drafts in sync with the current selection.
  useEffect(() => {
    setDrafts((prev) => {
      const next: Record<string, ItemDraft> = {};
      for (const id of selectedAssets) {
        next[id] = prev[id] || { disposeDate: new Date().toISOString().slice(0, 10), disposeReason: "", dataWipeChecked: false };
      }
      return next;
    });
  }, [selectedAssets]);

  const resetCreateForm = () => {
    setApplicantId("");
    setSelectedAssets([]);
    setDrafts({});
  };

  const allDraftsValid = selectedAssets.length > 0 && selectedAssets.every((id) => (drafts[id]?.disposeReason || "").trim() !== "");

  const handleCreate = async () => {
    if (!applicantId || !allDraftsValid) return;
    setCreating(true);
    try {
      await apiClient.post("/api/disposal-requests", {
        applicant_id: applicantId,
        items: selectedAssets.map((id) => ({
          asset_id: id,
          dispose_date: drafts[id]?.disposeDate || null,
          dispose_reason: drafts[id]?.disposeReason || "",
          data_wipe_checked: drafts[id]?.dataWipeChecked || false,
        })),
      });
      setShowCreate(false);
      resetCreateForm();
      load();
    } catch (err: unknown) {
      const resp = (err as { response?: { data?: { error?: string; assets?: string[] } } })?.response?.data;
      await dialog.error(resp?.error || (err instanceof Error ? err.message : "建立失敗"), resp?.assets || []);
    } finally { setCreating(false); }
  };

  const confirmApprove = async (req: DisposalRequest) => {
    if (!(await dialog.confirm(`核准後將立即報廢此申請的 ${req.items.length} 筆資產，此動作無法復原。確認核准？`))) return;
    try {
      await apiClient.post(`/api/disposal-requests/${req.id}/approve`);
      load();
    } catch (err: unknown) {
      const resp = (err as { response?: { data?: { error?: string } } })?.response?.data;
      await dialog.error(resp?.error || "核准失敗");
    }
  };

  const confirmReject = async () => {
    if (!rejectTarget) return;
    try {
      await apiClient.post(`/api/disposal-requests/${rejectTarget.id}/reject`, { reason: rejectReason });
      setRejectTarget(null);
      load();
    } catch (err) {
      await dialog.error("拒絕失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const columnDefs = useMemo<ColDef<DisposalRequest>[]>(() => [
    {
      headerName: "",
      colId: "expand",
      width: 44,
      sortable: false,
      filter: false,
      resizable: false,
      cellRendererSelector: (p) => (p.data?.items.length ?? 0) > 1
        ? { component: "agGroupCellRenderer", params: { suppressCount: true } }
        : undefined,
    },
    { headerName: "申請單號", field: "request_number", width: 100, cellClass: "font-mono text-sm font-medium" },
    { headerName: "申請人員", field: "applicant_name", width: 120 },
    {
      headerName: "資產數量", colId: "count", width: 100,
      valueGetter: (p) => p.data?.items.length ?? 0,
    },
    {
      headerName: "狀態", field: "status", width: 120,
      cellRenderer: (p: ICellRendererParams<DisposalRequest>) => {
        const sc = statusConfig[p.value as string] || statusConfig.pending;
        return <span className={`badge badge-sm gap-1 ${sc.badge}`}>{sc.icon} {sc.label}</span>;
      },
    },
    { headerName: "核准人", field: "approver_name", width: 120, valueFormatter: (p) => p.value || "-" },
    {
      headerName: "操作", colId: "actions", width: 220, pinned: "right", sortable: false, filter: false,
      cellRenderer: (p: ICellRendererParams<DisposalRequest>) => {
        const req = p.data!;
        return (
          <div className="flex gap-1 h-full items-center">
            <button onClick={() => setDetailTarget(req)} className="btn btn-ghost btn-xs gap-1"><Eye size={12} /> 明細</button>
            {req.status === "pending" && perm.canApprove && (
              <>
                <button onClick={() => confirmApprove(req)} className="btn btn-success btn-xs gap-1"><CheckCircle size={12} /> 核准</button>
                <button onClick={() => { setRejectTarget(req); setRejectReason(""); }} className="btn btn-error btn-xs gap-1"><AlertCircle size={12} /> 拒絕</button>
              </>
            )}
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [perm.canApprove]);

  const detailCellRendererParams = useMemo(() => ({
    detailGridOptions: {
      columnDefs: [
        { headerName: "NO", valueGetter: (p: any) => p.data?.line_no, width: 60 },
        { headerName: "資產名稱", flex: 1, valueGetter: (p: any) => p.data?.asset_name || "-" },
        { headerName: "資產編號", flex: 1, cellClass: "font-mono text-xs", valueGetter: (p: any) => p.data?.asset_number || "-" },
        { headerName: "報廢日期", width: 110, valueGetter: (p: any) => p.data?.dispose_date || "-" },
        { headerName: "報廢原因", flex: 1, valueGetter: (p: any) => p.data?.dispose_reason || "-" },
        {
          headerName: "資料清除檢核", width: 110,
          cellRenderer: (p: ICellRendererParams<DisposalItem>) =>
            p.data?.data_wipe_checked
              ? <Check size={14} className="text-success" />
              : <X size={14} className="text-error" />,
        },
      ] as ColDef<DisposalItem>[],
      defaultColDef: { sortable: false, filter: false, resizable: true },
      domLayout: "autoHeight" as const,
      headerHeight: 32,
      rowHeight: 32,
    },
    getDetailRowData: (p: any) => { p.successCallback((p.data as DisposalRequest).items); },
  }), []);

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">資訊資產報廢申請</h1>
          <p className="text-sm text-base-content/60">申請報廢資產，核准後執行資料清除與報廢</p>
        </div>
        <div className="flex gap-2 flex-wrap">
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="select select-bordered select-sm">
            <option value="">全部狀態</option>
            <option value="pending">待核准</option>
            <option value="approved">已報廢</option>
            <option value="rejected">已拒絕</option>
          </select>
          <label className="flex items-center gap-1.5 cursor-pointer text-sm">
            <input type="checkbox" className="checkbox checkbox-xs" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} />
            顯示已封存
          </label>
          {perm.hasAccess && (
            <button onClick={() => downloadExportExcel(statusFilter)} className="btn btn-outline btn-sm gap-1">
              <FileDown size={14} /> 匯出 Excel
            </button>
          )}
          {perm.canRequest && (
            <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm gap-1">
              <Trash2 size={14} /> 新增報廢申請
            </button>
          )}
        </div>
      </div>

      {showCreate && (
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <div className="flex items-center justify-between mb-2">
              <h2 className="card-title text-base">新增資訊資產報廢申請</h2>
              <button onClick={() => { setShowCreate(false); resetCreateForm(); }} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
            </div>
            <div className="space-y-4">
              <div className="form-control">
                <label className="label"><span className="label-text font-medium">申請人員</span></label>
                {!perm.canApprove ? (
                  <input type="text" value={user?.display_name || user?.username || ""} className="input input-bordered input-sm max-w-xs" disabled />
                ) : (
                  <select value={applicantId} onChange={(e) => setApplicantId(e.target.value)} className="select select-bordered select-sm max-w-xs">
                    <option value="">選擇使用者</option>
                    {users.map((u) => (
                      <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
                    ))}
                  </select>
                )}
              </div>

              <div className="form-control">
                <label className="label"><span className="label-text font-medium">選擇資產</span></label>
                <AssetPicker selected={selectedAssets} onChange={setSelectedAssets} showFilters endpoint="/api/pickable-assets" />
              </div>

              {selectedAssets.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="table table-sm">
                    <thead>
                      <tr>
                        <th>NO</th>
                        <th>資產名稱</th>
                        <th>資產編號</th>
                        <th>報廢日期</th>
                        <th>報廢原因*</th>
                        <th>資料清除檢核*</th>
                      </tr>
                    </thead>
                    <tbody>
                      {selectedAssets.map((id, i) => {
                        const a = assetById.get(id);
                        const d = drafts[id] || { disposeDate: "", disposeReason: "", dataWipeChecked: false };
                        return (
                          <tr key={id}>
                            <td>{i + 1}</td>
                            <td>{a?.name || "-"}</td>
                            <td className="font-mono text-xs">{a?.asset_number || "-"}</td>
                            <td>
                              <input
                                type="date"
                                value={d.disposeDate}
                                onChange={(e) => setDrafts((prev) => ({ ...prev, [id]: { ...d, disposeDate: e.target.value } }))}
                                className="input input-bordered input-xs"
                              />
                            </td>
                            <td>
                              <input
                                type="text"
                                value={d.disposeReason}
                                onChange={(e) => setDrafts((prev) => ({ ...prev, [id]: { ...d, disposeReason: e.target.value } }))}
                                className="input input-bordered input-xs w-full"
                                placeholder="報廢原因"
                              />
                            </td>
                            <td>
                              <input
                                type="checkbox"
                                className="checkbox checkbox-sm"
                                checked={d.dataWipeChecked}
                                onChange={(e) => setDrafts((prev) => ({ ...prev, [id]: { ...d, dataWipeChecked: e.target.checked } }))}
                              />
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}

              <div className="flex gap-2">
                <button onClick={handleCreate} disabled={creating || !applicantId || !allDraftsValid} className="btn btn-success btn-sm gap-1">
                  {creating && <span className="loading loading-spinner loading-xs"></span>}
                  提交申請
                </button>
                <button onClick={() => { setShowCreate(false); resetCreateForm(); }} className="btn btn-ghost btn-sm">取消</button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="card bg-base-100 shadow p-2">
        <DataGrid<DisposalRequest>
          rowData={requests}
          columnDefs={columnDefs}
          loading={loading}
          getRowId={(p) => p.data.id}
          overlayNoRowsTemplate={`<span class="opacity-50">尚無報廢申請記錄</span>`}
          masterDetail
          isRowMaster={(data) => data.items.length > 1}
          detailCellRendererParams={detailCellRendererParams}
          detailRowAutoHeight
          getRowClass={(p) => p.data?.is_archived ? "opacity-50" : ""}
        />
      </div>

      {/* Detail dialog */}
      <dialog className={`modal ${detailTarget ? "modal-open" : ""}`}>
        <div className="modal-box max-w-2xl">
          <h3 className="font-bold text-lg">報廢申請明細 #{detailTarget?.request_number}</h3>
          <div className="overflow-x-auto mt-3">
            <table className="table table-sm">
              <thead>
                <tr>
                  <th>NO</th><th>資產名稱</th><th>資產編號</th><th>報廢日期</th><th>報廢原因</th><th>資料清除檢核</th>
                </tr>
              </thead>
              <tbody>
                {detailTarget?.items.map((it) => (
                  <tr key={it.line_no}>
                    <td>{it.line_no}</td>
                    <td>{it.asset_name || "-"}</td>
                    <td className="font-mono text-xs">{it.asset_number || "-"}</td>
                    <td>{it.dispose_date || "-"}</td>
                    <td>{it.dispose_reason || "-"}</td>
                    <td>{it.data_wipe_checked ? <Check size={14} className="text-success" /> : <X size={14} className="text-error" />}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {detailTarget?.reject_reason && (
            <div role="alert" className="alert alert-error mt-3 py-2">
              <span className="text-sm">拒絕原因：{detailTarget.reject_reason}</span>
            </div>
          )}
          <div className="modal-action">
            <button className="btn btn-sm" onClick={() => setDetailTarget(null)}>關閉</button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setDetailTarget(null)}>close</button>
        </form>
      </dialog>

      {/* Reject dialog */}
      <dialog className={`modal ${rejectTarget ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg">拒絕報廢申請</h3>
          <div className="form-control mt-4">
            <label className="label"><span className="label-text text-sm">拒絕原因</span></label>
            <textarea
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              className="textarea textarea-bordered textarea-sm"
              rows={2}
            />
          </div>
          <div className="modal-action">
            <button className="btn btn-sm" onClick={() => setRejectTarget(null)}>取消</button>
            <button className="btn btn-error btn-sm gap-1" onClick={confirmReject}>
              <X size={14} /> 確認拒絕
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setRejectTarget(null)}>close</button>
        </form>
      </dialog>
    </div>
  );
}
