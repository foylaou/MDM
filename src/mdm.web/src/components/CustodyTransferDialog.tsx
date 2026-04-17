import { useState, useEffect, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { ArrowRightLeft, X } from "lucide-react";
import apiClient from "../lib/apiClient";
import { useDialog } from "./DialogProvider";

interface UserOption {
  id: string;
  username: string;
  display_name: string;
}

interface CustodyTransferDialogProps {
  open: boolean;
  assetId: string;
  assetName: string;
  assetNumber: string;
  currentCustodianId: string | null;
  currentCustodianName: string;
  onClose: () => void;
  onDone: () => void;
}

export function CustodyTransferDialog({
  open, assetId, assetName, assetNumber,
  currentCustodianId, currentCustodianName,
  onClose, onDone,
}: CustodyTransferDialogProps) {
  const { t } = useTranslation();
  const dialog = useDialog();
  const [users, setUsers] = useState<UserOption[]>([]);
  const [mode, setMode] = useState<"transfer" | "revoke">("transfer");
  const [toUserId, setToUserId] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      apiClient.get("/api/users-list").then(({ data }) => setUsers(data.users || [])).catch(() => {});
      setToUserId("");
      setReason("");
      setMode(currentCustodianId ? "transfer" : "transfer");
    }
  }, [open, currentCustodianId]);

  const action = currentCustodianId ? (mode === "revoke" ? "revoke" : "transfer") : "assign";

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (action !== "revoke" && !toUserId) {
      await dialog.error(t("custody.errorSelectUser"));
      return;
    }
    if (!reason.trim()) {
      await dialog.error(t("custody.errorReasonRequired"));
      return;
    }
    setSubmitting(true);
    try {
      await apiClient.post("/api/assets-custody", {
        action, asset_id: assetId, to_user_id: toUserId, reason: reason.trim(),
      });
      onDone();
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "";
      await dialog.error(t("custody.errorSubmit") + (msg ? `: ${msg}` : ""));
    } finally {
      setSubmitting(false);
    }
  };

  if (!open) return null;

  return (
    <dialog className="modal modal-open">
      <div className="modal-box">
        <div className="flex items-center justify-between mb-4">
          <h3 className="font-bold text-lg flex items-center gap-2">
            <ArrowRightLeft size={18} />
            {currentCustodianId ? t("custody.transferTitle") : t("custody.assignTitle")}
          </h3>
          <button onClick={onClose} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3">
          <div className="bg-base-200 rounded-lg p-3 text-sm space-y-1">
            <div><span className="opacity-60">{t("custody.asset")}：</span><span className="font-medium">{assetName}</span> <span className="font-mono text-xs opacity-60">({assetNumber})</span></div>
            <div><span className="opacity-60">{t("custody.currentCustodian")}：</span><span className="font-medium">{currentCustodianName || t("custody.none")}</span></div>
          </div>

          {currentCustodianId && (
            <div className="join w-full">
              <button
                type="button"
                onClick={() => setMode("transfer")}
                className={`join-item btn btn-sm flex-1 ${mode === "transfer" ? "btn-primary" : "btn-ghost"}`}
              >
                {t("custody.modeTransfer")}
              </button>
              <button
                type="button"
                onClick={() => setMode("revoke")}
                className={`join-item btn btn-sm flex-1 ${mode === "revoke" ? "btn-warning" : "btn-ghost"}`}
              >
                {t("custody.modeRevoke")}
              </button>
            </div>
          )}

          {action !== "revoke" && (
            <div className="form-control">
              <label className="label py-1"><span className="label-text text-xs">{t("custody.newCustodian")}</span></label>
              <select
                value={toUserId}
                onChange={(e) => setToUserId(e.target.value)}
                className="select select-bordered select-sm"
                required
              >
                <option value="">{t("custody.selectUser")}</option>
                {users.filter((u) => u.id !== currentCustodianId).map((u) => (
                  <option key={u.id} value={u.id}>{u.display_name || u.username}</option>
                ))}
              </select>
            </div>
          )}

          <div className="form-control">
            <label className="label py-1"><span className="label-text text-xs">{t("custody.reason")}</span></label>
            <input
              type="text"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("custody.reasonPlaceholder")}
              className="input input-bordered input-sm"
              required
            />
          </div>

          <div className="modal-action">
            <button type="button" onClick={onClose} className="btn btn-ghost btn-sm">{t("common.cancel")}</button>
            <button
              type="submit"
              disabled={submitting}
              className={`btn btn-sm ${action === "revoke" ? "btn-warning" : "btn-primary"}`}
            >
              {submitting && <span className="loading loading-spinner loading-xs"></span>}
              {action === "revoke" ? t("custody.confirmRevoke") : t("custody.confirmTransfer")}
            </button>
          </div>
        </form>
      </div>
      <form method="dialog" className="modal-backdrop"><button type="button" onClick={onClose}>close</button></form>
    </dialog>
  );
}
