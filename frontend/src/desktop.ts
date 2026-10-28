export * as App from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/appservice";
export * as Proxy from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/proxyservice";
export * as Profiles from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/profileservice";
export * as Settings from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/settingsservice";
export { Events, Window } from "@wailsio/runtime";
import type { RouterAPI } from "./routerTypes";
import type { ProfileFilesAPI } from "./profileTypes";
export const Router: RouterAPI | null = null;
export const ProfileFiles: ProfileFilesAPI | null = null;
