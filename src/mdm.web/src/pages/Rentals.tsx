import { useState, useEffect } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { DevicePicker } from "../components/DevicePicker";
import apiClient from "../lib/apiClient";
import {
  Check, X, RotateCcw, Play, UserPlus, Clock,
  CheckCircle, AlertCircle, ArrowRight,
} from "lucide-react";

interface Rental {
  id: string;
  device_udid: string;
  borrower_id: string;
  borrower_name: string;
  approver_id?: string;
  approver_name: string;
  custodian_id?: string;
  custodian_name: string;
  status: string;
  purpose: string;
  borrow_date: string;
  expected_return?: string;
  actual_return?: string;
  notes: string;
  device_name: string;
  device_serial: string;
}

interface UserOption {
  id: string;
  username: string;
  display_name: string;
}

const statusConfig: Record<string, { label: string; badge: string; icon: React.ReactNode }> = {
  pending:  { label: "待核准", badge: "badge-warning",  icon: <Clock size={14} /> },
  approved: { label: "已核准", badge: "badge-info",     icon: <Check size={14} /> },
  active:   { label: "借出中", badge: "badge-success",  icon: <Play size={14} /> },
  returned: { label: "已歸還", badge: "badge-ghost",    icon: <RotateCcw size={14} /> },
  rejected: { label: "已拒絕", badge: "badge-error",    icon: <X size={14} /> },
};

export function Rentals() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const [rentals, setRentals] = useState<Rental[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [statusFilter, setStatusFilter] = useState("");

  // Create form
  const [selectedDevices, setSelectedDevices] = useState<string[]>([]);
  const [borrowerId, setBorrowerId] = useState("");
  const [purpose, setPurpose] = useState("");
  const [expectedReturn, setExpectedReturn] = useState("");
  const [notes, setNotes] = useState("");
  const [creating, setCreating] = useState(false);

  const loadRentals = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/rentals", { params: { status: statusFilter } });
      setRentals(data.rentals || []);
    } catch (err) { console.error("Load rentals:", err); }
    finally { setLoading(false); }
  };

  const loadUsers = async () => {
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch { /* */ }
  };

  useEffect(() => { loadRentals(); }, [statusFilter]);
  useEffect(() => { loadUsers(); }, []);

  const handleCreate = async () => {
    if (!borrowerId || selectedDevices.length === 0) return;
    setCreating(true);
    try {
      await apiClient.post("/api/rentals", {
        device_udids: selectedDevices,
        borrower_id: borrowerId,
        purpose,
        expected_return: expectedReturn || null,
        notes,
      });
      setShowCreate(false);
      setSelectedDevices([]);
      setBorrowerId("");
      setPurpose("");
      setExpectedReturn("");
      setNotes("");
      loadRentals();
    } catch (err) {
      alert("建立失敗: " + (err instanceof Error ? err.message : ""));
    } finally { setCreating(false); }
  };

  // Return dialog state
  const [returnRentalId, setReturnRentalId] = useState<string | null>(null);
  const [checklist, setChecklist] = useState({
    deviceReceived: false,
    screenOk: false,
    bodyOk: false,
    canPowerOn: false,
    accessoriesOk: false,
  });
  const [returnNotes, setReturnNotes] = useState("");

  const allChecked = Object.values(checklist).every(Boolean);

  const doAction = async (id: string, action: string) => {
    if (action === "return") {
      setReturnRentalId(id);
      setChecklist({ deviceReceived: false, screenOk: false, bodyOk: false, canPowerOn: false, accessoriesOk: false });
      setReturnNotes("");
      return;
    }
    const labels: Record<string, string> = {
      approve: "核准此租借申請？",
      activate: "確認借出裝置？",
      reject: "拒絕此租借申請？",
    };
    if (!confirm(labels[action] || `${action}?`)) return;
    try {
      await apiClient.post(`/api/rentals/${id}/${action}`);
      loadRentals();
    } catch (err) {
      alert("操作失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const confirmReturn = async () => {
    if (!returnRentalId) return;
    try {
      await apiClient.post(`/api/rentals/${returnRentalId}/return`, { notes: returnNotes });
      setReturnRentalId(null);
      loadRentals();
    } catch (err) {
      alert("歸還失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const isAdmin = user?.role === "admin";
  const isViewer = user?.role === "viewer";

  // Viewer: auto-set borrower to self
  useEffect(() => {
    if (isViewer && user?.id && !borrowerId) {
      setBorrowerId(user.id);
    }
  }, [isViewer, user]);

  // Can this user approve? Admin or current custodian of the device
  const canApprove = (rental: Rental) => {
    if (isAdmin) return true;
    if (rental.custodian_id && rental.custodian_id === user?.id) return true;
    return false;
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">租借管理</h1>
          <p className="text-sm text-base-content/60">裝置借出、歸還與追蹤</p>
        </div>
        <div className="flex gap-2">
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="select select-bordered select-sm">
            <option value="">全部狀態</option>
            <option value="pending">待核准</option>
            <option value="approved">已核准</option>
            <option value="active">借出中</option>
            <option value="returned">已歸還</option>
            <option value="rejected">已拒絕</option>
          </select>
          <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm gap-1">
            <UserPlus size={14} /> 新增租借
          </button>
        </div>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <h2 className="card-title text-base">新增租借申請</h2>
            <div className="space-y-4 mt-2">
              <div className="form-control">
                <label className="label"><span className="label-text font-medium">選擇裝置</span></label>
                <DevicePicker selected={selectedDevices} onChange={setSelectedDevices} showFilters />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">借用人</span></label>
                  {isViewer ? (
                    <input type="text" value={user?.display_name || user?.username || ""} className="input input-bordered input-sm" disabled />
                  ) : (
                    <select value={borrowerId} onChange={(e) => setBorrowerId(e.target.value)} className="select select-bordered select-sm">
                      <option value="">選擇使用者</option>
                      {users.map((u) => (
                        <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
                      ))}
                    </select>
                  )}
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">預計歸還日期</span></label>
                  <input type="date" value={expectedReturn} onChange={(e) => setExpectedReturn(e.target.value)} className="input input-bordered input-sm" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">用途</span></label>
                  <input type="text" value={purpose} onChange={(e) => setPurpose(e.target.value)} className="input input-bordered input-sm" placeholder="借用用途" />
                </div>
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium">備註</span></label>
                  <input type="text" value={notes} onChange={(e) => setNotes(e.target.value)} className="input input-bordered input-sm" placeholder="其他備註" />
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={handleCreate} disabled={creating || !borrowerId || selectedDevices.length === 0} className="btn btn-success btn-sm gap-1">
                  {creating && <span className="loading loading-spinner loading-xs"></span>}
                  提交申請
                </button>
                <button onClick={() => setShowCreate(false)} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Workflow */}
      <div className="flex items-center gap-2 text-xs text-base-content/50 px-1">
        <span className="badge badge-warning badge-xs">待核准</span>
        <span className="text-base-content/30">保管人或管理員核准</span>
        <ArrowRight size={12} />
        <span className="badge badge-info badge-xs">已核准</span>
        <ArrowRight size={12} />
        <span className="badge badge-success badge-xs">借出中</span>
        <ArrowRight size={12} />
        <span className="badge badge-ghost badge-xs">已歸還</span>
      </div>

      {/* Table */}
      <div className="card bg-base-100 shadow">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>裝置</th>
                <th>借用人</th>
                <th>保管人</th>
                <th>用途</th>
                <th>狀態</th>
                <th>借出日期</th>
                <th>預計歸還</th>
                <th>核准人</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={9} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : rentals.length === 0 ? (
                <tr><td colSpan={9} className="text-center py-8 text-base-content/50">尚無租借記錄</td></tr>
              ) : rentals.map((r) => {
                const sc = statusConfig[r.status] || statusConfig.pending;
                return (
                  <tr key={r.id} className="hover">
                    <td>
                      <div className="font-medium">{r.device_name || r.device_serial}</div>
                      <div className="text-xs opacity-50 font-mono">{r.device_serial}</div>
                    </td>
                    <td className="font-medium">{r.borrower_name}</td>
                    <td className="text-sm opacity-70">{r.custodian_name || <span className="opacity-30">-</span>}</td>
                    <td className="text-sm">{r.purpose || "-"}</td>
                    <td>
                      <span className={`badge badge-sm gap-1 ${sc.badge}`}>
                        {sc.icon} {sc.label}
                      </span>
                    </td>
                    <td className="text-sm opacity-70">{new Date(r.borrow_date).toLocaleDateString()}</td>
                    <td className="text-sm opacity-70">{r.expected_return || "-"}</td>
                    <td className="text-sm">{r.approver_name || "-"}</td>
                    <td>
                      <div className="flex gap-1">
                        {r.status === "pending" && canApprove(r) && (
                          <>
                            <button onClick={() => doAction(r.id, "approve")} className="btn btn-success btn-xs gap-1"><CheckCircle size={12} /> 核准</button>
                            <button onClick={() => doAction(r.id, "reject")} className="btn btn-error btn-xs gap-1"><AlertCircle size={12} /> 拒絕</button>
                          </>
                        )}
                        {r.status === "approved" && isAdmin && (
                          <button onClick={() => doAction(r.id, "activate")} className="btn btn-primary btn-xs gap-1"><Play size={12} /> 借出</button>
                        )}
                        {r.status === "active" && canApprove(r) && (
                          <button onClick={() => doAction(r.id, "return")} className="btn btn-warning btn-xs gap-1"><RotateCcw size={12} /> 歸還</button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
      {/* Return checklist dialog */}
      <dialog className={`modal ${returnRentalId ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg">裝置歸還清點</h3>
          <p className="text-sm text-base-content/60 mt-1">請確認以下項目後完成歸還</p>

          <div className="space-y-3 mt-4">
            {[
              { key: "deviceReceived" as const, label: "已收到裝置" },
              { key: "screenOk" as const, label: "螢幕完好（無刮傷、裂痕）" },
              { key: "bodyOk" as const, label: "機身完好（無凹損、變形）" },
              { key: "canPowerOn" as const, label: "可正常開機使用" },
              { key: "accessoriesOk" as const, label: "配件齊全（充電線、保護套等）" },
            ].map((item) => (
              <label key={item.key} className="flex items-center gap-3 cursor-pointer p-2 rounded hover:bg-base-200">
                <input
                  type="checkbox"
                  className="checkbox checkbox-sm checkbox-success"
                  checked={checklist[item.key]}
                  onChange={(e) => setChecklist({ ...checklist, [item.key]: e.target.checked })}
                />
                <span className="text-sm">{item.label}</span>
              </label>
            ))}
          </div>

          <div className="form-control mt-4">
            <label className="label"><span className="label-text text-sm">備註（選填）</span></label>
            <textarea
              value={returnNotes}
              onChange={(e) => setReturnNotes(e.target.value)}
              placeholder="記錄裝置狀況、損壞情形等"
              className="textarea textarea-bordered textarea-sm"
              rows={2}
            />
          </div>

          {!allChecked && (
            <div role="alert" className="alert alert-warning mt-4 py-2">
              <span className="text-sm">請完成所有清點項目</span>
            </div>
          )}

          <div className="modal-action">
            <button className="btn btn-sm" onClick={() => setReturnRentalId(null)}>取消</button>
            <button className="btn btn-warning btn-sm gap-1" disabled={!allChecked} onClick={confirmReturn}>
              <RotateCcw size={14} /> 確認歸還
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setReturnRentalId(null)}>close</button>
        </form>
      </dialog>
    </div>
  );
}
