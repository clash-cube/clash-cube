import { isWeb } from "../platform";
import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Profiles as P, App, ProfileFiles, type Profile, type ImportRequest } from "../api";
import { Popover, Menu } from "../components/Popover";
import { toast, toastError } from "../components/Toast";
import { Globe, File, More, Plus, Refresh } from "../components/Icons";
import { ago, bytes } from "../format";
import { Segmented } from "../components/Segmented";
import { Modules } from "../components/modules";
import { useLeaving } from "../useLeaving";
import { useFlip } from "../useFlip";
import { RefusalBanner, RuntimeConfig } from "../components/RuntimeConfig";
const ProfileEditor = lazy(() => import("../components/ProfileEditor").then((m) => ({ default: m.ProfileEditor })));

const TABS = ["profiles", "modules", "merged"] as const;
type Tab = typeof TABS[number];

export function Profiles() {
  const t = useT();
  const profiles = useStore((s) => s.profiles);
  const current = useStore((s) => s.state?.profile);
  const [importAt, setImportAt] = useState<HTMLElement | null>(null);
  const importButton = useRef<HTMLButtonElement>(null);
  const pendingImport = useStore((s) => s.imports[0]);
  const [activeImport, setActiveImport] = useState<ImportRequest | null>(null);
  const [updatingAll, setUpdatingAll] = useState(false);
  const [flash, setFlash] = useState("");
  const [editing, setEditing] = useState<Profile | null>(null);
  const [tab, setTab] = useState<Tab>(TABS.find((x) => "#" + x === location.hash) ?? "profiles");
  const rows = useLeaving(profiles, (p) => p.id);
  const list = useRef<HTMLDivElement>(null);
  useFlip(list, JSON.stringify([tab, rows.map(({ item }) => item.id)]), { axis: "y", enter: false });
  // bumped by the banner's Show, to open the refusal again
  const [focus, setFocus] = useState(0);
  const moduleDraft = useStore((s) => s.moduleDraft);
  useEffect(() => { if (moduleDraft) setTab("modules"); }, [moduleDraft]);

  useEffect(() => {
    if (pendingImport && !importAt) {
      setTab("profiles");
      setActiveImport(pendingImport);
      setImportAt(importButton.current);
    }
  }, [pendingImport, importAt]);

  const closeImport = () => {
    if (activeImport) useStore.setState((s) => ({ imports: s.imports.filter((r) => r !== activeImport) }));
    setActiveImport(null);
    setImportAt(null);
  };

  const flashRow = (id: string) => { setFlash(id); setTimeout(() => setFlash(""), 900); };
  const copyProfile = async (p: Profile) => {
    const cp = await P.Duplicate(p.id, t("{name} (copy)", { name: p.name }));
    if (p.id === current) { await P.Use(cp.id); toast(t("Now using {name}, which you can edit", { name: cp.name })); }
    else toast(t("Made {name}", { name: cp.name }));
    flashRow(cp.id);
    return cp;
  };
  const activeProfile = profiles.find((p) => p.id === current);

  const updateAll = async () => {
    setUpdatingAll(true);
    try {
      const failed = await P.UpdateAll();
      if (failed?.length) toast(failed[0], "err", 4000); else toast(t("Updated {name}", { name: t("Profiles") }));
    } catch (e) { toastError(e); }
    setUpdatingAll(false);
  };

  return (
    <div className="view profiles-view">
      <div className="view-head">
        <Segmented className="track small" value={tab} onChange={setTab} options={[
          { value: "profiles", label: `${t("Profiles")} ${profiles.length}` },
          { value: "modules", label: t("Modules") },
          { value: "merged", label: t("Merged") },
        ]} />
        <div className="view-tools page-tab-tools" key={tab}>
          {tab === "profiles" && <>
          {ProfileFiles && activeProfile && <button className="btn small" onClick={() => setEditing(activeProfile)}>{t(activeProfile.url ? "View configuration" : "Edit configuration")}</button>}
          <button className="btn small" disabled={updatingAll} onClick={updateAll}><Refresh size={13} />{updatingAll ? t("Updating…") : t("Update all")}</button>
          <button ref={importButton} className="btn small primary" onClick={(e) => { if (!importAt) setImportAt(e.currentTarget); }}><Plus size={13} />{t("Import")}</button>
          </>}
        </div>
      </div>
      <RefusalBanner onEdit={ProfileFiles && activeProfile ? () => setEditing(activeProfile) : undefined} onShow={tab === "merged" ? undefined : () => { setTab("merged"); setFocus((n) => n + 1); }} />
      {editing && ProfileFiles && <Suspense fallback={<p role="status">{t("Loading…")}</p>}><ProfileEditor key={editing.id} profile={editing} active={editing.id === current} onClose={() => setEditing(null)} onCopy={async () => { setEditing(await copyProfile(editing)); }} /></Suspense>}
      <ImportPopover key={activeImport ? JSON.stringify(activeImport) : "manual"} request={activeImport} anchor={importAt} onClose={closeImport} onDone={(p) => { closeImport(); flashRow(p.id); }} />
      <div key={tab} className="page-tab-content">
      {tab === "merged" ? <RuntimeConfig focus={focus} /> : tab === "modules" ? <Modules /> : <div className="list" ref={list}>
        {rows.map(({ item: p, leaving }) => <div key={p.id} data-key={p.id} className={leaving ? "profile-leaving" : undefined} {...(leaving ? { inert: "" as const } : {})}><ProfileRow p={p} current={p.id === current} flash={flash === p.id} onFlash={() => flashRow(p.id)} onEdit={() => setEditing(p)} onCopy={() => copyProfile(p).then(() => {})} /></div>)}
      </div>}
      </div>
    </div>
  );
}

function ProfileRow({ p, current, flash, onFlash, onEdit, onCopy }: { p: Profile; current: boolean; flash: boolean; onFlash: () => void; onEdit: () => void; onCopy: () => Promise<void> }) {
  const t = useT();
  const [menuAt, setMenuAt] = useState<HTMLElement | null>(null);
  const [updating, setUpdating] = useState(false);
  const [armed, setArmed] = useState(false);
  // the profile's own modules, which go with it
  const [mods, setMods] = useState(0);
  const [renaming, setRenaming] = useState(false);
  const busy = useStore((s) => s.state?.busy);

  useEffect(() => {
    if (!armed) return;
    const tm = setTimeout(() => setArmed(false), 3000);
    return () => clearTimeout(tm);
  }, [armed]);

  const use = async () => {
    if (current) return;
    try { await P.Use(p.id); toast(t("Switched to {name}", { name: p.name })); onFlash(); } catch (e) { toastError(e); }
  };
  const update = async () => {
    setUpdating(true);
    try { await P.Update(p.id); toast(t("Updated {name}", { name: p.name })); onFlash(); } catch (e) { toastError(e); }
    setUpdating(false);
  };
  const openInEditor = async () => {
    try {
      await P.OpenInEditor(p.id);
      if (p.url && !isWeb) toast(t("Edits here are lost at the next update. Make an editable copy, or use a module."), "", 5000);
    } catch (e) { toastError(e); }
  };
  const remove = async () => {
    if (!armed) {
      setArmed(true);
      P.Modules().then((ms) => setMods((ms ?? []).filter((m) => m.profile === p.id).length)).catch(() => setMods(0));
      return;
    }
    try { await P.Remove(p.id); toast(t("Removed")); } catch (e) { toastError(e); }
  };

  const used = (p.upload ?? 0) + (p.download ?? 0);
  const total = p.total ?? 0;
  const pct = total ? Math.min(100, (used / total) * 100) : 0;
  const expired = p.expire ? p.expire * 1000 < Date.now() : false;
  const sub = [
    p.url ? new URL(p.url).host : t("Local file"),
    t("Updated {t}", { t: ago(p.updated, t) }),
    p.expire ? (expired ? t("Expired") : t("Expires {d}", { d: new Date(p.expire * 1000).toLocaleDateString() })) : "",
  ].filter(Boolean).join(" · ");

  return (
    <div className={"row click profile" + (current ? " current" : "") + (flash ? " flash" : "")} onClick={use}>
      <span className={"ic" + (current ? " on" : "")}>{p.url ? <Globe /> : <File />}</span>
      <div className="who">
        {renaming ? (
          <RenameInput value={p.name} done={async (name) => { setRenaming(false); if (name && name !== p.name) { try { await P.Edit(p.id, name, p.interval); } catch (e) { toastError(e); } } }} />
        ) : (
          <div className="name">{p.name}{current && <span className="badge">{t("In use")}</span>}</div>
        )}
        <div className="sub">{sub}</div>
        {total > 0 && (
          <div className="usage" title={`${bytes(used)} / ${bytes(total)}`}>
            <div className="bar"><i style={{ width: pct + "%" }} className={pct > 90 ? "hot" : ""} /></div>
            <span>{bytes(used)} / {bytes(total)}</span>
          </div>
        )}
      </div>
      <div className="end" onClick={(e) => e.stopPropagation()}>
        {current && busy === "reloading" && <span className="sub">{t("Reloading…")}</span>}
        {p.url && <button className={"icon" + (updating ? " spin" : "")} title={t("Update")} onClick={update} disabled={updating}><Refresh size={14} /></button>}
        <button className={"icon" + (menuAt ? " on" : "")} aria-label={t("Profile actions")} onClick={(e) => setMenuAt(menuAt ? null : e.currentTarget)}><More size={14} /></button>
        {armed && <button className="btn small danger armed" onClick={remove}>{mods ? t("Click again to remove, with its {n} modules", { n: mods }) : t("Click again to remove")}</button>}
      </div>
      <Popover anchor={menuAt} open={!!menuAt} onClose={() => setMenuAt(null)} align="end">
        <Menu close={() => setMenuAt(null)} items={[
          ...(!current ? [{ label: t("Use"), onClick: use }] : []),
          ...(p.url ? [{ label: t("Copy URL"), onClick: () => { App.CopyText(p.url!); toast(t("Copied")); } }] : []),
          { label: t("Rename"), onClick: () => setRenaming(true) },
          ...(ProfileFiles ? [{ label: t(p.url ? "View configuration" : "Edit configuration"), onClick: onEdit }] : []),
          ...(p.url ? [{ label: t("Make an editable copy"), onClick: () => { void onCopy().catch(toastError); } }] : []),
          { label: t(isWeb ? "Download configuration" : "Open in editor"), onClick: openInEditor },
          ...(!isWeb ? [{ label: t("Show in Finder"), onClick: () => P.Reveal(p.id).catch(toastError) }] : []),
          ...(!current ? ["sep" as const, { label: t("Remove"), danger: true, onClick: remove }] : []),
        ]} />
      </Popover>
    </div>
  );
}

function RenameInput({ value, done }: { value: string; done: (v: string) => void }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { ref.current?.focus(); ref.current?.select(); }, []);
  return (
    <input
      ref={ref}
      className="input rename"
      defaultValue={value}
      onClick={(e) => e.stopPropagation()}
      onBlur={(e) => done(e.currentTarget.value.trim())}
      onKeyDown={(e) => { if (e.key === "Enter") e.currentTarget.blur(); if (e.key === "Escape") done(value); }}
    />
  );
}

const INTERVALS = [0, 6, 12, 24, 72];

function ImportPopover({ anchor, request, onClose, onDone }: { anchor: HTMLElement | null; request: ImportRequest | null; onClose: () => void; onDone: (p: Profile) => void }) {
  const t = useT();
  const [url, setUrl] = useState(request?.url ?? "");
  const [name, setName] = useState(request?.name ?? "");
  const [interval, setInterval] = useState(24);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { if (anchor) setTimeout(() => input.current?.focus(), 60); }, [anchor]);

  const submit = async () => {
    if (busy || !url.trim()) return;
    setBusy(true);
    try {
      const p = await P.ImportURL(url.trim(), name.trim(), interval);
      toast(t("Imported {name}", { name: p.name }));
      setUrl(""); setName("");
      onDone(p);
    } catch (e) { toastError(e); }
    setBusy(false);
  };
  const file = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const p = await P.ImportFile();
      if (p?.id) { toast(t("Imported {name}", { name: p.name })); onDone(p); }
    } catch (e) { toastError(e); }
    setBusy(false);
  };

  return (
    <Popover anchor={anchor} open={!!anchor} onClose={() => { if (!busy) onClose(); }} align="end" width={360}>
      <form className="pop-form" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <h3>{t("Import from URL")}</h3>
        <label>{t("Subscription URL")}<input ref={input} className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" /></label>
        <label>{t("Name (optional)")}<input className="input" value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label>{t("Auto update")}
          <select className="input" value={interval} onChange={(e) => setInterval(+e.target.value)}>
            {INTERVALS.map((h) => <option key={h} value={h}>{h ? t("Every {n}h", { n: h }) : t("Never")}</option>)}
          </select>
        </label>
        <div className="foot">
          <button type="button" className="btn" disabled={busy} onClick={file}>{t("Import a file…")}</button>
          <div className="grow" />
          <button type="submit" className="btn primary" disabled={busy || !url.trim()}>{busy ? t("Updating…") : t("Import")}</button>
        </div>
      </form>
    </Popover>
  );
}
