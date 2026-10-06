import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react";
import { Proxy, type Globe, type GlobeRoute } from "../api";
import { useStore } from "../store";
import { locale, useT } from "../i18n";
import { usePoll } from "../usePoll";
import { bytes, flag, speed } from "../format";
import { startCore } from "../actions";
import { errText } from "../components/Toast";
import { Segmented } from "../components/Segmented";
import { attachEarth, routeKey, type Earth } from "../components/globe/earth";
import { Eye } from "../components/Icons";
import { OverviewTabs } from "./Usage";

type Filter = "all" | "proxy" | "direct";

// an address with its host part hidden, for a screen others may see
const mask = (ip: string) => ip.includes(":")
  ? ip.split(":").slice(0, 2).join(":") + ":****:****"
  : ip.split(".").slice(0, 2).join(".") + ".*.*";

// GlobeView is where the connections go now, on a globe: an arc from this
// Mac's country, through the nodes' when their names tell, to each country
// connections go to, with light running along those moving data. The list
// beside it names them, busiest first; pointing at one lights its arc.
export function GlobeView() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const lang = useStore((s) => s.settings?.lang);
  const [globe, setGlobe] = useState<Globe | null>(null);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [lit, setLit] = useState<string | null>(null);
  const [hover, setHover] = useState<{ cc: string; x: number; y: number } | null>(null);
  const [broken, setBroken] = useState("");
  const [showIP, setShowIP] = useState(false);
  const host = useRef<HTMLDivElement>(null);
  const earth = useRef<Earth | null>(null);

  const names = useMemo(() => new Intl.DisplayNames([locale()], { type: "region" }), [lang]);
  const name = (cc: string) => { try { return names.of(cc) ?? cc; } catch { return cc; } };
  const place = (cc: string) => `${flag(cc)} ${name(cc)}`;
  const placeRef = useRef(place);
  placeRef.current = place;

  usePoll(() => {
    if (!running) return;
    Proxy.Globe().then((g) => { setGlobe(g); setError(""); }).catch((e) => setError(errText(e)));
  }, 1000, [running]);

  const routes = useMemo(
    () => (globe?.routes ?? []).filter((r) => filter === "all" || (filter === "direct") === r.direct),
    [globe, filter],
  );
  const listed = useSteady(routes);
  const list = useRef<HTMLDivElement>(null);
  useFlip(list, listed.map(routeKey).join(","));

  useEffect(() => {
    if (!running || !host.current) return;
    let gone = false;
    attachEarth(host.current, {
      onHover: (cc, x, y) => setHover(cc ? { cc, x, y } : null),
      label: (cc) => placeRef.current(cc),
    }).then((e) => { if (gone) e.detach(); else earth.current = e; setBroken(""); })
      .catch((e) => !gone && setBroken(errText(e)));
    return () => { gone = true; earth.current?.detach(); earth.current = null; };
  }, [running]);

  useEffect(() => { if (globe) earth.current?.setData(globe, routes); }, [globe, routes, lang]);
  useEffect(() => { earth.current?.setHighlight(lit); }, [lit]);

  const countries = new Set(routes.map((r) => r.to)).size;
  const conns = routes.reduce((n, r) => n + r.conns, 0);
  const up = routes.reduce((n, r) => n + r.up, 0), down = routes.reduce((n, r) => n + r.down, 0);

  return (
    <div className="view globe-view">
      <div className="view-head">
        <OverviewTabs value="globe" />
        {running && (
          <div className="view-tools">
            <Segmented className="track small" value={filter} onChange={setFilter} options={[
              { value: "all", label: t("All") }, { value: "proxy", label: t("Proxy") }, { value: "direct", label: t("Direct") },
            ]} />
          </div>
        )}
      </div>

      {!running ? (
        <div className="card empty-state">
          <b>{t("Global connections")}</b>
          {t("Start the core to see where connections go.")}
          <div><button className="btn small primary" onClick={startCore}>{t("Start core")}</button></div>
        </div>
      ) : (
        <div className="globe-body">
          <div className="card globe-card">
            <div className="globe-host" ref={host} />
            <div className="globe-stats">
              <div className="globe-origin">
                {globe?.origin ? t("From {place}", { place: place(globe.origin) }) : t("Locating…")}
                {globe?.originIp && (
                  <span className="globe-ip">
                    <span className="mono">{showIP ? globe.originIp : mask(globe.originIp)}</span>
                    <button className="icon" title={t(showIP ? "Hide address" : "Show address")} onClick={() => setShowIP(!showIP)}>
                      <Eye size={13} off={!showIP} />
                    </button>
                  </span>
                )}
              </div>
              <div className="globe-figs num">
                <span>{t(countries === 1 ? "{n} country" : "{n} countries", { n: countries })}</span>
                <span>{t(conns === 1 ? "{n} connection" : "{n} connections", { n: conns })}</span>
                <span className="ud up"><b>↑</b> {speed(up)}</span>
                <span className="ud down"><b>↓</b> {speed(down)}</span>
              </div>
            </div>
            <div className="globe-legend">
              <span><i className="proxy" />{t("Proxy")}</span>
              <span><i className="direct" />{t("Direct")}</span>
              <span className="hint">{t("Drag to turn, scroll to zoom")}</span>
            </div>
            {(broken || error || (globe && !routes.length)) && (
              <div className="globe-note">{broken ? `${t("Couldn't draw the globe")}: ${broken}` : error || t("No connections with a known location yet")}</div>
            )}
          </div>

          <div className="list globe-list" ref={list}>
            {listed.map((r) => (
              <RouteRow key={routeKey(r)} r={r} lit={lit === routeKey(r)} name={name} place={place}
                onEnter={() => setLit(routeKey(r))} onLeave={() => setLit(null)} onClick={() => earth.current?.focus(r.to)} />
            ))}
            {!routes.length && <div className="globe-list-empty">{t("No connections")}</div>}
          </div>
        </div>
      )}

      {hover && globe && <Tip cc={hover.cc} x={hover.x} y={hover.y} routes={routes} origin={globe.origin} place={place} />}
    </div>
  );
}

// useSteady keeps the list's order until a route gets clearly busier than
// the one above it, so rows don't trade places at every poll.
function useSteady(routes: GlobeRoute[]) {
  const order = useRef<string[]>([]);
  return useMemo(() => {
    const by = new Map(routes.map((r) => [routeKey(r), r]));
    const keys = order.current.filter((k) => by.has(k));
    // the backend's order is busiest first: newcomers start from it
    for (const k of by.keys()) if (!keys.includes(k)) keys.push(k);
    const speed = (k: string) => { const r = by.get(k)!; return r.up + r.down; };
    for (let i = 1; i < keys.length; i++) {
      for (let j = i; j > 0 && speed(keys[j]) > speed(keys[j - 1]) * 1.3 + 1024; j--) {
        [keys[j - 1], keys[j]] = [keys[j], keys[j - 1]];
      }
    }
    order.current = keys;
    return keys.map((k) => by.get(k)!);
  }, [routes]);
}

// useFlip slides a list's rows (children with data-key) from where they
// were to where they are when their order changes, and fades new ones in.
// Transforms leave the layout alone, so the list never overflows midway.
function useFlip(box: RefObject<HTMLElement>, order: string) {
  const tops = useRef(new Map<string, number>());
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const easing = getComputedStyle(el).getPropertyValue("--ease-out").trim() || "ease-out";
    const first = tops.current.size === 0;
    const next = new Map<string, number>();
    el.querySelectorAll<HTMLElement>(":scope > [data-key]").forEach((row, i) => {
      const key = row.dataset.key!, top = row.offsetTop, was = tops.current.get(key);
      next.set(key, top);
      if (still) return;
      if (was === undefined) row.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 280, easing, delay: first ? Math.min(i, 10) * 25 : 0, fill: "backwards" });
      else if (was !== top) row.animate([{ transform: `translateY(${was - top}px)` }, { transform: "none" }], { duration: 420, easing });
    });
    tops.current = next;
  }, [order]);
}

function RouteRow({ r, lit, name, place, onEnter, onLeave, onClick }: {
  r: GlobeRoute; lit: boolean; name: (cc: string) => string; place: (cc: string) => string;
  onEnter: () => void; onLeave: () => void; onClick: () => void;
}) {
  const t = useT();
  const host = r.hosts?.[0]?.host;
  return (
    <button className={"row click" + (lit ? " on" : "")} data-key={routeKey(r)}
      onMouseEnter={onEnter} onMouseLeave={onLeave} onClick={onClick}>
      <span className="ic flag">{flag(r.to)}</span>
      <div className="who">
        <div className="name">
          {name(r.to)}
          {r.conns > 1 && <span className="globe-conns num" title={t("{n} connections", { n: r.conns })}>{r.conns}</span>}
        </div>
        <div className="sub">
          <i className={"way " + (r.direct ? "direct" : "proxy")} />
          {r.direct ? t("Direct") : r.via ? t("via {place}", { place: place(r.via) }) : t("Proxy")}
          {host && <> · {host}{(r.hosts?.length ?? 0) > 1 ? ` +${(r.hosts?.length ?? 1) - 1}` : ""}</>}
        </div>
      </div>
      <div className="end globe-rate num">
        <span className={r.up + r.down ? "" : "idle"}>{speed(r.up + r.down)}</span>
        <small>{bytes(r.total)}</small>
      </div>
    </button>
  );
}

// Tip is a country's routes and busiest hosts, by the pointer.
function Tip({ cc, x, y, routes, origin, place }: {
  cc: string; x: number; y: number; routes: GlobeRoute[]; origin: string; place: (cc: string) => string;
}) {
  const t = useT();
  const here = routes.filter((r) => r.to === cc);
  const through = routes.filter((r) => r.via === cc && r.to !== cc);
  const hosts = here.flatMap((r) => r.hosts ?? []).sort((a, b) => b.total - a.total).slice(0, 5);
  const left = Math.min(x + 14, innerWidth - 250);
  return (
    <div className="globe-tip" style={{ left, top: y + 14 }}>
      <div className="globe-tip-head">{place(cc)}{cc === origin && <span className="chip">{t("You")}</span>}</div>
      {here.map((r) => (
        <div className="globe-tip-row" key={routeKey(r)}>
          <span><i className={"way " + (r.direct ? "direct" : "proxy")} />{r.direct ? t("Direct") : r.via ? t("via {place}", { place: place(r.via) }) : t("Proxy")}</span>
          <span className="num">{t(r.conns === 1 ? "{n} connection" : "{n} connections", { n: r.conns })} · {speed(r.up + r.down)}</span>
        </div>
      ))}
      {through.length > 0 && (
        <div className="globe-tip-row">
          <span>{t("Relays to {n} countries", { n: new Set(through.map((r) => r.to)).size })}</span>
          <span className="num">{t("{n} connections", { n: through.reduce((n, r) => n + r.conns, 0) })}</span>
        </div>
      )}
      {hosts.length > 0 && (
        <div className="globe-tip-hosts">
          {hosts.map((h) => <div key={h.host}><span>{h.host}</span><span className="num">{bytes(h.total)}</span></div>)}
        </div>
      )}
    </div>
  );
}
