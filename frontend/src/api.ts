// The Go services, re-exported under short names.
export * as App from "../bindings/github.com/localhost-copilot/clashferry/internal/gui/appservice";
export * as Proxy from "../bindings/github.com/localhost-copilot/clashferry/internal/gui/proxyservice";
export * as Profiles from "../bindings/github.com/localhost-copilot/clashferry/internal/gui/profileservice";
export * as Settings from "../bindings/github.com/localhost-copilot/clashferry/internal/gui/settingsservice";
export type { App as AppInfo, Group, Member, Patch, Provider } from "../bindings/github.com/localhost-copilot/clashferry/internal/gui/models";
export type { Lookup, State, Event, HelperStatus, GeoInfo, Connectivity, ClientRate, Egress, DNSEgress, ProxyEgress, Network, LatencySample } from "../bindings/github.com/localhost-copilot/clashferry/internal/backend/models";
export type { NetworkRule, NetworkActions } from "../bindings/github.com/localhost-copilot/clashferry/internal/settings/models";
export type { Profile, ImportRequest } from "../bindings/github.com/localhost-copilot/clashferry/internal/profiles/models";
export type { Settings as SettingsT } from "../bindings/github.com/localhost-copilot/clashferry/internal/settings/models";
export type { Traffic, Memory, Log, Connection, Connections, Rule, RuleProvider } from "../bindings/github.com/localhost-copilot/clashferry/internal/mihomoapi/models";
export type { Rule as UserRule } from "../bindings/github.com/localhost-copilot/clashferry/internal/userrules/models";
