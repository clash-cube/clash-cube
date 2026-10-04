import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { Settings as S } from "../api";
import type { Hotkey } from "../../bindings/github.com/localhost-copilot/clashferry/internal/gui/models";
import { Close } from "./Icons";
import { errText, toast } from "./Toast";

const ACTIONS: Record<string, [string, string]> = {
  panel: ["Show the menu bar panel", "Opens or closes it, from any app"],
  main: ["Show the main window", "Brings it forward, or hides it when it is in front"],
  mode: ["Switch outbound mode", "Rule → Global → Direct, in turn"],
  systemProxy: ["Turn the system proxy on or off", ""],
  tun: ["Turn enhanced mode on or off", ""],
};

// The keys as macOS writes them: ⌃⌥⇧⌘ then the key.
const GLYPH: Record<string, string> = { Ctrl: "⌃", Option: "⌥", Shift: "⇧", Cmd: "⌘" };
const KEY_GLYPH: Record<string, string> = {
  Space: "Space", Return: "↩", Tab: "⇥", Backspace: "⌫", Delete: "⌦", Left: "←", Right: "→", Up: "↑", Down: "↓",
  Home: "↖", End: "↘", "Page up": "⇞", "Page down": "⇟",
};
export function glyphs(keys: string) {
  if (!keys) return "";
  const parts = keys.split("+");
  const key = parts.pop()!;
  return parts.map((m) => GLYPH[m] ?? m).join("") + (KEY_GLYPH[key] ?? key);
}

// The key a keydown names, as the Go side writes it; "" for a modifier.
// By the physical key, so ⌥K is K and not the ˚ it types.
function keyOf(e: KeyboardEvent): string {
  const c = e.code;
  if (/^Key[A-Z]$/.test(c)) return c.slice(3);
  if (/^Digit\d$/.test(c)) return c.slice(5);
  if (/^F\d{1,2}$/.test(c)) return c;
  const named: Record<string, string> = {
    Space: "Space", Enter: "Return", Tab: "Tab", Backspace: "Backspace", Delete: "Delete",
    ArrowLeft: "Left", ArrowRight: "Right", ArrowUp: "Up", ArrowDown: "Down", Home: "Home", End: "End",
    PageUp: "Page up", PageDown: "Page down", Minus: "-", Equal: "=", BracketLeft: "[", BracketRight: "]",
    Quote: "'", Semicolon: ";", Backslash: "\\", Comma: ",", Period: ".", Slash: "/", Backquote: "`",
  };
  return named[c] ?? "";
}

function modsOf(e: KeyboardEvent): string[] {
  return [e.ctrlKey && "Ctrl", e.altKey && "Option", e.shiftKey && "Shift", e.metaKey && "Cmd"].filter(Boolean) as string[];
}

// Hotkeys is the Settings section for the global shortcuts: one row an
// action, its recorder on the right.
export function Hotkeys() {
  const t = useT();
  const [keys, setKeys] = useState<Hotkey[]>([]);
  const [filling, setFilling] = useState(false);
  useEffect(() => { S.Hotkeys().then((k) => setKeys(k ?? [])).catch(() => {}); }, []);
  const label = (a: string) => t(ACTIONS[a]?.[0] ?? a);
  // fills only the empty ones; the user's own are kept
  const recommend = async () => {
    setFilling(true);
    try {
      const r = await S.UseRecommendedHotkeys();
      setKeys(r.hotkeys ?? keys);
      if (r.skipped?.length) toast(t("Set the rest; these shortcuts are taken: {names}", { names: r.skipped.map(label).join(t(", ")) }), "", 5000);
      else toast(t("Recommended shortcuts set"));
    } catch (e) { toast(errText(e), "err", 4000); }
    setFilling(false);
  };
  const empty = keys.some((k) => !k.keys);
  return (
    <>
      {empty && keys.length > 0 && (
        <div className="row hotkeys-recommend">
          <div className="who">
            <div className="name">{t("Use recommended shortcuts")}</div>
            <div className="sub">{t("⌃⌥⌘ and a letter: P panel, M window, O mode, S system proxy, E enhanced mode. Only the empty ones are filled.")}</div>
          </div>
          <div className="end"><button className="btn small" disabled={filling} onClick={recommend}>{t("Use")}</button></div>
        </div>
      )}
      {keys.map((k) => (
        <div className="row" key={k.action}>
          <div className="who">
            <div className="name">{label(k.action)}</div>
            {(k.error || ACTIONS[k.action]?.[1]) && <div className={"sub" + (k.error ? " err" : "")}>{k.error ? t(k.error) : t(ACTIONS[k.action][1])}</div>}
          </div>
          <div className="end">
            <Recorder value={k.keys} label={label}
              onSet={async (v) => { setKeys((await S.SetHotkey(k.action, v)) ?? keys); }} />
          </div>
        </div>
      ))}
    </>
  );
}

// Recorder is a shortcut field as macOS draws one: click, then press the
// keys. The modifiers held show as they're pressed; Esc cancels and ⌫
// clears. A combination refused says why, and the field shakes.
function Recorder({ value, label, onSet }: { value: string; label: (action: string) => string; onSet: (v: string) => Promise<void> }) {
  const t = useT();
  const [recording, setRecording] = useState(false);
  const [held, setHeld] = useState("");
  const [problem, setProblem] = useState("");
  const [shake, setShake] = useState(0);
  const ref = useRef<HTMLButtonElement>(null);

  const stop = () => { setRecording(false); setHeld(""); S.RecordHotkey(false); };
  useEffect(() => {
    if (!recording) return;
    S.RecordHotkey(true);
    const down = async (e: KeyboardEvent) => {
      e.preventDefault(); e.stopPropagation();
      const mods = modsOf(e);
      const key = keyOf(e);
      if (!key) { setHeld(mods.map((m) => GLYPH[m]).join("")); return; }
      if (mods.length === 0 && key === "Backspace") { stop(); setProblem(""); await onSet("").catch(() => {}); return; }
      const keys = [...mods, key].join("+");
      try {
        await onSet(keys);
        setProblem(""); stop();
      } catch (err) {
        // a hotkeys.Problem comes as the error's cause
        const p = (err as { cause?: { text?: string; action?: string } })?.cause;
        const text = p?.text ?? errText(err);
        setProblem(t(text, { action: p?.action ? label(p.action) : "" }));
        setHeld(""); setShake((n) => n + 1);
      }
    };
    const up = (e: KeyboardEvent) => setHeld(modsOf(e).map((m) => GLYPH[m]).join(""));
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape" && modsOf(e).length === 0) { e.preventDefault(); e.stopPropagation(); stop(); setProblem(""); } };
    const away = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) { stop(); setProblem(""); } };
    window.addEventListener("keydown", esc, true);
    window.addEventListener("keydown", down, true);
    window.addEventListener("keyup", up, true);
    window.addEventListener("mousedown", away, true);
    window.addEventListener("blur", stop);
    return () => {
      window.removeEventListener("keydown", esc, true);
      window.removeEventListener("keydown", down, true);
      window.removeEventListener("keyup", up, true);
      window.removeEventListener("mousedown", away, true);
      window.removeEventListener("blur", stop);
    };
  }, [recording]);
  // leaving the page while recording puts the shortcuts back
  useEffect(() => () => { S.RecordHotkey(false); }, []);

  return (
    <div className="hotkey">
      {problem && <span className="hotkey-problem">{problem}</span>}
      <button ref={ref} key={shake} className={"hotkey-field" + (recording ? " recording" : "") + (value ? " set" : "") + (shake ? " shake" : "")}
        onClick={() => { setProblem(""); setRecording(!recording); if (recording) stop(); }}>
        {recording ? (held ? <span className="keys">{held}</span> : <span className="hint">{t("Type a shortcut…")}</span>)
          : value ? <span className="keys">{glyphs(value)}</span> : <span className="hint">{t("Record shortcut")}</span>}
      </button>
      {value && !recording && (
        <button className="icon hotkey-clear" title={t("Clear")} aria-label={t("Clear")} onClick={() => { setProblem(""); onSet("").catch(() => {}); }}><Close size={10} /></button>
      )}
    </div>
  );
}
