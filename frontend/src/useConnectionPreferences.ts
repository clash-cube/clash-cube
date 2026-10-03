import { useState } from "react";
import { defaultConnectionPreferences, parseConnectionPreferences, type ConnectionPreferences } from "./connectionPreferences";
import { toastError } from "./components/Toast";

const key = "connection-display-preferences";

export function useConnectionPreferences() {
  const [preferences, setPreferences] = useState(() => {
    try { return parseConnectionPreferences(localStorage.getItem(key)); }
    catch { return { ...defaultConnectionPreferences }; }
  });
  const update = (patch: Partial<ConnectionPreferences>) => {
    const next = { ...preferences, ...patch };
    setPreferences(next);
    try { localStorage.setItem(key, JSON.stringify(next)); }
    catch (error) { toastError(error); }
  };
  return { ...preferences, update };
}
