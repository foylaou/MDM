import { useState, type FormEvent } from "react";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { Lock, X } from "lucide-react";

interface ChangePasswordProps {
  open: boolean;
  onClose: () => void;
}

export function ChangePassword({ open, onClose }: ChangePasswordProps) {
  const { t } = useTranslation();
  const { clients } = useAuthStore();
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const [loading, setLoading] = useState(false);

  const reset = () => {
    setOldPassword(""); setNewPassword(""); setConfirmPassword("");
    setError(""); setSuccess(false);
  };

  const handleClose = () => { reset(); onClose(); };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    if (newPassword !== confirmPassword) { setError(t("setup.passwordMismatch")); return; }
    if (newPassword.length < 6) { setError(t("setup.passwordTooShort")); return; }
    if (!clients) return;

    setLoading(true);
    try {
      await clients.auth.changePassword({ oldPassword, newPassword });
      setSuccess(true);
      setTimeout(handleClose, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally { setLoading(false); }
  };

  if (!open) return null;

  return (
    <dialog className="modal modal-open">
      <div className="modal-box">
        <div className="flex items-center justify-between mb-4">
          <h3 className="font-bold text-lg flex items-center gap-2"><Lock size={18} /> {t("changePassword.title")}</h3>
          <button onClick={handleClose} className="btn btn-ghost btn-sm btn-circle"><X size={16} /></button>
        </div>

        {success ? (
          <div role="alert" className="alert alert-success">{t("changePassword.success")}</div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && <div role="alert" className="alert alert-error py-2"><span>{error}</span></div>}
            <div className="form-control">
              <label className="label"><span className="label-text">{t("changePassword.oldPassword")}</span></label>
              <input type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} className="input input-bordered" required />
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text">{t("changePassword.newPassword")}</span></label>
              <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="input input-bordered" required minLength={6} />
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text">{t("setup.confirmPassword")}</span></label>
              <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} className="input input-bordered" required minLength={6} />
            </div>
            <div className="modal-action">
              <button type="button" onClick={handleClose} className="btn">{t("common.cancel")}</button>
              <button type="submit" disabled={loading} className="btn btn-primary">
                {loading && <span className="loading loading-spinner loading-sm"></span>}
                {t("common.save")}
              </button>
            </div>
          </form>
        )}
      </div>
      <form method="dialog" className="modal-backdrop"><button onClick={handleClose}>close</button></form>
    </dialog>
  );
}
