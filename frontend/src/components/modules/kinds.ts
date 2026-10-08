import type { ComponentType } from "react";
import type { Module } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { Node } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import type { Group } from "../../api";

// A module kind is how one sort of module shows and is edited: YAML, a
// route, a port. The list and its rows know nothing of any kind; they ask
// the registry (registry.ts). Adding one: its Go Kind, then a ModuleKind
// here in a file of its own, listed in KINDS.

// RowCtx is what a row knows beyond its module.
export type RowCtx = {
  profile: string; // the profile in use; "" for none
  nodes: Node[] | null; // its nodes; null with the core stopped
  groups: Group[] | null; // its groups; null with the core stopped
  group?: Group; // the group the module made, if any
  open: boolean;
  onOpen: () => void;
  onPick: (g: Group, name: string) => void;
  // saves the module as changed on its row; false when it was refused
  onSave: (m: Module) => Promise<boolean>;
};

export type EditorProps = {
  module: Module;
  open: boolean;
  onCancel: () => void;
  onSave: (m: Module) => Promise<void>;
};

// An entry in the add menu, and in the list's empty state.
export type NewEntry = {
  key: string;
  name: string;
  hint: string;
  added?: boolean;
  // the draft it opens, laid over profile ("" for every one)
  make: (profile: string) => Module | Promise<Module>;
};
export type NewSection = { title: string; entries: NewEntry[] };

export type ModuleKind = {
  id: string;
  is: (m: Module) => boolean;
  // the row's second line
  Summary: ComponentType<{ m: Module; ctx: RowCtx }>;
  // controls on the row's right, before its menu and switch
  End?: ComponentType<{ m: Module; ctx: RowCtx }>;
  // unrolled under the row, and for a new one
  Editor: ComponentType<EditorProps>;
  // the add menu's sections for it, given the modules there are. A hook,
  // called on every render in the registry's order.
  useNew?: (mods: Module[]) => NewSection[];
  // a copy of m to add after it; false when a copy can't coexist with it
  duplicate?: false | ((m: Module) => Promise<Module>);
};
