import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/authStore";
import { useTranslation } from "react-i18next";
import { Lock, User, UserPlus } from "lucide-react";
import apiClient from "../lib/apiClient";

type Mode = "login" | "register";

export function Login() {
  const { t } = useTranslation();
  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [loading, setLoading] = useState(false);
  const { login } = useAuthStore();
  const navigate = useNavigate();

  const reset = () => {
    setUsername(""); setPassword(""); setDisplayName(""); setConfirmPassword("");
    setError(""); setSuccess("");
  };

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await login(username, password);
      navigate("/dashboard");
    } catch (err) {
      const msg = (err as { response?: { data?: { code?: string; error?: string } } })?.response?.data;
      if (msg?.code === "inactive") {
        setError("帳號尚未啟用，請等待管理員審核");
      } else {
        setError(msg?.error || (err instanceof Error ? err.message : t("login.failed")));
      }
    } finally { setLoading(false); }
  };

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    if (password !== confirmPassword) { setError(t("setup.passwordMismatch")); return; }
    if (password.length < 6) { setError(t("setup.passwordTooShort")); return; }
    setLoading(true);
    try {
      await apiClient.post("/api/register", { username, password, display_name: displayName || username });
      setSuccess("註冊成功！請等待管理員啟用帳號後再登入。");
      reset();
      setTimeout(() => { setMode("login"); setSuccess(""); }, 3000);
    } catch (err) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data;
      setError(msg?.error || "註冊失敗");
    } finally { setLoading(false); }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-base-200 p-4">
      <div className="card bg-base-100 shadow-xl w-full max-w-md">
        <div className="card-body">
          <div className="text-center mb-4">
            <div className="bg-primary text-primary-content w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4 text-2xl font-bold">
              M
            </div>
            <h1 className="text-2xl font-bold">{t("login.title")}</h1>
            <p className="text-base-content/60 text-sm mt-1">
              {mode === "login" ? t("login.subtitle") : "建立新帳號"}
            </p>
          </div>

          {error && <div role="alert" className="alert alert-error py-2"><span>{error}</span></div>}
          {success && <div role="alert" className="alert alert-success py-2"><span>{success}</span></div>}

          {mode === "login" ? (
            <form onSubmit={handleLogin} className="space-y-4">
              <label className="input input-bordered flex items-center gap-2 w-full">
                <User size={16} className="opacity-50" />
                <input type="text" className="grow" placeholder={t("login.username")} value={username} onChange={(e) => setUsername(e.target.value)} required />
              </label>
              <label className="input input-bordered flex items-center gap-2 w-full">
                <Lock size={16} className="opacity-50" />
                <input type="password" className="grow" placeholder={t("login.password")} value={password} onChange={(e) => setPassword(e.target.value)} required />
              </label>
              <button type="submit" disabled={loading} className="btn btn-primary w-full">
                {loading && <span className="loading loading-spinner loading-sm"></span>}
                {loading ? t("login.submitting") : t("login.submit")}
              </button>
              <div className="text-center">
                <button type="button" onClick={() => { setMode("register"); reset(); }} className="btn btn-ghost btn-sm gap-1">
                  <UserPlus size={14} /> 註冊新帳號
                </button>
              </div>
            </form>
          ) : (
            <form onSubmit={handleRegister} className="space-y-4">
              <label className="input input-bordered flex items-center gap-2 w-full">
                <User size={16} className="opacity-50" />
                <input type="text" className="grow" placeholder="帳號" value={username} onChange={(e) => setUsername(e.target.value)} required />
              </label>
              <label className="input input-bordered flex items-center gap-2 w-full">
                <User size={16} className="opacity-50" />
                <input type="text" className="grow" placeholder="顯示名稱（選填）" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              </label>
              <label className="input input-bordered flex items-center gap-2 w-full">
                <Lock size={16} className="opacity-50" />
                <input type="password" className="grow" placeholder="密碼（至少 6 字元）" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={6} />
              </label>
              <label className="input input-bordered flex items-center gap-2 w-full">
                <Lock size={16} className="opacity-50" />
                <input type="password" className="grow" placeholder="確認密碼" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} required />
              </label>
              <button type="submit" disabled={loading} className="btn btn-success w-full">
                {loading && <span className="loading loading-spinner loading-sm"></span>}
                {loading ? "註冊中..." : "註冊"}
              </button>
              <div className="text-center">
                <button type="button" onClick={() => { setMode("login"); reset(); }} className="btn btn-ghost btn-sm">
                  已有帳號？登入
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
