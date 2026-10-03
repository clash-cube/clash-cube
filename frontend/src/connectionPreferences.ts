import type { ConnectionGroupBy, ConnectionSort } from "./connections";

export type ConnectionPreferences = {
  by: ConnectionGroupBy;
  net: "all" | "tcp" | "udp";
  sort: ConnectionSort;
  ascending: boolean;
};

export const defaultConnectionPreferences: ConnectionPreferences = {
  by: "process", net: "all", sort: "time", ascending: false,
};

// Validate each saved field independently so stale or damaged preferences do
// not leave controls without a selection or hide the connection list.
export function parseConnectionPreferences(json: string | null): ConnectionPreferences {
  try {
    const value = JSON.parse(json ?? "null");
    if (!value || typeof value !== "object" || Array.isArray(value)) return { ...defaultConnectionPreferences };
    return {
      by: ["none", "process", "host", "rule", "source"].includes(value.by) ? value.by : defaultConnectionPreferences.by,
      net: ["all", "tcp", "udp"].includes(value.net) ? value.net : defaultConnectionPreferences.net,
      sort: ["time", "host", "up", "down", "upload", "download"].includes(value.sort) ? value.sort : defaultConnectionPreferences.sort,
      ascending: typeof value.ascending === "boolean" ? value.ascending : defaultConnectionPreferences.ascending,
    };
  } catch { return { ...defaultConnectionPreferences }; }
}
