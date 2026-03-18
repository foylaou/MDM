import { useState } from "react";
import { useAuthStore } from "../stores/authStore";
import { useEventStore } from "../stores/eventStore";
import { useTranslation } from "react-i18next";
import { DevicePicker } from "../components/DevicePicker";
import {
  Download, Package, Trash2, Send, Building2, Link as LinkIcon,
} from "lucide-react";

type Mode = "vpp" | "enterprise";

export function Apps() {
  const { t } = useTranslation();
  const { clients } = useAuthStore();
  const { trackCommand } = useEventStore();

  const [mode, setMode] = useState<Mode>("vpp");

  // VPP state
  const [appInput, setAppInput] = useState("");
  const [selectedDevices, setSelectedDevices] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  // Enterprise state
  const [manifestUrl, setManifestUrl] = useState("");

  // Remove app state
  const [removeId, setRemoveId] = useState("");
  const [showRemove, setShowRemove] = useState(false);

  // Parse App Store URL or ID → iTunes Store ID
  const parseAppId = (input: string): string => {
    // Handle URL: https://apps.apple.com/app/xxx/id123456789
    const match = input.match(/id(\d+)/);
    if (match) return match[1];
    // Handle plain number
    if (/^\d+$/.test(input.trim())) return input.trim();
    return input.trim();
  };

  const handleInstallVPP = async () => {
    if (!clients || selectedDevices.length === 0 || !appInput.trim()) return;
    const adamId = parseAppId(appInput);
    setLoading(true);
    setResult(null);
    try {
      // Step 1: Assign VPP license (need serial numbers from device list)
      // For now we pass UDIDs — the command service handles the mapping
      const assignResp = await clients.vpp.assignLicense({
        adamId,
        serialNumbers: selectedDevices, // in practice these should be serial numbers
      });

      // Step 2: Install app via MDM command
      const installResp = await clients.command.installApp({
        udids: selectedDevices,
        itunesStoreId: adamId,
        assignVppLicense: true,
      });
      trackCommand(`安裝 App (${adamId})`, selectedDevices, installResp.commandUuid);

      setResult(JSON.stringify({ vpp: assignResp.status, install: installResp.rawResponse }, null, 2));
    } catch (err) {
      setResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setLoading(false); }
  };

  const handleInstallEnterprise = async () => {
    if (!clients || selectedDevices.length === 0 || !manifestUrl.trim()) return;
    setLoading(true);
    setResult(null);
    try {
      const resp = await clients.command.installEnterpriseApp({
        udids: selectedDevices,
        manifestUrl: manifestUrl.trim(),
      });
      trackCommand("安裝企業 App", selectedDevices, resp.commandUuid);
      setResult(resp.rawResponse || "OK");
    } catch (err) {
      setResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setLoading(false); }
  };

  const handleRemoveApp = async () => {
    if (!clients || selectedDevices.length === 0 || !removeId.trim()) return;
    setLoading(true);
    setResult(null);
    try {
      const resp = await clients.command.removeApp({
        udids: selectedDevices,
        identifier: removeId.trim(),
      });
      trackCommand(`移除 App (${removeId})`, selectedDevices, resp.commandUuid);
      setResult(resp.rawResponse || "OK");
      setShowRemove(false);
    } catch (err) {
      setResult(`Error: ${err instanceof Error ? err.message : "Unknown"}`);
    } finally { setLoading(false); }
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold">{t("nav.apps") || "App 管理"}</h1>
        <p className="text-sm text-base-content/60">安裝、移除 App 到裝置</p>
      </div>

      {/* Mode tabs */}
      <div role="tablist" className="tabs tabs-boxed w-fit">
        <button role="tab" className={`tab gap-1.5 ${mode === "vpp" ? "tab-active" : ""}`} onClick={() => setMode("vpp")}>
          <Download size={14} /> VPP App
        </button>
        <button role="tab" className={`tab gap-1.5 ${mode === "enterprise" ? "tab-active" : ""}`} onClick={() => setMode("enterprise")}>
          <Building2 size={14} /> 企業 App
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Left: App form */}
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            {mode === "vpp" ? (
              <>
                <h2 className="card-title text-base gap-2"><Package size={18} /> 安裝 VPP App</h2>
                <div role="alert" className="alert alert-info mt-2 text-sm">
                  <span>
                    請先至{" "}
                    <a href="https://business.apple.com/" target="_blank" rel="noopener noreferrer" className="link link-primary font-medium">
                      Apple Business Manager
                    </a>
                    {" "}採購對應數量的 App 授權，再複製 App Store URL 或 ID 進行安裝。
                  </span>
                </div>
                <div className="form-control mt-2">
                  <label className="label"><span className="label-text font-medium">App Store URL 或 ID</span></label>
                  <label className="input input-bordered flex items-center gap-2">
                    <LinkIcon size={14} className="opacity-50" />
                    <input
                      type="text"
                      placeholder="https://apps.apple.com/app/xxx/id123456 或 123456"
                      value={appInput}
                      onChange={(e) => setAppInput(e.target.value)}
                      className="grow"
                    />
                  </label>
                  {appInput && (
                    <label className="label">
                      <span className="label-text-alt">iTunes Store ID: {parseAppId(appInput)}</span>
                    </label>
                  )}
                </div>
              </>
            ) : (
              <>
                <h2 className="card-title text-base gap-2"><Building2 size={18} /> 安裝企業 App</h2>
                <div className="form-control mt-2">
                  <label className="label"><span className="label-text font-medium">Manifest URL</span></label>
                  <label className="input input-bordered flex items-center gap-2">
                    <LinkIcon size={14} className="opacity-50" />
                    <input
                      type="text"
                      placeholder="https://example.com/app/manifest.plist"
                      value={manifestUrl}
                      onChange={(e) => setManifestUrl(e.target.value)}
                      className="grow"
                    />
                  </label>
                </div>
              </>
            )}

            {/* Target devices */}
            <div className="form-control mt-3">
              <label className="label"><span className="label-text font-medium">目標裝置</span></label>
              <DevicePicker selected={selectedDevices} onChange={setSelectedDevices} />
            </div>

            {/* Actions */}
            <div className="flex gap-2 mt-4">
              <button
                onClick={mode === "vpp" ? handleInstallVPP : handleInstallEnterprise}
                disabled={loading || selectedDevices.length === 0}
                className="btn btn-primary gap-1"
              >
                {loading ? <span className="loading loading-spinner loading-sm"></span> : <Send size={14} />}
                安裝到 {selectedDevices.length} 台裝置
              </button>
              <button
                onClick={() => setShowRemove(!showRemove)}
                className="btn btn-outline btn-error gap-1"
              >
                <Trash2 size={14} /> 移除 App
              </button>
            </div>

            {/* Remove app form */}
            {showRemove && (
              <div className="mt-3 p-3 border border-error/20 rounded-lg bg-error/5">
                <div className="form-control">
                  <label className="label"><span className="label-text font-medium text-error">App Bundle ID</span></label>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      placeholder="com.example.app"
                      value={removeId}
                      onChange={(e) => setRemoveId(e.target.value)}
                      className="input input-bordered input-sm flex-1"
                    />
                    <button
                      onClick={handleRemoveApp}
                      disabled={loading || !removeId.trim() || selectedDevices.length === 0}
                      className="btn btn-error btn-sm gap-1"
                    >
                      <Trash2 size={14} /> 移除
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Right: Result */}
        <div className="card bg-base-100 shadow">
          <div className="card-body">
            <h2 className="card-title text-base">{t("commands.result")}</h2>
            {result ? (
              <pre className="bg-base-200 p-4 rounded-lg overflow-auto text-sm font-mono max-h-96 whitespace-pre-wrap">
                {result}
              </pre>
            ) : (
              <div className="text-center py-12 text-base-content/50">
                <Package size={48} className="mx-auto mb-3 opacity-30" />
                <p>選擇 App 和目標裝置後點擊安裝</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
