// The Go services, re-exported under short names.
export * as App from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/appservice";
export * as Proxy from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/proxyservice";
export * as Profiles from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/profileservice";
export * as Settings from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/settingsservice";
export type { Group, Member, Patch, Provider } from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/models";
export type { State, Event, HelperStatus, Connectivity, ClientRate, Egress, DNSEgress, ProxyEgress } from "../bindings/github.com/localhost-copilot/mihomobar/internal/backend/models";
export type { Profile } from "../bindings/github.com/localhost-copilot/mihomobar/internal/profiles/models";
export type { Settings as SettingsT } from "../bindings/github.com/localhost-copilot/mihomobar/internal/settings/models";
export type { Traffic, Memory, Log, Connection, Connections, Rule } from "../bindings/github.com/localhost-copilot/mihomobar/internal/mihomoapi/models";
