import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { Search, More } from "../components/Icons";
import { Menu, Popover, type MenuItem } from "../components/Popover";

// Pages own their actions; the shell owns their placement and history.
export function PageActions({ query, onQuery, children, items, count }: { query?: string; onQuery?: (q: string) => void; children?: ReactNode; items?: MenuItem[]; count?: number }) {
  const t = useT();
  const [search, setSearch] = useState(false);
  const [menu, setMenu] = useState(false);
  useEffect(() => {
    const back = () => setSearch(!!history.state?.search);
    window.addEventListener("popstate", back);
    return () => window.removeEventListener("popstate", back);
  }, []);
  useEffect(() => { document.querySelector(".mobile-bar")?.classList.toggle("searching", search); return () => document.querySelector(".mobile-bar")?.classList.remove("searching"); }, [search]);
  const slot = document.getElementById("mobile-actions");
  const countSlot = document.getElementById("mobile-page-count");
  if (!slot) return null;
  return <>{countSlot && count !== undefined && !search && createPortal(count, countSlot)}{createPortal(<>
    {onQuery && (search ? <label className="mobile-search"><input autoFocus value={query} placeholder={t("Search")} onChange={e => onQuery(e.target.value)} /><button className="icon" aria-label={t("Clear")} onClick={() => query ? onQuery("") : history.back()}>×</button></label> : <button className="icon" aria-label={t("Search")} onClick={() => { history.pushState({ ...history.state, search: true }, ""); setSearch(true); }}><Search size={22} /></button>)}
    {children}
    {items && <button className="icon" aria-label={t("More")} onClick={() => setMenu(true)}><More size={22} /></button>}
    <Popover anchor={null} open={menu} onClose={() => setMenu(false)}><Menu items={items ?? []} close={() => setMenu(false)} /></Popover>
  </>, slot)}</>;
}
