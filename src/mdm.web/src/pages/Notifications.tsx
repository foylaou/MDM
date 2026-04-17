import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import apiClient from "../lib/apiClient";
import { Check, AlertCircle, Clock } from "lucide-react";

interface Notification {
  id: string;
  type: string;
  event: string;
  recipient: string;
  subject: string;
  status: string;
  error_message?: string;
  reference_id?: string;
  created_at: string;
  sent_at?: string;
}

const eventLabels: Record<string, string> = {
  rental_request: "租借申請",
  rental_approved: "租借核准",
  rental_rejected: "租借拒絕",
  rental_activated: "裝置借出",
  rental_overdue: "逾期催還",
  rental_returned: "裝置歸還",
};

const statusIcons: Record<string, React.ReactNode> = {
  sent: <Check size={16} className="text-success" />,
  failed: <AlertCircle size={16} className="text-error" />,
  pending: <Clock size={16} className="text-warning" />,
};

export function Notifications() {
  const { t } = useTranslation();
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [eventFilter, setEventFilter] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ limit: "100" });
      if (eventFilter) params.set("event", eventFilter);
      const { data } = await apiClient.get(`/api/notifications?${params}`);
      setNotifications(data.notifications || []);
    } catch {
      setNotifications([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, [eventFilter]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("nav.notifications")}</h1>
          <p className="text-sm text-base-content/60">Email 通知發送記錄</p>
        </div>
        <select
          className="select select-bordered select-sm"
          value={eventFilter}
          onChange={(e) => setEventFilter(e.target.value)}
        >
          <option value="">全部事件</option>
          {Object.entries(eventLabels).map(([k, v]) => (
            <option key={k} value={k}>{v}</option>
          ))}
        </select>
      </div>

      <div className="overflow-x-auto bg-base-100 rounded-lg border border-base-300">
        <table className="table table-sm">
          <thead>
            <tr>
              <th>狀態</th>
              <th>事件</th>
              <th>收件人</th>
              <th>主旨</th>
              <th>建立時間</th>
              <th>發送時間</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={6} className="text-center py-8"><span className="loading loading-spinner loading-sm"></span></td></tr>
            ) : notifications.length === 0 ? (
              <tr><td colSpan={6} className="text-center py-8 text-base-content/50">尚無通知記錄</td></tr>
            ) : notifications.map((n) => (
              <tr key={n.id}>
                <td>
                  <div className="flex items-center gap-1">
                    {statusIcons[n.status] || statusIcons.pending}
                    <span className="text-xs">{n.status}</span>
                  </div>
                </td>
                <td><span className="badge badge-sm badge-ghost">{eventLabels[n.event] || n.event}</span></td>
                <td className="text-sm">{n.recipient}</td>
                <td className="text-sm max-w-xs truncate">{n.subject}</td>
                <td className="text-xs opacity-70">{new Date(n.created_at).toLocaleString()}</td>
                <td className="text-xs opacity-70">{n.sent_at ? new Date(n.sent_at).toLocaleString() : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
