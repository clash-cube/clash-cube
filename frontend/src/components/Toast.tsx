import { create } from "zustand";

type Toast = { id: number; text: string; kind: "ok" | "err" | ""; leaving?: boolean };

const useToasts = create<{ list: Toast[] }>(() => ({ list: [] }));
let seq = 0;

// toast shows a short outcome at the window's foot.
export function toast(text: string, kind: Toast["kind"] = "ok", ms = 2200) {
  const id = ++seq;
  useToasts.setState((s) => ({ list: [...s.list.slice(-2), { id, text, kind }] }));
  setTimeout(() => {
    useToasts.setState((s) => ({ list: s.list.map((t) => (t.id === id ? { ...t, leaving: true } : t)) }));
    setTimeout(() => useToasts.setState((s) => ({ list: s.list.filter((t) => t.id !== id) })), 200);
  }, ms);
}

export const toastError = (e: unknown) => toast(errText(e), "err", 4000);

export function errText(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  if (e && typeof e === "object" && "message" in e) return String((e as { message: unknown }).message);
  return String(e);
}

export function Toasts() {
  const list = useToasts((s) => s.list);
  return (
    <div className="toasts">
      {list.map((t) => <div key={t.id} className={"toast " + t.kind + (t.leaving ? " leaving" : "")}>{t.text}</div>)}
    </div>
  );
}
