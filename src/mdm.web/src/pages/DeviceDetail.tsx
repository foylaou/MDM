import { useState, useEffect, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useAuthStore } from "../stores/authStore";
import { useEventStore } from "../stores/eventStore";
import { useTranslation } from "react-i18next";
import { ResponseViewer } from "../components/ResponseViewer";
import {
  ArrowLeft, RefreshCw, Smartphone, Lock, RotateCcw, Power,
  MapPin, Volume2, Bell, Info, AppWindow, FileText, Shield, Award,
  Package, MapPinOff, Battery, Wifi, HardDrive, KeyRound, Trash2,
  Download, Building2, PackageMinus, Upload, ClipboardList,
} from "lucide-react";
import { ProfilePicker } from "../components/ProfilePicker";
import { AssetForm } from "../components/AssetForm";

interface DeviceData {
  udid: string;
  serial_number: string;
  device_name: string;
  model: string;
  os_version: string;
  last_seen: string;
  enrollment_status: string;
  is_supervised: boolean;
  is_lost_mode: boolean;
  battery_level: number;
  details: Record<string, unknown>;
}

// --- Tab definitions ---
interface TabDef {
  key: string;
  label: string;
  icon: React.ReactNode;
  method: string;        // gRPC command method to sync
  detailsKey: string;    // key in device.details for cached base64 plist
  updatedKey: string;    // key in device.details for last updated time
}

const tabs: TabDef[] = [
  { key: "info",     label: "概覽",     icon: <Info size={16} />,     method: "getDeviceInfo",         detailsKey: "device_info",         updatedKey: "device_info" },
  { key: "apps",     label: "已裝 App", icon: <AppWindow size={16} />, method: "getInstalledApps",      detailsKey: "installed_apps_raw",  updatedKey: "installed_apps_updated" },
  { key: "profiles", label: "描述檔",   icon: <FileText size={16} />,  method: "getProfileList",        detailsKey: "profiles_raw",        updatedKey: "profiles_updated" },
  { key: "security", label: "安全性",   icon: <Shield size={16} />,    method: "getSecurityInfo",       detailsKey: "security_raw",        updatedKey: "security_updated" },
  { key: "certs",    label: "憑證",     icon: <Award size={16} />,     method: "getCertificateList",    detailsKey: "certs_raw",           updatedKey: "certs_updated" },
  { key: "updates",  label: "更新",     icon: <Package size={16} />,   method: "getAvailableOSUpdates", detailsKey: "updates_raw",         updatedKey: "updates_updated" },
];

// --- Action command definitions ---
interface ActionCmd {
  label: string;
  method: string;
  icon: React.ReactNode;
  danger?: boolean;
  requiresLostMode?: boolean;
  roles?: string[]; // allowed roles, empty = all
}

const actionCommands: ActionCmd[] = [
  { label: "推播",         method: "sendPush",           icon: <Bell size={14} /> },
  { label: "鎖定裝置",     method: "lockDevice",         icon: <Lock size={14} /> },
  { label: "重新啟動",     method: "restartDevice",      icon: <RotateCcw size={14} /> },
  { label: "關機",         method: "shutdownDevice",     icon: <Power size={14} /> },
  { label: "清除密碼",     method: "clearPasscode",      icon: <KeyRound size={14} /> },
  { label: "啟用遺失模式", method: "enableLostMode",     icon: <MapPin size={14} /> },
  { label: "關閉遺失模式", method: "disableLostMode",    icon: <MapPinOff size={14} /> },
  { label: "定位",         method: "getDeviceLocation",  icon: <MapPin size={14} />,   requiresLostMode: true },
  { label: "播放聲音",     method: "playLostModeSound",  icon: <Volume2 size={14} />,  requiresLostMode: true },
  { label: "安裝 App",     method: "installApp",         icon: <Download size={14} />, roles: ["admin", "operator"] },
  { label: "安裝企業 App", method: "installEnterpriseApp", icon: <Building2 size={14} />, roles: ["admin", "operator"] },
  { label: "移除 App",     method: "removeApp",          icon: <PackageMinus size={14} />, roles: ["admin", "operator"] },
  { label: "清除裝置",     method: "eraseDevice",        icon: <Trash2 size={14} />,   danger: true, roles: ["admin"] },
];

export function DeviceDetail() {
  const { t } = useTranslation();
  const { udid } = useParams<{ udid: string }>();
  const { clients, user: currentUser } = useAuthStore();
  const { trackCommand, events } = useEventStore();
  const userRole = currentUser?.role || "viewer";
  const [device, setDevice] = useState<DeviceData | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState("info");
  const [syncing, setSyncing] = useState<string | null>(null);
  const [executing, setExecuting] = useState<string | null>(null);
  const [actionResult, setActionResult] = useState<string | null>(null);
  const [showInstallProfile, setShowInstallProfile] = useState(false);
  const [selectedProfileId, setSelectedProfileId] = useState("");
  const [selectedProfilePayload, setSelectedProfilePayload] = useState("");

  const baseUrl = import.meta.env.DEV ? "" : window.location.origin;

  const loadDevice = useCallback(async () => {
    if (!udid) return;
    setLoading(true);
    try {
      const resp = await fetch(`${baseUrl}/api/devices/${udid}`, {
        credentials: "include",
      });
      if (resp.ok) setDevice(await resp.json());
    } catch (err) { console.error("Load device:", err); }
    finally { setLoading(false); }
  }, [udid, baseUrl]);

  useEffect(() => { loadDevice(); }, [loadDevice]);

  // Auto-refresh from DB when new acknowledge events arrive for this device
  const deviceEventCount = events.filter((e) => e.udid === udid && e.eventType === "acknowledge").length;
  useEffect(() => {
    if (deviceEventCount > 0) {
      const timer = setTimeout(loadDevice, 1500);
      return () => clearTimeout(timer);
    }
  }, [deviceEventCount]);

  // Sync a specific tab (send query command)
  const syncTab = async (tab: TabDef) => {
    if (!clients || !udid) return;
    setSyncing(tab.key);
    try {
      // @ts-expect-error dynamic method call
      const resp = await clients.command[tab.method]({ udids: [udid] });
      trackCommand(tab.label, [udid], resp.commandUuid);
    } catch (err) {
      console.error("Sync failed:", err);
    } finally {
      setSyncing(null);
    }
  };

  // Sync ALL tabs at once
  const syncAll = async () => {
    if (!clients || !udid) return;
    setSyncing("all");
    for (const tab of tabs) {
      try {
        // @ts-expect-error dynamic method call
        const resp = await clients.command[tab.method]({ udids: [udid] });
        trackCommand(tab.label, [udid], resp.commandUuid);
      } catch (err) { console.error(`Sync ${tab.key}:`, err); }
    }
    setSyncing(null);
  };

  // Execute an action command
  const executeAction = async (cmd: ActionCmd) => {
    if (!clients || !udid) return;
    if (cmd.danger && !confirm(`確定要執行「${cmd.label}」嗎？此操作無法復原。`)) return;
    setExecuting(cmd.method);
    setActionResult(null);
    try {
      // @ts-expect-error dynamic method call
      const resp = await clients.command[cmd.method]({ udids: [udid] });
      trackCommand(cmd.label, [udid], resp.commandUuid);
      if (resp.rawResponse) setActionResult(resp.rawResponse);
    } catch (err) {
      setActionResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setExecuting(null); }
  };

  // Remove a profile by identifier
  const removeProfile = async (identifier: string) => {
    if (!clients || !udid) return;
    setExecuting("removeProfile");
    try {
      const resp = await clients.command.removeProfile({ udids: [udid], identifier });
      trackCommand("移除描述檔", [udid], resp.commandUuid);
    } catch (err) {
      setActionResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setExecuting(null); }
  };

  // Install a profile
  const installProfile = async () => {
    if (!clients || !udid || !selectedProfilePayload) return;
    setShowInstallProfile(false);
    setExecuting("installProfile");
    try {
      const resp = await clients.command.installProfile({ udids: [udid], payload: new TextEncoder().encode(atob(selectedProfilePayload)) });
      trackCommand("安裝描述檔", [udid], resp.commandUuid);
      setSelectedProfileId("");
      setSelectedProfilePayload("");
    } catch (err) {
      setActionResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setExecuting(null); }
  };

  // Get cached payload for the active tab
  const currentTab = tabs.find((t) => t.key === activeTab);
  const getCachedPayload = (): string | null => {
    if (!currentTab || !device?.details) return null;
    const raw = device.details[currentTab.detailsKey];
    if (typeof raw === "string") return raw;
    if (typeof raw === "object" && raw !== null) return JSON.stringify(raw, null, 2);
    return null;
  };
  const getUpdatedAt = (): string | null => {
    if (!currentTab || !device?.details) return null;
    const t = device.details[currentTab.updatedKey];
    if (typeof t === "string" && t.includes("T")) return new Date(t).toLocaleString();
    if (typeof t === "object" && t !== null) {
      // device_info is an object, check updated_at inside
      const obj = t as Record<string, unknown>;
      if (typeof obj.updated_at === "string") return new Date(obj.updated_at).toLocaleString();
    }
    return null;
  };

  if (loading) {
    return <div className="flex justify-center py-12"><span className="loading loading-spinner loading-lg"></span></div>;
  }
  if (!device) {
    return (
      <div className="text-center py-12">
        <p className="text-base-content/50">Device not found</p>
        <Link to="/devices" className="btn btn-ghost btn-sm mt-4"><ArrowLeft size={14} /> {t("devices.title")}</Link>
      </div>
    );
  }

  const batteryPercent = device.battery_level >= 0 ? Math.round(device.battery_level * 100) : null;
  const cachedPayload = getCachedPayload();
  const updatedAt = getUpdatedAt();

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link to="/devices" className="btn btn-ghost btn-sm btn-circle"><ArrowLeft size={18} /></Link>
        <div className="flex-1">
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Smartphone size={24} />
            {device.device_name || device.serial_number || udid}
          </h1>
          <p className="text-sm text-base-content/60 font-mono">{udid}</p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <span className={`badge ${device.enrollment_status === "enrolled" ? "badge-success" : "badge-ghost"}`}>{device.enrollment_status}</span>
          {device.is_lost_mode && <span className="badge badge-error gap-1"><MapPin size={12} /> 遺失模式</span>}
          {device.is_supervised && <span className="badge badge-info gap-1"><Shield size={12} /> 受監管</span>}
        </div>
        <button onClick={syncAll} disabled={syncing !== null} className="btn btn-primary btn-sm gap-1">
          {syncing === "all" ? <span className="loading loading-spinner loading-xs"></span> : <RefreshCw size={14} />}
          全部同步
        </button>
      </div>

      {/* Stats row */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        {[
          { icon: <Smartphone size={14} />, label: t("devices.serial"), value: device.serial_number },
          { icon: <HardDrive size={14} />, label: t("devices.model"), value: device.model },
          { icon: <Package size={14} />, label: t("devices.os"), value: device.os_version },
          { icon: <Wifi size={14} />, label: t("devices.lastSeen"), value: device.last_seen ? new Date(device.last_seen).toLocaleString() : "-" },
          ...(batteryPercent !== null ? [{ icon: <Battery size={14} />, label: "電量", value: `${batteryPercent}%` }] : []),
        ].map((item) => (
          <div key={item.label} className="stat bg-base-100 rounded-box shadow p-3">
            <div className="stat-title text-xs flex items-center gap-1">{item.icon}{item.label}</div>
            <div className="stat-value text-sm font-medium truncate">{item.value || "-"}</div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <div className="card bg-base-100 shadow">
        <div className="border-b border-base-300">
          <div role="tablist" className="tabs tabs-bordered px-4">
            {tabs.map((tab) => {
              const hasData = device.details?.[tab.detailsKey] !== undefined;
              return (
                <button
                  key={tab.key}
                  role="tab"
                  className={`tab gap-1.5 ${activeTab === tab.key ? "tab-active" : ""}`}
                  onClick={() => setActiveTab(tab.key)}
                >
                  {tab.icon}
                  {tab.label}
                  {hasData && <span className="w-1.5 h-1.5 rounded-full bg-success"></span>}
                </button>
              );
            })}
            <button
              role="tab"
              className={`tab gap-1.5 ${activeTab === "asset" ? "tab-active" : ""}`}
              onClick={() => setActiveTab("asset")}
            >
              <ClipboardList size={16} />
              {t("assets.title")}
            </button>
          </div>
        </div>

        <div className="card-body p-4">
          {activeTab === "asset" ? (
            /* Asset tab — special handling */
            <AssetForm deviceUdid={udid!} />
          ) : (
            <>
              {/* Tab header with sync + action buttons */}
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h3 className="font-semibold flex items-center gap-2">
                    {currentTab?.icon} {currentTab?.label}
                  </h3>
                  {updatedAt && (
                    <p className="text-xs text-base-content/50 mt-0.5">上次同步：{updatedAt}</p>
                  )}
                </div>
                <div className="flex gap-2">
                  {activeTab === "profiles" && (
                    <button onClick={() => setShowInstallProfile(true)} className="btn btn-primary btn-sm gap-1">
                      <Upload size={14} /> 安裝描述檔
                    </button>
                  )}
                  <button
                    onClick={() => currentTab && syncTab(currentTab)}
                    disabled={syncing !== null}
                    className="btn btn-outline btn-sm gap-1"
                  >
                    {syncing === currentTab?.key ? <span className="loading loading-spinner loading-xs"></span> : <RefreshCw size={14} />}
                    同步
                  </button>
                </div>
              </div>

              {/* Tab content */}
              {syncing === currentTab?.key ? (
                <div className="flex items-center justify-center py-8 gap-2 text-base-content/50">
                  <span className="loading loading-spinner loading-md"></span>
                  正在查詢，等待裝置回應...
                </div>
              ) : cachedPayload ? (
                <ResponseViewer
                  rawPayload={cachedPayload}
                  onRemoveProfile={activeTab === "profiles" ? removeProfile : undefined}
                />
              ) : (
                <div className="text-center py-8 text-base-content/50">
                  尚無資料，點擊「同步」查詢
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {/* Action commands */}
      <div className="card bg-base-100 shadow">
        <div className="card-body p-4">
          <h2 className="card-title text-base mb-2">裝置操作</h2>
          <div className="flex flex-wrap gap-2">
            {actionCommands.filter((cmd) => !cmd.roles || cmd.roles.includes(userRole)).map((cmd) => {
              const disabled = executing !== null || (cmd.requiresLostMode && !device.is_lost_mode);
              return (
                <div key={cmd.method} className={cmd.requiresLostMode && !device.is_lost_mode ? "tooltip" : ""} data-tip={cmd.requiresLostMode ? "需先啟用遺失模式" : ""}>
                  <button
                    onClick={() => executeAction(cmd)}
                    disabled={disabled}
                    className={`btn btn-sm gap-1 ${cmd.danger ? "btn-error" : "btn-outline"}`}
                  >
                    {executing === cmd.method ? <span className="loading loading-spinner loading-xs"></span> : cmd.icon}
                    {cmd.label}
                  </button>
                </div>
              );
            })}
          </div>

          {actionResult && (
            <div className="mt-4">
              <h3 className="text-sm font-medium mb-2">{t("commands.result")}</h3>
              <ResponseViewer rawPayload={actionResult} />
            </div>
          )}
        </div>
      </div>
      {/* Install Profile Modal */}
      <dialog className={`modal ${showInstallProfile ? "modal-open" : ""}`}>
        <div className="modal-box">
          <h3 className="font-bold text-lg flex items-center gap-2"><Upload size={18} /> 安裝描述檔</h3>
          <div className="py-4">
            <ProfilePicker
              selectedId={selectedProfileId}
              onSelect={(id, base64) => {
                setSelectedProfileId(id);
                setSelectedProfilePayload(base64);
              }}
            />
          </div>
          <div className="modal-action">
            <button className="btn" onClick={() => setShowInstallProfile(false)}>取消</button>
            <button
              className="btn btn-primary"
              disabled={!selectedProfilePayload}
              onClick={installProfile}
            >
              安裝
            </button>
          </div>
        </div>
        <form method="dialog" className="modal-backdrop">
          <button onClick={() => setShowInstallProfile(false)}>close</button>
        </form>
      </dialog>
    </div>
  );
}
