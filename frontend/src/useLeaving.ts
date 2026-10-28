import { useEffect, useRef, useState } from "react";
import { calm } from "./components/Sidebar";

// Retain removed items in their old slots; restoring a key cancels its exit.
export function useLeaving<T>(items: T[], key: (item: T) => string, ms = 220): { item: T; leaving: boolean }[] {
  const [previous, setPrevious] = useState(items);
  const [rows, setRows] = useState(() => items.map((item) => ({ item, leaving: false })));
  if (previous.length !== items.length || previous.some((item, i) => item !== items[i])) {
    const next = items.map((item) => ({ item, leaving: false }));
    const keys = new Set(items.map(key));
    if (!calm()) rows.forEach((row, i) => {
      if (!keys.has(key(row.item))) next.splice(i, 0, { ...row, leaving: true });
    });
    setPrevious(items);
    setRows(next);
  }
  const exits = JSON.stringify(rows.filter((row) => row.leaving).map((row) => key(row.item)));
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  useEffect(() => {
    const leaving = new Set<string>(JSON.parse(exits));
    for (const [id, timer] of timers.current) if (!leaving.has(id)) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
    // Each removal owns its deadline; later removals must not prolong it.
    for (const id of leaving) if (!timers.current.has(id)) {
      timers.current.set(id, setTimeout(() => {
        timers.current.delete(id);
        setRows((rows) => rows.filter((row) => !row.leaving || key(row.item) !== id));
      }, calm() ? 0 : ms));
    }
  }, [exits, ms]);
  useEffect(() => () => { timers.current.forEach(clearTimeout); timers.current.clear(); }, []);
  return rows;
}
