import type { ConnectionSort } from "./connections";

export const connectionColumns = [
  { id: "host", label: "Host", width: 220, min: 140 },
  { id: "process", label: "Process", width: 150, min: 100 },
  { id: "source", label: "Source", width: 190, min: 120 },
  { id: "network", label: "Network", width: 100, min: 80 },
  { id: "rule", label: "Rule", width: 180, min: 100 },
  { id: "chain", label: "Chain", width: 180, min: 100 },
  { id: "speed", label: "Speed", width: 170, min: 100 },
  { id: "up", label: "Upload speed", width: 100, min: 80 },
  { id: "down", label: "Download speed", width: 100, min: 80 },
  { id: "upload", label: "Uploaded", width: 100, min: 80 },
  { id: "download", label: "Downloaded", width: 100, min: 80 },
  { id: "total", label: "Total", width: 100, min: 80 },
  { id: "time", label: "Start time", width: 110, min: 90 },
] as const satisfies readonly { id: ConnectionSort; label: string; width: number; min: number }[];

export type ConnectionColumn = typeof connectionColumns[number]["id"];
export const defaultColumns: ConnectionColumn[] = ["host", "chain", "speed", "total", "time"];
export const isTextColumn = (id: ConnectionColumn) => ["host", "process", "source", "network", "rule", "chain"].includes(id);
export function clampColumnWidth(id: ConnectionColumn, width: number) {
  const column = connectionColumns.find((c) => c.id === id)!;
  return Number.isFinite(width) ? Math.round(Math.max(column.min, Math.min(600, width))) : column.width;
}

export function parseColumnLayout(value: { columns?: unknown; widths?: unknown }) {
  const selected = Array.isArray(value.columns) ? value.columns : defaultColumns;
  // Host remains visible so a row always has an identifiable destination.
  const columns = connectionColumns.filter((c) => c.id === "host" || selected.includes(c.id)).map((c) => c.id);
  const widths: Partial<Record<ConnectionColumn, number>> = {};
  if (value.widths && typeof value.widths === "object") {
    for (const column of connectionColumns) {
      const width = (value.widths as Record<string, unknown>)[column.id];
      if (typeof width === "number" && Number.isFinite(width)) widths[column.id] = clampColumnWidth(column.id, width);
    }
  }
  return { columns, widths };
}
