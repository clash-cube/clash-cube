import type { Module } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { ModuleKind } from "./kinds";
import { yamlKind } from "./yaml";
import { routeKind } from "./route";
import { portKind } from "./port";

// KINDS is every kind but YAML, which takes the modules none of them
// claims. Their order is the add menu's, after YAML's templates.
const KINDS: ModuleKind[] = [routeKind, portKind];

export const kindOf = (m: Module): ModuleKind => KINDS.find((k) => k.is(m)) ?? yamlKind;

// the kinds as the add menu lists them
export const MENU: ModuleKind[] = [yamlKind, ...KINDS];
