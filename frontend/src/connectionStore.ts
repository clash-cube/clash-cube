import { useEffect } from "react";
import { create } from "zustand";
import { Proxy } from "./api";
import { useStore } from "./store";
import { ConnectionTracker, type ConnectionSnapshot } from "./connections";
import { errText } from "./components/Toast";

const tracker = new ConnectionTracker();
export const useConnectionStore = create<{
  snapshot: ConnectionSnapshot; closing: Set<string>; error: string;
}>(() => ({ snapshot: tracker.snapshot, closing: new Set(), error: "" }));

const publish = () => useConnectionStore.setState({ snapshot: tracker.snapshot, closing: new Set(tracker.closing) });
export const closeConnections = (ids: string[]) => tracker.close(ids, Proxy.CloseConnection, publish);

// One serialized feed in the main window, independent of its selected page.
// Pausing the Connections view never interrupts history collection.
export function useConnectionFeed() {
  const running = useStore((s) => s.state?.core === "running");
  const profile = useStore((s) => s.state?.profile);
  useEffect(() => {
    tracker.reset();
    publish();
  }, [profile]);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    if (!running) {
      tracker.stop();
      publish();
      useConnectionStore.setState({ error: "" });
      return;
    }
    const poll = async () => {
      try {
        await tracker.refresh(async () => (await Proxy.Connections()).connections ?? []);
        if (!disposed) { publish(); useConnectionStore.setState({ error: "" }); }
      } catch (error) {
        // A failed request isn't an empty snapshot; keep the last good data.
        if (!disposed) useConnectionStore.setState({ error: errText(error) });
      } finally {
        if (!disposed) timer = setTimeout(poll, 1000);
      }
    };
    poll();
    return () => { disposed = true; clearTimeout(timer); tracker.invalidate(); };
  }, [running, profile]);
}
