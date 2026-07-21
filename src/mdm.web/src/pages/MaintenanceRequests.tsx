import { useState, useEffect, useMemo } from "react";
import { useAuthStore } from "../stores/authStore";
import { useModulePermission } from "../hooks/useModulePermission";
import { AssetPicker } from "../components/AssetPicker";
import apiClient from "../lib/apiClient";
import { useDialog } from "../components/DialogProvider";
import {
  Check, X, RotateCcw, Clock, PenLine,
  CheckCircle, AlertCircle, FileDown, Wrench,
} from "lucide-react";
import type { ColDef, ICellRendererParams } from "ag-grid-enterprise";
import { DataGrid } from "../components/DataGrid";

interface MaintenanceRequest {
  id: string;
  request_number: number;
  asset_id: string;
  asset_name: string;
  asset_number: string;
  applicant_id: string;
  applicant_name: string;
  reason: string;
  vendor: string;
  technician: string;
  checkout_date: string | null;
  return_date: string | null;
  process_notes: string;
  status: string;
  handler_id?: string;
  handler_name: string;
  supervisor_id?: string;
  supervisor_name: string;
  reject_reason: string;
  is_archived: boolean;
  contains_sensitive_data: boolean;
  vendor_nda_ref: string;
  data_wiped_before_checkout: boolean;
  loaner_info: string;
  loaner_provided_date: string | null;
  loaner_security_checked: boolean;
  loaner_returned_date: string | null;
}

interface UserOption {
  id: string;
  username: string;
  display_name: string;
}

const statusConfig: Record<string, { label: string; badge: string; icon: React.ReactNode }> = {
  pending:        { label: "待承辦",   badge: "badge-warning", icon: <Clock size={14} /> },
  handler_signed: { label: "待主管核准", badge: "badge-info",    icon: <PenLine size={14} /> },
  approved:       { label: "維修中",   badge: "badge-success", icon: <Wrench size={14} /> },
  returned:       { label: "已歸還",   badge: "badge-ghost",   icon: <RotateCcw size={14} /> },
  rejected:       { label: "已拒絕",   badge: "badge-error",   icon: <X size={14} /> },
};

async function downloadExportExcel(status: string) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const resp = await apiClient.get(`/api/maintenance-requests-export?${params}`, { responseType: "blob" });
  const url = URL.createObjectURL(resp.data);
  const a = document.createElement("a");
  a.href = url;
  const disposition = resp.headers["content-disposition"] || "";
  const match = disposition.match(/filename="?([^"]+)"?/);
  a.download = match?.[1] || `設備維修申請_${new Date().toISOString().slice(0, 10)}.xlsx`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function MaintenanceRequests() {
  const { user } = useAuthStore();
  const perm = useModulePermission("maintenance");
  const dialog = useDialog();

  const [requests, setRequests] = useState<MaintenanceRequest[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);

  // Create form
  const [selectedAssets, setSelectedAssets] = useState<string[]>([]);
  const [applicantId, setApplicantId] = useState("");
  const [reason, setReason] = useState("");
  const [vendor, setVendor] = useState("");
  const [technician, setTechnician] = useState("");
  const [checkoutDate, setCheckoutDate] = useState("");
  const [returnDatePlanned, setReturnDatePlanned] = useState("");
  const [containsSensitiveData, setContainsSensitiveData] = useState(false);
  const [vendorNdaRef, setVendorNdaRef] = useState("");

  // Approve dialog
  const [approveTarget, setApproveTarget] = useState<MaintenanceRequest | null>(null);
  const [approveCheckoutDate, setApproveCheckoutDate] = useState("");
  const [approveDataWiped, setApproveDataWiped] = useState(false);
  const [approveLoanerInfo, setApproveLoanerInfo] = useState("");
  const [approveLoanerProvidedDate, setApproveLoanerProvidedDate] = useState("");
  const [approveLoanerSecurityChecked, setApproveLoanerSecurityChecked] = useState(false);

  // Return dialog
  const [returnTarget, setReturnTarget] = useState<MaintenanceRequest | null>(null);
  const [returnDate, setReturnDate] = useState("");
  const [processNotes, setProcessNotes] = useState("");
  const [loanerReturnedDate, setLoanerReturnedDate] = useState("");

  // Reject dialog
  const [rejectTarget, setRejectTarget] = useState<MaintenanceRequest | null>(null);
  const [rejectReason, setRejectReason] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/maintenance-requests", {
        params: { status: statusFilter, show_archived: showArchived ? "true" : "" },
      });
      setRequests(data.requests || []);
    } catch (err) { console.error("Load maintenance requests:", err); }
    finally { setLoading(false); }
  };

  const loadUsers = async () => {
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch { /* */ }
  };

  useEffect(() => { load(); }, [statusFilter, showArchived]);
  useEffect(() => { loadUsers(); }, []);

  useEffect(() => {
    if (!perm.canOperate && user?.id && !applicantId) {
      setApplicantId(user.id);
    }
  }, [perm.canOperate, user, applicantId]);

  const resetCreateForm = () => {
    setSelectedAssets([]);
    setApplicantId("");
    setReason("");
    setVendor("");
    setTechnician("");
    setCheckoutDate("");
    setReturnDatePlanned("");
    setContainsSensitiveData(false);
    setVendorNdaRef("");
  };

  const handleCreate = async () => {
    if (!applicantId || selectedAssets.length === 0) return;
    setCreating(true);
    try {
      await apiClient.post("/api/maintenance-requests", {
        asset_ids: selectedAssets,
        applicant_id: applicantId,
        reason, vendor, technician,
        checkout_date: checkoutDate || null,
        return_date: returnDatePlanned || null,
        contains_sensitive_data: containsSensitiveData,
        vendor_nda_ref: vendorNdaRef,
      });
      setShowCreate(false);
      resetCreateForm();
      load();
    } catch (err: unknown) {
      const resp = (err as { response?: { data?: { error?: string } } })?.response?.data;
      await dialog.error(resp?.error || (err instanceof Error ? err.message : "建立失敗"));
    } finally { setCreating(false); }
  };

  const doAction = async (req: MaintenanceRequest, action: "sign" | "reject") => {
    const labels: Record<string, string> = { sign: "承辦人員簽核，確認受理此維修申請？" };
    if (action === "sign") {
      if (!(await dialog.confirm(labels.sign))) return;
      try {
        await apiClient.post(`/api/maintenance-requests/${req.id}/sign`);
        load();
      } catch (err) {
        await dialog.error("操作失敗: " + (err instanceof Error ? err.message : ""));
      }
      return;
    }
    setRejectTarget(req);
    setRejectReason("");
  };

  const openApprove = (req: MaintenanceRequest) => {
    setApproveTarget(req);
    setApproveCheckoutDate(req.checkout_date || new Date().toISOString().slice(0, 10));
    setApproveDataWiped(false);
    setApproveLoanerInfo("");
    setApproveLoanerProvidedDate("");
    setApproveLoanerSecurityChecked(false);
  };

  const confirmApprove = async () => {
    if (!approveTarget) return;
    try {
      await apiClient.post(`/api/maintenance-requests/${approveTarget.id}/approve`, {
        checkout_date: approveCheckoutDate || null,
        data_wiped_before_checkout: approveDataWiped,
        loaner_info: approveLoanerInfo,
        loaner_provided_date: approveLoanerProvidedDate || null,
        loaner_security_checked: approveLoanerSecurityChecked,
      });
      setApproveTarget(null);
      load();
    } catch (err) {
      await dialog.error("核准失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const openReturn = (req: MaintenanceRequest) => {
    setReturnTarget(req);
    setReturnDate(new Date().toISOString().slice(0, 10));
    setProcessNotes("");
    setLoanerReturnedDate(req.loaner_info ? new Date().toISOString().slice(0, 10) : "");
  };

  const confirmReturn = async () => {
    if (!returnTarget) return;
    try {
      await apiClient.post(`/api/maintenance-requests/${returnTarget.id}/return`, {
        return_date: returnDate || null,
        process_notes: processNotes,
        loaner_returned_date: loanerReturnedDate || null,
      });
      setReturnTarget(null);
      load();
    } catch (err) {
      await dialog.error("歸還失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const confirmReject = async () => {
    if (!rejectTarget) return;
    try {
      await apiClient.post(`/api/maintenance-requests/${rejectTarget.id}/reject`, { reason: rejectReason });
      setRejectTarget(null);
      load();
    } catch (err) {
      await dialog.error("拒絕失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const columnDefs = useMemo<ColDef<MaintenanceRequest>[]>(() => [
    { headerName: "申請單號", field: "request_number", width: 100, cellClass: "font-mono text-sm font-medium" },
    { headerName: "設備名稱", field: "asset_name", minWidth: 140, valueFormatter: (p) => p.value || "-" },
    { headerName: "設備編號", field: "asset_number", width: 130, cellClass: "font-mono text-xs" },
    { headerName: "申請人員", field: "applicant_name", width: 110 },
    { headerName: "申請原因", field: "reason", minWidth: 140, valueFormatter: (p) => p.value || "-" },
    { headerName: "維修廠商", field: "vendor", width: 120, valueFormatter: (p) => p.value || "-" },
    { headerName: "維修人員", field: "technician", width: 110, valueFormatter: (p) => p.value || "-" },
    { headerName: "攜出日期", field: "checkout_date", width: 110, valueFormatter: (p) => p.value || "-" },
    { headerName: "歸還日期", field: "return_date", width: 110, valueFormatter: (p) => p.value || "-" },
    {
      headerName: "狀態", field: "status", width: 120,
      cellRenderer: (p: ICellRendererParams<MaintenanceRequest>) => {
        const sc = statusConfig[p.value as string] || statusConfig.pending;
        return <span className={`badge badge-sm gap-1 ${sc.badge}`}>{sc.icon} {sc.label}</span>;
      },
    },
    { headerName: "承辦人員", field: "handler_name", width: 110, valueFormatter: (p) => p.value || "-" },
    { headerName: "權責主管", field: "supervisor_name", width: 110, valueFormatter: (p) => p.value || "-" },
    {
      headerName: "含機敏資料", field: "contains_sensitive_data", width: 110,
      cellRenderer: (p: ICellRendererParams<MaintenanceRequest>) =>
        p.value ? <span className="badge badge-sm badge-error">是</span> : <span className="badge badge-sm badge-ghost">否</span>,
    },
    { headerName: "替代機資訊", field: "loaner_info", minWidth: 140, valueFormatter: (p) => p.value || "-" },
    {
      headerName: "操作", colId: "actions", width: 220, pinned: "right", sortable: false, filter: false,
      cellRenderer: (p: ICellRendererParams<MaintenanceRequest>) => {
        const req = p.data!;
        return (
          <div className="flex gap-1 h-full items-center">
            {req.status === "pending" && perm.canOperate && (
              <>
                <button onClick={() => doAction(req, "sign")} className="btn btn-info btn-xs gap-1"><Check size={12} /> 承辦</button>
                <button onClick={() => doAction(req, "reject")} className="btn btn-error btn-xs gap-1"><AlertCircle size={12} /> 拒絕</button>
              </>
            )}
            {req.status === "handler_signed" && perm.canApprove && (
              <>
                <button onClick={() => openApprove(req)} className="btn btn-success btn-xs gap-1"><CheckCircle size={12} /> 核准</button>
                <button onClick={() => doAction(req, "reject")} className="btn btn-error btn-xs gap-1"><AlertCircle size={12} /> 拒絕</button>
              </>
            )}
            {req.status === "approved" && perm.canOperate && (
              <button onClick={() => openReturn(req)} className="btn btn-warning btn-xs gap-1"><RotateCcw size={12} /> 歸還</button>
            )}
          </div>
        );
      },
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [perm.canOperate, perm.canApprove]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">設備維修申請</h1>
          <p className="text-sm text-base-content/60">資通設備進出及維護申請單</p>
        </div>
        <div className="flex gap-2 flex-wrap">
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="select select-bordered select-sm">
            <option value="">全部狀態</option>
            <option value="pending">待承辦</option>
            <option value="handler_signed">待主管核准</option>
            <option value="approved">維修中</option>
            <option value="returned">已歸還</option>
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
              <Wrench size={14} /> 新增維修申請
            </button>
          )}
        </div>
      </div>

      {showCreate && (
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <div className="flex items-center justify-between mb-2">
              <h2 className="card-title text-base">新增設備維修申請</h2>
              <button onClick={() => { setShowCreate(false); resetCreateForm(); }} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
            </div>
            <div className="space-y-4">
              <div className="form-control">
                <label className="label"><span className="label-text font-medium">設備名稱及編號</span></label>
                <AssetPicker selected={selectedAssets} onChange={setSelectedAssets} showFilters endpoint="/api/pickable-assets" />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">申請人員</span></label>
                  {!perm.canOperate ? (
                    <input type="text" value={user?.display_name || user?.username || ""} className="input input-bordered input-sm" disabled />
                  ) : (
                    <select value={applicantId} onChange={(e) => setApplicantId(e.target.value)} className="select select-bordered select-sm">
                      <option value="">選擇使用者</option>
                      {users.map((u) => (
                        <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
                      ))}
                    </select>
                  )}
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">申請原因</span></label>
                  <input type="text" value={reason} onChange={(e) => setReason(e.target.value)} className="input input-bordered input-sm" placeholder="故障描述、送修原因" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">維修廠商</span></label>
                  <input type="text" value={vendor} onChange={(e) => setVendor(e.target.value)} className="input input-bordered input-sm" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">維修人員</span></label>
                  <input type="text" value={technician} onChange={(e) => setTechnician(e.target.value)} className="input input-bordered input-sm" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">預計攜出日期</span></label>
                  <input type="date" value={checkoutDate} onChange={(e) => setCheckoutDate(e.target.value)} className="input input-bordered input-sm" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">預計歸還日期</span></label>
                  <input type="date" value={returnDatePlanned} onChange={(e) => setReturnDatePlanned(e.target.value)} className="input input-bordered input-sm" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">廠商保密協議編號</span></label>
                  <input type="text" value={vendorNdaRef} onChange={(e) => setVendorNdaRef(e.target.value)} className="input input-bordered input-sm" placeholder="選填，作為稽核佐證" />
                </div>
                <div className="form-control justify-end">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input type="checkbox" className="checkbox checkbox-sm" checked={containsSensitiveData} onChange={(e) => setContainsSensitiveData(e.target.checked)} />
                    <span className="label-text font-medium">設備內含機敏/個資資料</span>
                  </label>
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={handleCreate} disabled={creating || !applicantId || selectedAssets.length === 0} className="btn btn-success btn-sm gap-1">
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
        <DataGrid<MaintenanceRequest>
          rowData={requests}
          columnDefs={columnDefs}
          loading={loading}
          getRowId={(p) => p.data.id}
          overlayNoRowsTemplate={`<span class="opacity-50">尚無維修申請記錄</span>`}
          getRowClass={(p) => p.data?.is_archived ? "opacity-50" : ""}
        />
      </div>

      {/* Approve dialog */}
      <dialog className={`modal ${approveTarget ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg">權責主管核准</h3>
          <p className="text-sm text-base-content/60 mt-1">核准後裝置將標記為維修中；以下為送修前的資安檢核</p>

          <div className="form-control mt-4">
            <label className="label"><span className="label-text text-sm">攜出日期</span></label>
            <input type="date" value={approveCheckoutDate} onChange={(e) => setApproveCheckoutDate(e.target.value)} className="input input-bordered input-sm" />
          </div>

          <label className="flex items-center gap-3 cursor-pointer p-2 mt-3 rounded hover:bg-base-200">
            <input
              type="checkbox"
              className="checkbox checkbox-sm checkbox-success"
              checked={approveDataWiped}
              onChange={(e) => setApproveDataWiped(e.target.checked)}
            />
            <span className="text-sm">送修前已備份／已清除機敏資料</span>
          </label>

          <div className="divider my-2 text-xs opacity-50">廠商替代機（若有提供）</div>

          <div className="form-control">
            <label className="label"><span className="label-text text-sm">替代機資訊（廠牌型號/序號）</span></label>
            <input type="text" value={approveLoanerInfo} onChange={(e) => setApproveLoanerInfo(e.target.value)} className="input input-bordered input-sm" placeholder="無提供則留空" />
          </div>
          {approveLoanerInfo && (
            <>
              <div className="form-control mt-3">
                <label className="label"><span className="label-text text-sm">替代機提供日期</span></label>
                <input type="date" value={approveLoanerProvidedDate} onChange={(e) => setApproveLoanerProvidedDate(e.target.value)} className="input input-bordered input-sm" />
              </div>
              <label className="flex items-center gap-3 cursor-pointer p-2 mt-2 rounded hover:bg-base-200">
                <input
                  type="checkbox"
                  className="checkbox checkbox-sm checkbox-success"
                  checked={approveLoanerSecurityChecked}
                  onChange={(e) => setApproveLoanerSecurityChecked(e.target.checked)}
                />
                <span className="text-sm">替代機已完成資安檢查（掃毒／病毒防護確認）</span>
              </label>
            </>
          )}

          <div className="modal-action">
            <button className="btn btn-sm" onClick={() => setApproveTarget(null)}>取消</button>
            <button className="btn btn-success btn-sm gap-1" onClick={confirmApprove}>
              <CheckCircle size={14} /> 確認核准
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setApproveTarget(null)}>close</button>
        </form>
      </dialog>

      {/* Return dialog */}
      <dialog className={`modal ${returnTarget ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg">設備歸還登記</h3>
          <p className="text-sm text-base-content/60 mt-1">記錄歸還日期與作業過程</p>
          <div className="form-control mt-4">
            <label className="label"><span className="label-text text-sm">歸還日期</span></label>
            <input type="date" value={returnDate} onChange={(e) => setReturnDate(e.target.value)} className="input input-bordered input-sm" />
          </div>
          {returnTarget?.loaner_info && (
            <div className="form-control mt-3">
              <label className="label"><span className="label-text text-sm">替代機收回日期（{returnTarget.loaner_info}）</span></label>
              <input type="date" value={loanerReturnedDate} onChange={(e) => setLoanerReturnedDate(e.target.value)} className="input input-bordered input-sm" />
            </div>
          )}
          <div className="form-control mt-3">
            <label className="label"><span className="label-text text-sm">作業過程</span></label>
            <textarea
              value={processNotes}
              onChange={(e) => setProcessNotes(e.target.value)}
              placeholder="記錄維修內容、更換零件等"
              className="textarea textarea-bordered textarea-sm"
              rows={3}
            />
          </div>
          <div className="modal-action">
            <button className="btn btn-sm" onClick={() => setReturnTarget(null)}>取消</button>
            <button className="btn btn-warning btn-sm gap-1" onClick={confirmReturn}>
              <RotateCcw size={14} /> 確認歸還
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setReturnTarget(null)}>close</button>
        </form>
      </dialog>

      {/* Reject dialog */}
      <dialog className={`modal ${rejectTarget ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg">拒絕維修申請</h3>
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
