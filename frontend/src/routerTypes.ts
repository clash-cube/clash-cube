export type AccessPolicy = { mode: "all" | "include" | "exclude"; addresses: string[]; macs: string[] };
export type RouterState = { enabled: boolean; active: boolean; interface: string; error?: string; access?: AccessPolicy };
export type RouterMaintenance = {
  config: { geoIntervalHours: number; updateFeed: string };
  geo: { attempt: number; success: number; failed: boolean };
};
export type CoreUpdate = {
  installed: string; latest?: string; available: boolean; bytes?: number;
  job: { phase: string; version?: string; started?: number; error?: string };
};
export type RouterDiagnostics = {
  system: string; kernel: string; uptime: number; memoryTotal: number; memoryFree: number;
  appVersion: string; coreVersion: string; core: string;
  storage: { path: string; total: number; free: number }[];
  packages: { name: string; version: string }[];
  checks: { name: string; ok: boolean }[];
};
export type RouterAPI = {
  Get(): Promise<RouterState>;
  SetEnabled(on: boolean): Promise<RouterState>;
  SetAccess(policy: AccessPolicy): Promise<RouterState>;
  Maintenance(): Promise<RouterMaintenance>;
  SetMaintenance(config: RouterMaintenance["config"]): Promise<RouterMaintenance>;
  Updates(): Promise<CoreUpdate>;
  CheckUpdates(): Promise<CoreUpdate>;
  UpdateCore(): Promise<CoreUpdate>;
  Diagnostics(): Promise<RouterDiagnostics>;
};
