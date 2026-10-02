import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Profiles as P, App, type Profile } from "../api";
import { Popover, Menu } from "../components/Popover";
import { toast, toastError } from "../components/Toast";
import { Globe, File, More, Plus, Refresh } from "../components/Icons";
import { ago, bytes } from "../format";

export function Profiles() {
  const t = useT();
  const profiles = useStore((s) => s.profiles);
  const current = useStore((s) => s.state?.profile);
  const [importAt, setImportAt] = useState<HTMLElement | null>(null);
  const [updatingAll, setUpdatingAll] = useState(false);
  const [flash, setFlash] = useState("");

  const flashRow = (id: string) => { setFlash(id); setTimeout(() => setFlash(""), 900); };

  const updateAll = async () => {
    setUpdatingAll(true);
    try {
      const failed = await P.UpdateAll();
      if (failed?.length) toast(failed[0], "err", 4000); else toast(t("Updated {name}", { name: t("Profiles") }));
    } catch (e) { toastError(e); }
    setUpdatingAll(false);
  };

  return (
    <div className="view">
      <div className="view-head">
        <h2>{t("Profiles")}</h2>
        <span className="sub">{profiles.length}</span>
        <div className="grow" />
        <button className="btn small" disabled={updatingAll} onClick={updateAll}><Refresh size={13} />{updatingAll ? t("Updating…") : t("Update all")}</button>
        <button className="btn small primary" onClick={(e) => setImportAt(importAt ? null : e.currentTarget)}><Plus size={13} />{t("Import")}</button>
      </div>
      <ImportPopover anchor={importAt} onClose={() => setImportAt(null)} onDone={(p) => { setImportAt(null); flashRow(p.id); }} />
      <div className="list">
        {profiles.map((p) => <ProfileRow key={p.id} p={p} current={p.id === current} flash={flash === p.id} onFlash={() => flashRow(p.id)} />)}
      </div>
    </div>
  );
}

function ProfileRow({ p, current, flash, onFlash }: { p: Profile; current: boolean; flash: boolean; onFlash: () => void }) {
  const t = useT();
  const [menuAt, setMenuAt] = useState<HTMLElement | null>(null);
  const [updating, setUpdating] = useState(false);
  const [armed, setArmed] = useState(false);
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
  const remove = async () => {
    if (!armed) { setArmed(true); return; }
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
        <button className={"icon" + (menuAt ? " on" : "")} onClick={(e) => setMenuAt(menuAt ? null : e.currentTarget)}><More size={14} /></button>
        {armed && <button className="btn small danger armed" onClick={remove}>{t("Click again to remove")}</button>}
      </div>
      <Popover anchor={menuAt} open={!!menuAt} onClose={() => setMenuAt(null)} align="end">
        <Menu close={() => setMenuAt(null)} items={[
          ...(!current ? [{ label: t("Use"), onClick: use }] : []),
          ...(p.url ? [{ label: t("Copy URL"), onClick: () => { App.CopyText(p.url!); toast(t("Copied")); } }] : []),
          { label: t("Rename"), onClick: () => setRenaming(true) },
          { label: t("Open in editor"), onClick: () => P.OpenInEditor(p.id).catch(toastError) },
          { label: t("Show in Finder"), onClick: () => P.Reveal(p.id).catch(toastError) },
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

function ImportPopover({ anchor, onClose, onDone }: { anchor: HTMLElement | null; onClose: () => void; onDone: (p: Profile) => void }) {
  const t = useT();
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [interval, setInterval] = useState(24);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { if (anchor) setTimeout(() => input.current?.focus(), 60); }, [anchor]);

  const submit = async () => {
    if (!url.trim()) return;
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
    try {
      const p = await P.ImportFile();
      if (p?.id) { toast(t("Imported {name}", { name: p.name })); onDone(p); }
    } catch (e) { toastError(e); }
  };

  return (
    <Popover anchor={anchor} open={!!anchor} onClose={onClose} align="end" width={360}>
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
          <button type="button" className="btn" onClick={file}>{t("Import a file…")}</button>
          <div className="grow" />
          <button type="submit" className="btn primary" disabled={busy || !url.trim()}>{busy ? t("Updating…") : t("Import")}</button>
        </div>
      </form>
    </Popover>
  );
}
