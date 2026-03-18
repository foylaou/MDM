import { useEffect, useRef } from "react";
import { useAuthStore } from "../stores/authStore";
import { useEventStore } from "../stores/eventStore";

/**
 * Hook that manages the gRPC event stream.
 * Should be mounted once at the layout level.
 */
export function useEventStream() {
  const clients = useAuthStore((s) => s.clients);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const streaming = useEventStore((s) => s.streaming);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    console.log("[stream] check:", { hasClients: !!clients, streaming, isAuthenticated });
    if (!clients || !streaming || !isAuthenticated) {
      abortRef.current?.abort();
      return;
    }
    console.log("[stream] connecting...");

    const controller = new AbortController();
    abortRef.current = controller;

    (async () => {
      try {
        const stream = clients.event.streamEvents(
          { filterUdid: "" },
          { signal: controller.signal }
        );
        for await (const event of stream) {
          // Use getState() to avoid stale closure
          useEventStore.getState().processEvent(event);
        }
      } catch (err) {
        const msg = (err as Error).message || "";
        if ((err as Error).name === "AbortError" || msg.includes("aborted") || msg.includes("canceled")) return;
        console.error("gRPC stream error:", err);
        // Auto-retry after 5s
        setTimeout(() => {
          const s = useEventStore.getState();
          if (s.streaming) {
            s.setStreaming(false);
            setTimeout(() => useEventStore.getState().setStreaming(true), 200);
          }
        }, 5000);
      }
    })();

    return () => controller.abort();
  }, [clients, streaming, isAuthenticated]); // removed processEvent from deps
}
