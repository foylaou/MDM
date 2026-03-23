import { useState, useEffect, useMemo } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { DevicePicker } from "../components/DevicePicker";
import apiClient from "../lib/apiClient";
import { useDialog } from "../components/DialogProvider";
import * as XLSX from "xlsx";
import {
  Check, X, RotateCcw, Play, UserPlus, Clock,
  CheckCircle, AlertCircle, ArrowRight, FileDown, Archive,
  ChevronDown, ChevronRight,
} from "lucide-react";

interface ReturnChecklist {
  deviceReceived?: boolean;
  screenOk?: boolean;
  bodyOk?: boolean;
  canPowerOn?: boolean;
  accessoriesOk?: boolean;
}

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
  rental_number: number;
  is_archived: boolean;
  return_checklist?: ReturnChecklist;
  return_notes?: string;
}

interface RentalGroup {
  rental_number: number;
  rentals: Rental[];
  borrower_name: string;
  purpose: string;
  status: string;
  borrow_date: string;
  expected_return?: string;
  actual_return?: string;
  approver_name: string;
  is_archived: boolean;
  custodian_name: string;
  custodian_id?: string;
  return_checklist?: ReturnChecklist;
  return_notes?: string;
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

function groupByRentalNumber(rentals: Rental[]): RentalGroup[] {
  const map = new Map<number, Rental[]>();
  for (const r of rentals) {
    const list = map.get(r.rental_number) || [];
    list.push(r);
    map.set(r.rental_number, list);
  }
  const groups: RentalGroup[] = [];
  for (const [num, items] of map) {
    const first = items[0];
    groups.push({
      rental_number: num,
      rentals: items,
      borrower_name: first.borrower_name,
      purpose: first.purpose,
      status: first.status,
      borrow_date: first.borrow_date,
      expected_return: first.expected_return,
      actual_return: first.actual_return,
      approver_name: first.approver_name,
      is_archived: first.is_archived,
      custodian_name: first.custodian_name,
      custodian_id: first.custodian_id,
      return_checklist: first.return_checklist,
      return_notes: first.return_notes,
    });
  }
  groups.sort((a, b) => b.rental_number - a.rental_number);
  return groups;
}

const checklistLabels: Record<string, string> = {
  deviceReceived: "已收到裝置",
  screenOk: "螢幕完好",
  bodyOk: "機身完好",
  canPowerOn: "可正常開機",
  accessoriesOk: "配件齊全",
};

function buildExcelRows(groups: RentalGroup[]) {
  const rows: Record<string, unknown>[] = [];
  for (const g of groups) {
    const sc = statusConfig[g.status] || statusConfig.pending;
    const deviceNames = g.rentals.map((r) => r.device_name || r.device_serial).join("、");
    const deviceSerials = g.rentals.map((r) => r.device_serial).join("、");
    const cl = g.return_checklist;
    const row: Record<string, unknown> = {
      "單號": g.rental_number,
      "裝置數": g.rentals.length,
      "裝置名稱": deviceNames,
      "裝置序號": deviceSerials,
      "借用人": g.borrower_name,
      "保管人": g.custodian_name || "",
      "用途": g.purpose,
      "狀態": sc.label,
      "借出日期": g.borrow_date ? new Date(g.borrow_date).toLocaleDateString() : "",
      "預計歸還": g.expected_return || "",
      "實際歸還": g.actual_return ? new Date(g.actual_return).toLocaleDateString() : "",
      "核准人": g.approver_name || "",
      "備註": g.rentals[0]?.notes || "",
    };
    // Return checklist columns
    for (const [key, label] of Object.entries(checklistLabels)) {
      row[`歸還清點-${label}`] = cl?.[key as keyof ReturnChecklist] ? "V" : "";
    }
    row["歸還備註"] = g.return_notes || "";
    row["存查"] = g.is_archived ? "是" : "";
    rows.push(row);
  }
  return rows;
}

function downloadExcel(rows: Record<string, unknown>[], filename: string) {
  const ws = XLSX.utils.json_to_sheet(rows);
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, "租借記錄");
  XLSX.writeFile(wb, filename);
}

export function Rentals() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const dialog = useDialog();
  const [rentals, setRentals] = useState<Rental[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [statusFilter, setStatusFilter] = useState("");
  const [showArchived, setShowArchived] = useState(false);

  // Selection — by rental_number
  const [selectedNumbers, setSelectedNumbers] = useState<Set<number>>(new Set());
  // Expanded groups
  const [expandedNumbers, setExpandedNumbers] = useState<Set<number>>(new Set());

  // Create form
  const [selectedDevices, setSelectedDevices] = useState<string[]>([]);
  const [borrowerId, setBorrowerId] = useState("");
  const [purpose, setPurpose] = useState("");
  const [expectedReturn, setExpectedReturn] = useState("");
  const [notes, setNotes] = useState("");
  const [creating, setCreating] = useState(false);

  const groups = useMemo(() => groupByRentalNumber(rentals), [rentals]);

  const loadRentals = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get("/api/rentals", {
        params: { status: statusFilter, show_archived: showArchived ? "true" : "" },
      });
      setRentals(data.rentals || []);
      setSelectedNumbers(new Set());
    } catch (err) { console.error("Load rentals:", err); }
    finally { setLoading(false); }
  };

  const loadUsers = async () => {
    try {
      const { data } = await apiClient.get("/api/users-list");
      setUsers(data.users || []);
    } catch { /* */ }
  };

  useEffect(() => { loadRentals(); }, [statusFilter, showArchived]);
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
    } catch (err: unknown) {
      const resp = (err as { response?: { data?: { error?: string; devices?: string[] } } })?.response?.data;
      await dialog.error(resp?.error || (err instanceof Error ? err.message : "建立失敗"), resp?.devices || []);
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

  const doAction = async (rentalId: string, action: string) => {
    if (action === "return") {
      setReturnRentalId(rentalId);
      setChecklist({ deviceReceived: false, screenOk: false, bodyOk: false, canPowerOn: false, accessoriesOk: false });
      setReturnNotes("");
      return;
    }
    const labels: Record<string, string> = {
      approve: "核准此租借申請（整批）？",
      activate: "確認借出裝置（整批）？",
      reject: "拒絕此租借申請（整批）？",
    };
    if (!(await dialog.confirm(labels[action] || `${action}?`))) return;
    try {
      await apiClient.post(`/api/rentals/${rentalId}/${action}`);
      loadRentals();
    } catch (err) {
      await dialog.error("操作失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const confirmReturn = async () => {
    if (!returnRentalId) return;
    try {
      await apiClient.post(`/api/rentals/${returnRentalId}/return`, {
        notes: returnNotes,
        checklist,
      });
      setReturnRentalId(null);
      loadRentals();
    } catch (err) {
      await dialog.error("歸還失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  const isAdmin = user?.role === "admin";
  const isViewer = user?.role === "viewer";
  const canExport = user?.role === "admin" || user?.role === "operator";

  useEffect(() => {
    if (isViewer && user?.id && !borrowerId) {
      setBorrowerId(user.id);
    }
  }, [isViewer, user]);

  const canApprove = (group: RentalGroup) => {
    if (isAdmin) return true;
    if (group.custodian_id && group.custodian_id === user?.id) return true;
    return false;
  };

  // Selection helpers
  const toggleSelect = (num: number) => {
    setSelectedNumbers((prev) => {
      const next = new Set(prev);
      if (next.has(num)) next.delete(num); else next.add(num);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedNumbers.size === groups.length) {
      setSelectedNumbers(new Set());
    } else {
      setSelectedNumbers(new Set(groups.map((g) => g.rental_number)));
    }
  };

  const toggleExpand = (num: number) => {
    setExpandedNumbers((prev) => {
      const next = new Set(prev);
      if (next.has(num)) next.delete(num); else next.add(num);
      return next;
    });
  };

  const selectedGroups = useMemo(
    () => groups.filter((g) => selectedNumbers.has(g.rental_number)),
    [groups, selectedNumbers],
  );

  // Export
  const handleExport = () => {
    const target = selectedGroups.length > 0 ? selectedGroups : groups;
    if (target.length === 0) return;
    const rows = buildExcelRows(target);
    const now = new Date().toISOString().slice(0, 10);
    downloadExcel(rows, `租借記錄_${now}.xlsx`);
  };

  // Export + archive
  const handleExportAndArchive = async () => {
    if (selectedGroups.length === 0) {
      await dialog.alert("請先勾選要存查的記錄");
      return;
    }
    const rows = buildExcelRows(selectedGroups);
    const now = new Date().toISOString().slice(0, 10);
    downloadExcel(rows, `租借記錄_存查_${now}.xlsx`);

    // Get all rental IDs from selected groups
    const allIds = selectedGroups.flatMap((g) => g.rentals.map((r) => r.id));
    try {
      await apiClient.post("/api/rentals-archive", { ids: allIds });
      loadRentals();
    } catch (err) {
      await dialog.error("標記存查失敗: " + (err instanceof Error ? err.message : ""));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">租借管理</h1>
          <p className="text-sm text-base-content/60">裝置借出、歸還與追蹤</p>
        </div>
        <div className="flex gap-2 flex-wrap">
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="select select-bordered select-sm" data-tour="rental-filter">
            <option value="">全部狀態</option>
            <option value="pending">待核准</option>
            <option value="approved">已核准</option>
            <option value="active">借出中</option>
            <option value="returned">已歸還</option>
            <option value="rejected">已拒絕</option>
          </select>
          <label className="flex items-center gap-1.5 cursor-pointer text-sm">
            <input
              type="checkbox"
              className="checkbox checkbox-xs"
              checked={showArchived}
              onChange={(e) => setShowArchived(e.target.checked)}
            />
            顯示存查
          </label>
          <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm gap-1" data-tour="rental-create">
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
      <div className="flex items-center gap-2 text-xs text-base-content/50 px-1" data-tour="rental-workflow">
        <span className="badge badge-warning badge-xs">待核准</span>
        <span className="text-base-content/30">保管人或管理員核准</span>
        <ArrowRight size={12} />
        <span className="badge badge-info badge-xs">已核准</span>
        <ArrowRight size={12} />
        <span className="badge badge-success badge-xs">借出中</span>
        <ArrowRight size={12} />
        <span className="badge badge-ghost badge-xs">已歸還</span>
      </div>

      {/* Export buttons */}
      {canExport && (
        <div className="flex gap-2 items-center">
          <button onClick={handleExport} disabled={groups.length === 0} className="btn btn-outline btn-sm gap-1">
            <FileDown size={14} /> 匯出記錄
            {selectedNumbers.size > 0 && <span className="badge badge-sm badge-primary">{selectedNumbers.size}</span>}
          </button>
          <button onClick={handleExportAndArchive} disabled={selectedNumbers.size === 0} className="btn btn-secondary btn-sm gap-1">
            <Archive size={14} /> 匯出記錄並存查
            {selectedNumbers.size > 0 && <span className="badge badge-sm">{selectedNumbers.size}</span>}
          </button>
          {selectedNumbers.size > 0 && (
            <span className="text-sm text-base-content/60">
              已選 {selectedNumbers.size} 筆
            </span>
          )}
        </div>
      )}

      {/* Table */}
      <div className="card bg-base-100 shadow" data-tour="rental-table">
        <div className="overflow-x-auto">
          <table className="table table-sm">
            <thead>
              <tr>
                {canExport && (
                  <th>
                    <input
                      type="checkbox"
                      className="checkbox checkbox-xs"
                      checked={groups.length > 0 && selectedNumbers.size === groups.length}
                      onChange={toggleSelectAll}
                    />
                  </th>
                )}
                <th></th>
                <th>單號</th>
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
                <tr><td colSpan={canExport ? 12 : 11} className="text-center py-8"><span className="loading loading-spinner loading-md"></span></td></tr>
              ) : groups.length === 0 ? (
                <tr><td colSpan={canExport ? 12 : 11} className="text-center py-8 text-base-content/50">尚無租借記錄</td></tr>
              ) : groups.map((g) => {
                const sc = statusConfig[g.status] || statusConfig.pending;
                const isExpanded = expandedNumbers.has(g.rental_number);
                const hasMultiple = g.rentals.length > 1;
                const firstRentalId = g.rentals[0].id;
                return (
                  <>{/* Group header row */}
                  <tr key={g.rental_number} className={`hover ${g.is_archived ? "opacity-50" : ""}`}>
                    {canExport && (
                      <td>
                        <input
                          type="checkbox"
                          className="checkbox checkbox-xs"
                          checked={selectedNumbers.has(g.rental_number)}
                          onChange={() => toggleSelect(g.rental_number)}
                        />
                      </td>
                    )}
                    <td className="w-6">
                      {hasMultiple && (
                        <button onClick={() => toggleExpand(g.rental_number)} className="btn btn-ghost btn-xs btn-circle">
                          {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                        </button>
                      )}
                    </td>
                    <td className="font-mono text-sm font-medium">
                      {g.rental_number}
                      {g.is_archived && <span className="badge badge-xs badge-ghost ml-1">存查</span>}
                    </td>
                    <td>
                      {hasMultiple ? (
                        <div>
                          <span className="font-medium">{g.rentals[0].device_name || g.rentals[0].device_serial}</span>
                          <span className="badge badge-sm badge-outline ml-1">共 {g.rentals.length} 台</span>
                        </div>
                      ) : (
                        <div>
                          <div className="font-medium">{g.rentals[0].device_name || g.rentals[0].device_serial}</div>
                          <div className="text-xs opacity-50 font-mono">{g.rentals[0].device_serial}</div>
                        </div>
                      )}
                    </td>
                    <td className="font-medium">{g.borrower_name}</td>
                    <td className="text-sm opacity-70">{g.custodian_name || <span className="opacity-30">-</span>}</td>
                    <td className="text-sm">{g.purpose || "-"}</td>
                    <td>
                      <span className={`badge badge-sm gap-1 ${sc.badge}`}>
                        {sc.icon} {sc.label}
                      </span>
                    </td>
                    <td className="text-sm opacity-70">{new Date(g.borrow_date).toLocaleDateString()}</td>
                    <td className="text-sm opacity-70">{g.expected_return || "-"}</td>
                    <td className="text-sm">{g.approver_name || "-"}</td>
                    <td>
                      <div className="flex gap-1">
                        {g.status === "pending" && canApprove(g) && (
                          <>
                            <button onClick={() => doAction(firstRentalId, "approve")} className="btn btn-success btn-xs gap-1"><CheckCircle size={12} /> 核准</button>
                            <button onClick={() => doAction(firstRentalId, "reject")} className="btn btn-error btn-xs gap-1"><AlertCircle size={12} /> 拒絕</button>
                          </>
                        )}
                        {g.status === "approved" && isAdmin && (
                          <button onClick={() => doAction(firstRentalId, "activate")} className="btn btn-primary btn-xs gap-1"><Play size={12} /> 借出</button>
                        )}
                        {g.status === "active" && canApprove(g) && (
                          <button onClick={() => doAction(firstRentalId, "return")} className="btn btn-warning btn-xs gap-1"><RotateCcw size={12} /> 歸還</button>
                        )}
                      </div>
                    </td>
                  </tr>
                  {/* Expanded device rows */}
                  {isExpanded && g.rentals.map((r, idx) => (
                    <tr key={r.id} className={`bg-base-200/50 ${g.is_archived ? "opacity-50" : ""}`}>
                      {canExport && <td></td>}
                      <td></td>
                      <td className="text-xs opacity-50 text-right">{idx + 1}</td>
                      <td>
                        <div className="font-medium text-sm">{r.device_name || r.device_serial}</div>
                        <div className="text-xs opacity-50 font-mono">{r.device_serial}</div>
                      </td>
                      <td colSpan={8}></td>
                    </tr>
                  ))}
                  </>
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
          <p className="text-sm text-base-content/60 mt-1">請確認以下項目後完成歸還（整批裝置）</p>

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
