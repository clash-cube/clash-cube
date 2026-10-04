// The Go services, re-exported under short names.
export * as App from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/appservice";
export * as Proxy from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/proxyservice";
export * as Profiles from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/profileservice";
export * as Settings from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/settingsservice";
export type { App as AppInfo, Group, Member, Patch, Provider } from "../bindings/github.com/localhost-copilot/mihomobar/internal/gui/models";
export type { Lookup, State, Event, HelperStatus, GeoInfo, Connectivity, ClientRate, Egress, DNSEgress, ProxyEgress, Network } from "../bindings/github.com/localhost-copilot/mihomobar/internal/backend/models";
export type { NetworkRule, NetworkActions } from "../bindings/github.com/localhost-copilot/mihomobar/internal/settings/models";
export type { Profile, ImportRequest } from "../bindings/github.com/localhost-copilot/mihomobar/internal/profiles/models";
export type { Settings as SettingsT } from "../bindings/github.com/localhost-copilot/mihomobar/internal/settings/models";
export type { Traffic, Memory, Log, Connection, Connections, Rule, RuleProvider } from "../bindings/github.com/localhost-copilot/mihomobar/internal/mihomoapi/models";
export type { Rule as UserRule } from "../bindings/github.com/localhost-copilot/mihomobar/internal/userrules/models";
