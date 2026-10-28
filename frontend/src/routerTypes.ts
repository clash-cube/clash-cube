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
  Devices(): Promise<DevicesView>;
  ApplyDevice(request: DeviceRequest): Promise<DevicesView>;
  PauseDevice(id: string, revision: string, paused: boolean): Promise<DevicesView>;
  ReleaseDevice(id: string, revision: string): Promise<DevicesView>;
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

export type DeviceTarget = { name: string; profile: string; provider: string; fingerprint: string; type: string };
export type LANDevice = { name: string; mac: string; ip: string; reserved: boolean; online: boolean };
export type DevicePolicy = { id: string; name: string; mac: string; ip: string; slot: number; target: DeviceTarget; paused: boolean; pending: boolean; protection: string; connectivity: string };
export type DevicesView = { revision: string; devices: LANDevice[]; policies: DevicePolicy[]; nodes: (DeviceTarget & { reason?: string })[]; error?: string };
export type DeviceRequest = { revision: string; id: string; name: string; mac: string; ip: string; target: DeviceTarget; reserve: boolean };
