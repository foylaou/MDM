import { useState, useMemo } from "react";
import { useEventStore } from "../stores/eventStore";
import { useTranslation } from "react-i18next";
import { Search, Trash2, Wifi, WifiOff, X } from "lucide-react";
import { ResponseViewer } from "../components/ResponseViewer";
import type { MDMEvent } from "../gen/mdm/v1/event_pb";

export function Events() {
  const { t } = useTranslation();
  const { streaming, setStreaming, events, clearEvents } = useEventStore();
  const [filterUdid, setFilterUdid] = useState("");
  const [selectedEvent, setSelectedEvent] = useState<MDMEvent | null>(null);

  const filtered = useMemo(() => {
    if (!filterUdid) return events;
    const q = filterUdid.toLowerCase();
    return events.filter(
      (e) => e.udid.toLowerCase().includes(q) || e.commandUuid.toLowerCase().includes(q)
    );
  }, [events, filterUdid]);

  const getStatusBadge = (status: string) => {
    switch (status.toLowerCase()) {
      case "acknowledged": return "badge-success";
      case "error": return "badge-error";
      case "sent": return "badge-info";
      default: return "badge-ghost";
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("events.title")}</h1>
          <p className="text-sm text-base-content/60">{t("events.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="input input-bordered input-sm flex items-center gap-2">
            <Search size={14} className="opacity-50" />
            <input type="text" placeholder={t("events.filterUdid")} value={filterUdid} onChange={(e) => setFilterUdid(e.target.value)} className="grow w-36" />
          </label>
          {events.length > 0 && (
            <button onClick={clearEvents} className="btn btn-ghost btn-sm gap-1"><Trash2 size={14} /></button>
          )}
        </div>
      </div>

      {/* Stream toggle */}
      <div className={`alert ${streaming ? "alert-success" : "alert-warning"} py-3`}>
        <div className="flex items-center gap-3 flex-1">
          {streaming ? <Wifi size={18} /> : <WifiOff size={18} />}
          <span className="text-sm font-medium">{streaming ? t("events.listening") : t("stream.off")}</span>
        </div>
        <label className="swap">
          <input type="checkbox" checked={streaming} onChange={(e) => setStreaming(e.target.checked)} />
          <div className="swap-on">{t("events.stop")}</div>
          <div className="swap-off">{t("events.start")}</div>
        </label>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Event table */}
        <div className={`card bg-base-100 shadow ${selectedEvent ? "lg:col-span-2" : "lg:col-span-3"}`}>
          <div className="overflow-x-auto">
            <table className="table table-sm">
              <thead>
                <tr>
                  <th>{t("events.time")}</th>
                  <th>{t("events.type")}</th>
                  <th>UDID</th>
                  <th>{t("common.status")}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 ? (
                  <tr><td colSpan={4} className="text-center py-8 text-base-content/50">{streaming ? t("events.waiting") : t("events.noEvents")}</td></tr>
                ) : filtered.map((event, idx) => (
                  <tr
                    key={`${event.id}-${idx}`}
                    className={`hover cursor-pointer ${selectedEvent?.id === event.id ? "bg-primary/10" : ""}`}
                    onClick={() => setSelectedEvent(selectedEvent?.id === event.id ? null : event)}
                  >
                    <td className="text-sm opacity-70">
                      {event.timestamp ? new Date(event.timestamp.toDate()).toLocaleTimeString() : "-"}
                    </td>
                    <td><span className="badge badge-info badge-sm">{event.eventType}</span></td>
                    <td className="font-mono text-xs">{event.udid ? event.udid.slice(0, 16) + "..." : "-"}</td>
                    <td><span className={`badge badge-sm ${getStatusBadge(event.status)}`}>{event.status || "-"}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Detail panel */}
        {selectedEvent && (
          <div className="card bg-base-100 shadow lg:col-span-1">
            <div className="card-body p-4">
              <div className="flex items-center justify-between">
                <h3 className="card-title text-sm">Event Detail</h3>
                <button onClick={() => setSelectedEvent(null)} className="btn btn-ghost btn-xs btn-circle">
                  <X size={14} />
                </button>
              </div>
              <div className="space-y-2 mt-2 text-sm">
                <div className="flex justify-between">
                  <span className="opacity-60">Type</span>
                  <span className="badge badge-info badge-sm">{selectedEvent.eventType}</span>
                </div>
                <div className="flex justify-between">
                  <span className="opacity-60">UDID</span>
                  <span className="font-mono text-xs">{selectedEvent.udid || "-"}</span>
                </div>
                {selectedEvent.commandUuid && (
                  <div className="flex justify-between">
                    <span className="opacity-60">Command UUID</span>
                    <span className="font-mono text-xs">{selectedEvent.commandUuid}</span>
                  </div>
                )}
                <div className="flex justify-between">
                  <span className="opacity-60">Status</span>
                  <span className={`badge badge-sm ${getStatusBadge(selectedEvent.status)}`}>{selectedEvent.status}</span>
                </div>
                <div className="divider my-1"></div>
                <div className="text-xs font-medium opacity-60 mb-1">Payload</div>
                <ResponseViewer rawPayload={selectedEvent.rawPayload} />
              </div>
            </div>
          </div>
        )}
      </div>

      {filtered.length > 0 && (
        <div className="text-sm text-base-content/50">{t("common.showing", { count: filtered.length })} (max 200)</div>
      )}
    </div>
  );
}
