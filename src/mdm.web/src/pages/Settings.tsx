import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Mail, Inbox, Save, Send, PlugZap } from "lucide-react";
import apiClient from "../lib/apiClient";
import { useDialog } from "../components/DialogProvider";

interface MailSettings {
  smtp_enabled: boolean;
  smtp_host: string;
  smtp_port: string;
  smtp_username: string;
  smtp_password: string;
  smtp_from: string;
  smtp_from_name: string;
  smtp_tls: boolean;

  incoming_enabled: boolean;
  incoming_protocol: "imap" | "pop3";
  incoming_host: string;
  incoming_port: string;
  incoming_username: string;
  incoming_password: string;
  incoming_tls: boolean;
  incoming_mailbox: string;

  has_smtp_password?: boolean;
  has_incoming_password?: boolean;
  updated_at?: string;
}

const PASSWORD_PLACEHOLDER = "********";

const EMPTY: MailSettings = {
  smtp_enabled: false,
  smtp_host: "",
  smtp_port: "587",
  smtp_username: "",
  smtp_password: "",
  smtp_from: "",
  smtp_from_name: "",
  smtp_tls: true,
  incoming_enabled: false,
  incoming_protocol: "imap",
  incoming_host: "",
  incoming_port: "993",
  incoming_username: "",
  incoming_password: "",
  incoming_tls: true,
  incoming_mailbox: "INBOX",
};

export function Settings() {
  const { t } = useTranslation();
  const dialog = useDialog();

  const [form, setForm] = useState<MailSettings>(EMPTY);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testingSmtp, setTestingSmtp] = useState(false);
  const [testingIncoming, setTestingIncoming] = useState(false);
  const [testTo, setTestTo] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      const { data } = await apiClient.get<MailSettings>("/api/settings/mail");
      setForm({
        ...data,
        smtp_password: data.has_smtp_password ? PASSWORD_PLACEHOLDER : "",
        incoming_password: data.has_incoming_password ? PASSWORD_PLACEHOLDER : "",
      });
    } catch (err: any) {
      await dialog.error(t("settings.loadFailed") + ": " + (err?.response?.data?.error || err?.message || ""));
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  const update = <K extends keyof MailSettings>(key: K, value: MailSettings[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  const save = async () => {
    setSaving(true);
    try {
      await apiClient.put("/api/settings/mail", form);
      await dialog.alert(t("settings.saveOk"));
      load();
    } catch (err: any) {
      await dialog.error(t("settings.saveFailed") + ": " + (err?.response?.data?.error || err?.message || ""));
    } finally {
      setSaving(false);
    }
  };

  const testSmtp = async () => {
    if (!testTo) { await dialog.error(t("settings.testToRequired")); return; }
    setTestingSmtp(true);
    try {
      await apiClient.post("/api/settings/mail/test-smtp", { to: testTo });
      await dialog.alert(t("settings.testSmtpOk", { to: testTo }));
    } catch (err: any) {
      await dialog.error(t("settings.testSmtpFailed") + ": " + (err?.response?.data?.error || err?.message || ""));
    } finally {
      setTestingSmtp(false);
    }
  };

  const testIncoming = async () => {
    setTestingIncoming(true);
    try {
      const { data } = await apiClient.post("/api/settings/mail/test-incoming", {});
      const greet = data?.greeting ? "\n" + data.greeting.trim() : "";
      await dialog.alert(t("settings.testIncomingOk", { address: data?.address }) + greet);
    } catch (err: any) {
      await dialog.error(t("settings.testIncomingFailed") + ": " + (err?.response?.data?.error || err?.message || ""));
    } finally {
      setTestingIncoming(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <span className="loading loading-spinner loading-lg text-primary"></span>
      </div>
    );
  }

  return (
    <div className="space-y-4 max-w-4xl">
      <div>
        <h1 className="text-2xl font-bold">{t("nav.settings")}</h1>
        <p className="text-sm text-base-content/60">{t("settings.subtitle")}</p>
      </div>

      {/* Outgoing (SMTP) */}
      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <div className="flex items-center gap-2">
            <Mail size={18} className="text-primary" />
            <h2 className="card-title text-lg">{t("settings.outgoing")}</h2>
            <label className="ml-auto label cursor-pointer gap-2">
              <span className="label-text">{t("settings.enabled")}</span>
              <input
                type="checkbox"
                className="toggle toggle-primary"
                checked={form.smtp_enabled}
                onChange={(e) => update("smtp_enabled", e.target.checked)}
              />
            </label>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <label className="form-control">
              <span className="label label-text">{t("settings.host")}</span>
              <input className="input input-bordered input-sm" value={form.smtp_host}
                onChange={(e) => update("smtp_host", e.target.value)} placeholder="smtp.gmail.com" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.port")}</span>
              <input className="input input-bordered input-sm" value={form.smtp_port}
                onChange={(e) => update("smtp_port", e.target.value)} placeholder="587" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.username")}</span>
              <input className="input input-bordered input-sm" value={form.smtp_username}
                onChange={(e) => update("smtp_username", e.target.value)} autoComplete="off" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.password")}</span>
              <input className="input input-bordered input-sm" type="password" value={form.smtp_password}
                onChange={(e) => update("smtp_password", e.target.value)} autoComplete="new-password"
                placeholder={form.has_smtp_password ? t("settings.passwordSet") : ""} />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.from")}</span>
              <input className="input input-bordered input-sm" value={form.smtp_from}
                onChange={(e) => update("smtp_from", e.target.value)} placeholder="noreply@example.com" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.fromName")}</span>
              <input className="input input-bordered input-sm" value={form.smtp_from_name}
                onChange={(e) => update("smtp_from_name", e.target.value)} placeholder="MDM 管理平台" />
            </label>
            <label className="label cursor-pointer justify-start gap-2 md:col-span-2">
              <input type="checkbox" className="checkbox checkbox-sm"
                checked={form.smtp_tls}
                onChange={(e) => update("smtp_tls", e.target.checked)} />
              <span className="label-text">{t("settings.useTls")}</span>
            </label>
          </div>

          <div className="divider my-2"></div>
          <div className="flex flex-wrap items-end gap-2">
            <label className="form-control flex-1 min-w-48">
              <span className="label label-text">{t("settings.testTo")}</span>
              <input className="input input-bordered input-sm" type="email" value={testTo}
                onChange={(e) => setTestTo(e.target.value)} />
            </label>
            <button onClick={testSmtp} disabled={testingSmtp || !form.smtp_enabled}
              className="btn btn-outline btn-sm gap-1">
              {testingSmtp
                ? <span className="loading loading-spinner loading-xs"></span>
                : <Send size={14} />}
              {t("settings.testSmtp")}
            </button>
          </div>
        </div>
      </div>

      {/* Incoming (IMAP/POP3) */}
      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <div className="flex items-center gap-2">
            <Inbox size={18} className="text-primary" />
            <h2 className="card-title text-lg">{t("settings.incoming")}</h2>
            <label className="ml-auto label cursor-pointer gap-2">
              <span className="label-text">{t("settings.enabled")}</span>
              <input
                type="checkbox"
                className="toggle toggle-primary"
                checked={form.incoming_enabled}
                onChange={(e) => update("incoming_enabled", e.target.checked)}
              />
            </label>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <label className="form-control">
              <span className="label label-text">{t("settings.protocol")}</span>
              <select className="select select-bordered select-sm"
                value={form.incoming_protocol}
                onChange={(e) => update("incoming_protocol", e.target.value as "imap" | "pop3")}>
                <option value="imap">IMAP</option>
                <option value="pop3">POP3</option>
              </select>
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.mailbox")}</span>
              <input className="input input-bordered input-sm" value={form.incoming_mailbox}
                onChange={(e) => update("incoming_mailbox", e.target.value)} placeholder="INBOX" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.host")}</span>
              <input className="input input-bordered input-sm" value={form.incoming_host}
                onChange={(e) => update("incoming_host", e.target.value)} placeholder="imap.gmail.com" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.port")}</span>
              <input className="input input-bordered input-sm" value={form.incoming_port}
                onChange={(e) => update("incoming_port", e.target.value)} placeholder="993" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.username")}</span>
              <input className="input input-bordered input-sm" value={form.incoming_username}
                onChange={(e) => update("incoming_username", e.target.value)} autoComplete="off" />
            </label>
            <label className="form-control">
              <span className="label label-text">{t("settings.password")}</span>
              <input className="input input-bordered input-sm" type="password" value={form.incoming_password}
                onChange={(e) => update("incoming_password", e.target.value)} autoComplete="new-password"
                placeholder={form.has_incoming_password ? t("settings.passwordSet") : ""} />
            </label>
            <label className="label cursor-pointer justify-start gap-2 md:col-span-2">
              <input type="checkbox" className="checkbox checkbox-sm"
                checked={form.incoming_tls}
                onChange={(e) => update("incoming_tls", e.target.checked)} />
              <span className="label-text">{t("settings.useTls")}</span>
            </label>
          </div>

          <div className="divider my-2"></div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-base-content/60 flex-1">{t("settings.testIncomingHint")}</span>
            <button onClick={testIncoming} disabled={testingIncoming || !form.incoming_enabled}
              className="btn btn-outline btn-sm gap-1">
              {testingIncoming
                ? <span className="loading loading-spinner loading-xs"></span>
                : <PlugZap size={14} />}
              {t("settings.testIncoming")}
            </button>
          </div>
        </div>
      </div>

      <div className="flex justify-end gap-2">
        {form.updated_at && (
          <span className="text-xs text-base-content/50 self-center">
            {t("settings.updatedAt", { at: new Date(form.updated_at).toLocaleString() })}
          </span>
        )}
        <button onClick={save} disabled={saving} className="btn btn-primary btn-sm gap-1">
          {saving
            ? <span className="loading loading-spinner loading-xs"></span>
            : <Save size={14} />}
          {t("common.save")}
        </button>
      </div>
    </div>
  );
}
