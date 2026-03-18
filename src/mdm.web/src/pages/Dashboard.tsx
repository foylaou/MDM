import { useState, useEffect } from "react";
import { useAuthStore } from "../stores/authStore";
import { useEventStore } from "../stores/eventStore";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Tablet, Wifi, WifiOff, Terminal, Radio, RefreshCw, Check, AlertCircle, Clock } from "lucide-react";
import type { Device } from "../gen/mdm/v1/device_pb";

export function Dashboard() {
  const { t } = useTranslation();
  const { clients } = useAuthStore();
  const { events } = useEventStore();
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);

  const loadDevices = async () => {
    if (!clients) return;
    setLoading(true);
    try {
      const resp = await clients.device.listDevices({ pageSize: 200 });
      setDevices(resp.devices);
    } catch (err) { console.error("Failed to load devices:", err); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadDevices(); }, [clients]);

  const enrolled = devices.filter((d) => d.enrollmentStatus === "enrolled");
  const online = enrolled.filter((d) => {
    if (!d.lastSeen) return false;
    return Date.now() - new Date(d.lastSeen.toDate()).getTime() < 24 * 60 * 60 * 1000;
  });

  const osVersions = enrolled.reduce<Record<string, number>>((acc, d) => {
    const v = d.osVersion || "Unknown";
    acc[v] = (acc[v] || 0) + 1;
    return acc;
  }, {});

  const models = enrolled.reduce<Record<string, number>>((acc, d) => {
    const m = d.model || "Unknown";
    acc[m] = (acc[m] || 0) + 1;
    return acc;
  }, {});

  const Dots = () => <span className="loading loading-dots loading-sm"></span>;

  const recentEvents = events.slice(0, 15);

  const eventStatusIcon = (status: string) => {
    switch (status?.toLowerCase()) {
      case "acknowledged": return <Check size={14} className="text-success" />;
      case "error": return <AlertCircle size={14} className="text-error" />;
      case "sent": return <Clock size={14} className="text-info" />;
      default: return <Clock size={14} className="text-base-content/30" />;
    }
  };

  return (
    <div className="space-y-6">
      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="stat bg-base-100 rounded-box shadow">
          <div className="stat-figure text-primary"><Tablet size={28} /></div>
          <div className="stat-title">{t("dashboard.totalDevices")}</div>
          <div className="stat-value text-primary">{loading ? <Dots /> : devices.length}</div>
          <div className="stat-desc">{t("dashboard.totalDesc")}</div>
        </div>
        <div className="stat bg-base-100 rounded-box shadow">
          <div className="stat-figure text-success"><Wifi size={28} /></div>
          <div className="stat-title">{t("dashboard.enrolled")}</div>
          <div className="stat-value text-success">{loading ? <Dots /> : enrolled.length}</div>
          <div className="stat-desc">{t("dashboard.enrolledDesc")}</div>
        </div>
        <div className="stat bg-base-100 rounded-box shadow">
          <div className="stat-figure text-info"><Radio size={28} /></div>
          <div className="stat-title">{t("dashboard.online")}</div>
          <div className="stat-value text-info">{loading ? <Dots /> : online.length}</div>
          <div className="stat-desc">{t("dashboard.onlineDesc")}</div>
        </div>
        <div className="stat bg-base-100 rounded-box shadow">
          <div className="stat-figure text-warning"><WifiOff size={28} /></div>
          <div className="stat-title">{t("dashboard.offline")}</div>
          <div className="stat-value text-warning">{loading ? <Dots /> : enrolled.length - online.length}</div>
          <div className="stat-desc">{t("dashboard.offlineDesc")}</div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* OS + Model distribution */}
        <div className="lg:col-span-2 grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="card bg-base-100 shadow">
            <div className="card-body">
              <h2 className="card-title text-base">{t("dashboard.osDistribution")}</h2>
              {loading ? <div className="flex justify-center py-8"><span className="loading loading-spinner loading-md"></span></div> : (
                <div className="space-y-3 mt-2">
                  {Object.entries(osVersions).sort((a, b) => b[1] - a[1]).map(([version, count]) => (
                    <div key={version}>
                      <div className="flex justify-between text-sm mb-1">
                        <span className="font-medium">iPadOS {version}</span>
                        <span className="text-base-content/60">{count} ({Math.round((count / enrolled.length) * 100)}%)</span>
                      </div>
                      <progress className="progress progress-primary w-full" value={count} max={enrolled.length}></progress>
                    </div>
                  ))}
                  {Object.keys(osVersions).length === 0 && <div className="text-center py-4 text-base-content/50">{t("common.noData")}</div>}
                </div>
              )}
            </div>
          </div>
          <div className="card bg-base-100 shadow">
            <div className="card-body">
              <h2 className="card-title text-base">{t("dashboard.modelDistribution")}</h2>
              {loading ? <div className="flex justify-center py-8"><span className="loading loading-spinner loading-md"></span></div> : (
                <div className="space-y-3 mt-2">
                  {Object.entries(models).sort((a, b) => b[1] - a[1]).map(([model, count]) => (
                    <div key={model}>
                      <div className="flex justify-between text-sm mb-1">
                        <span className="font-medium">{model}</span>
                        <span className="text-base-content/60">{count} ({Math.round((count / enrolled.length) * 100)}%)</span>
                      </div>
                      <progress className="progress progress-secondary w-full" value={count} max={enrolled.length}></progress>
                    </div>
                  ))}
                  {Object.keys(models).length === 0 && <div className="text-center py-4 text-base-content/50">{t("common.noData")}</div>}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Event timeline */}
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <div className="flex items-center justify-between">
              <h2 className="card-title text-base">{t("events.title")}</h2>
              <Link to="/events" className="btn btn-ghost btn-xs">{t("common.filter")} &rarr;</Link>
            </div>
            {recentEvents.length === 0 ? (
              <div className="text-center py-8 text-base-content/50 text-sm">{t("events.noEvents")}</div>
            ) : (
              <ul className="timeline timeline-vertical timeline-compact mt-2">
                {recentEvents.map((evt, i) => (
                  <li key={`${evt.id}-${i}`}>
                    {i > 0 && <hr />}
                    <div className="timeline-start text-xs opacity-50 w-16">
                      {evt.timestamp ? new Date(evt.timestamp.toDate()).toLocaleTimeString() : ""}
                    </div>
                    <div className="timeline-middle">
                      {eventStatusIcon(evt.status)}
                    </div>
                    <div className="timeline-end timeline-box py-1 px-2 text-xs">
                      <span className="font-medium">{evt.eventType}</span>
                      <span className="opacity-50 ml-1">{evt.udid?.slice(0, 8)}...</span>
                    </div>
                    {i < recentEvents.length - 1 && <hr />}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>

      {/* Quick actions */}
      <div className="card bg-base-100 shadow">
        <div className="card-body">
          <h2 className="card-title text-base">{t("dashboard.quickActions")}</h2>
          <div className="flex flex-wrap gap-3 mt-2">
            <Link to="/devices" className="btn btn-outline btn-sm gap-2"><Tablet size={16} />{t("dashboard.viewDevices")}</Link>
            <Link to="/commands" className="btn btn-outline btn-sm gap-2"><Terminal size={16} />{t("dashboard.sendCommand")}</Link>
            <Link to="/events" className="btn btn-outline btn-sm gap-2"><Radio size={16} />{t("dashboard.monitorEvents")}</Link>
            <button onClick={loadDevices} className="btn btn-outline btn-sm gap-2"><RefreshCw size={16} />{t("common.refresh")}</button>
          </div>
        </div>
      </div>
    </div>
  );
}
