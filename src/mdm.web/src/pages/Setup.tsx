import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { User, Lock, UserCircle, ShieldCheck } from "lucide-react";

export function Setup() {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    if (password !== confirmPassword) { setError(t("setup.passwordMismatch")); return; }
    if (password.length < 6) { setError(t("setup.passwordTooShort")); return; }

    setLoading(true);
    try {
      const baseUrl = import.meta.env.DEV ? "" : window.location.origin;
      const resp = await fetch(`${baseUrl}/api/setup`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, display_name: displayName || username }),
      });
      if (!resp.ok) { const data = await resp.json(); throw new Error(data.error || "Setup failed"); }
      navigate("/login");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Setup failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-base-200 p-4">
      <div className="card bg-base-100 shadow-xl w-full max-w-lg">
        <div className="card-body">
          <div className="text-center mb-4">
            <div className="bg-success text-success-content w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <ShieldCheck size={32} />
            </div>
            <h1 className="text-2xl font-bold">{t("setup.title")}</h1>
            <p className="text-base-content/60 text-sm mt-1">{t("setup.subtitle")}</p>
          </div>

          <div role="alert" className="alert alert-info">
            <span className="text-sm">{t("setup.notice")}</span>
          </div>

          {error && <div role="alert" className="alert alert-error mt-2"><span>{error}</span></div>}

          <form onSubmit={handleSubmit} className="space-y-4 mt-4">
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">{t("setup.username")}</span></label>
              <label className="input input-bordered flex items-center gap-2">
                <User size={16} className="opacity-50" />
                <input type="text" className="grow" placeholder="admin" value={username} onChange={(e) => setUsername(e.target.value)} required />
              </label>
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">{t("setup.displayName")}</span></label>
              <label className="input input-bordered flex items-center gap-2">
                <UserCircle size={16} className="opacity-50" />
                <input type="text" className="grow" placeholder="Administrator" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              </label>
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">{t("setup.password")}</span></label>
              <label className="input input-bordered flex items-center gap-2">
                <Lock size={16} className="opacity-50" />
                <input type="password" className="grow" placeholder={t("setup.passwordHint")} value={password} onChange={(e) => setPassword(e.target.value)} required minLength={6} />
              </label>
            </div>
            <div className="form-control">
              <label className="label"><span className="label-text font-medium">{t("setup.confirmPassword")}</span></label>
              <label className="input input-bordered flex items-center gap-2">
                <Lock size={16} className="opacity-50" />
                <input type="password" className="grow" placeholder={t("setup.confirmPasswordHint")} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} required minLength={6} />
              </label>
            </div>
            <button type="submit" disabled={loading} className="btn btn-success w-full mt-2">
              {loading && <span className="loading loading-spinner loading-sm"></span>}
              {loading ? t("setup.submitting") : t("setup.submit")}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
