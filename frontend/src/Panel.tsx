import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore } from "./store";
import { useT } from "./i18n";
import { App, Proxy, type ClientRate } from "./api";
import { usePoll } from "./usePoll";
import { Segmented } from "./components/Segmented";
import { Switch } from "./components/Switch";
import { Sparkline } from "./components/Sparkline";
import { Bolt, Chevron, Gear, Logo, Power, Refresh, Window } from "./components/Icons";
import { Fold } from "./components/Fold";
import { AppIcon } from "./components/AppIcon";
import { coreLabel, restartCore, setMode, setSystemProxy, setTun, startCore } from "./actions";
import { speed, delayClass } from "./format";
import { useGroups } from "./useGroups";
import { fmtDelay } from "./views/Proxies";

// The tray panel: the switches one reaches for most, the groups to pick a
// proxy in, and the way to the window. It grows and shrinks with what it
// shows, the window gliding to the page's height.
export function Panel() {
  const t = useT();
  const state = useStore((s) => s.state);
  const traffic = useStore((s) => s.traffic);
  const history = useStore((s) => s.history);
  const { groups, select, testGroup, testOne, testAll, testing, progress, flash } = useGroups();
  const [open, setOpen] = useState<string>("");
  const top = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const foot = useRef<HTMLDivElement>(null);
  const core = state?.core ?? "stopped";
  const running = core === "running";

  useFit([top, body, foot]);

  // the panel opens fresh: groups folded, from the top, and the logo's
  // light going round once
  const [opened, setOpened] = useState(0);
  useEffect(() => {
    const onVis = () => { if (document.hidden) setOpen(""); else setOpened((n) => n + 1); };
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, []);

  const shown = (groups ?? []).filter((g) => !g.hidden && g.name !== "GLOBAL");

  return (
    <div className="app panel">
      <div className="ptop" ref={top}>
        <span className={"plogo logo" + (opened ? " spin" : "")} key={opened}><Logo size={20} /></span>
        <div className="pstatus">
          <div className="pname">{running ? state?.profileName : coreLabel()}</div>
          <div className="pspeed num">{running ? <>↑ {speed(traffic.up)} · ↓ {speed(traffic.down)}</> : state?.coreError || " "}</div>
        </div>
        {running && shown.length > 0 && (
          <button className={"icon" + (progress ? " zap" : "")} disabled={!!testing["all/"]} title={progress || t("Test all")} onClick={testAll}><Bolt /></button>
        )}
        <button className="icon" title={t("Open Dashboard")} onClick={() => App.ShowMain("")}><Window /></button>
        <button className="icon" title={t("Settings")} onClick={() => App.ShowMain("settings")}><Gear /></button>
      </div>

      <div className="pscroll">
        <div className="pbody" ref={body}>
          <Segmented
            className="track fill"
            value={state?.mode ?? "rule"}
            onChange={setMode}
            options={[{ value: "rule", label: t("Rule") }, { value: "global", label: t("Global") }, { value: "direct", label: t("Direct") }]}
          />
          <div className="list">
            <div className="row">
              <div className="who"><div className="name">{t("System Proxy")}</div></div>
              <Switch on={!!state?.systemProxy} onChange={setSystemProxy} />
            </div>
            <div className="row">
              <div className="who"><div className="name">{t("Enhanced Mode")}</div>{!state?.serviceMode && <div className="sub">{t("Installs a privileged helper on first use")}</div>}</div>
              <Switch on={!!state?.tun} onChange={setTun} />
            </div>
          </div>

          {running ? (
            <>
              <div className="pchart">
                <Sparkline data={history} height={34} />
                <TopClients />
              </div>
              <div className="pgroups">
                {shown.map((g) => {
                  const isOpen = open === g.name;
                  const now = (g.members ?? []).find((m) => m.name === g.now);
                  return (
                    <div className={"pgroup" + (isOpen ? " open" : "")} key={g.name}>
                      <button className="pghead" onClick={() => setOpen(isOpen ? "" : g.name)}>
                        <Chevron className={"chev" + (isOpen ? " open" : "")} />
                        <span className="pgname">{g.name}</span>
                        <span className="pgnow">{g.now}</span>
                        {now && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay)}</span>}
                      </button>
                      <Fold open={isOpen}>
                        <div className="pnodes">
                          <div className="pnodes-bar">
                            <span>{g.members?.length ?? 0} · {g.type}</span>
                            <button className={"icon" + (testing[g.name] ? " zap" : "")} onClick={() => testGroup(g)}><Bolt size={13} /></button>
                          </div>
                          {(g.members ?? []).map((m, i) => (
                            <button
                              key={m.name}
                              className={"pnode stagger" + (m.name === g.now ? " on" : "") + (flash === g.name + "/" + m.name ? " flash" : "")}
                              style={{ ["--i" as string]: Math.min(i, 16) }}
                              onClick={(e) => (e.altKey ? testOne(m.name) : g.type === "Selector" && m.name !== g.now && select(g.name, m.name))}
                            >
                              <span className="check">{m.name === g.now ? "✓" : ""}</span>
                              <span className="nname">{m.name}</span>
                              <span className={"delay " + (testing["#" + m.name] ? "testing" : delayClass(m.delay))}>{testing["#" + m.name] ? "···" : fmtDelay(m.delay)}</span>
                            </button>
                          ))}
                        </div>
                      </Fold>
                    </div>
                  );
                })}
              </div>
            </>
          ) : (
            <div className="pempty">
              {state?.busy || core === "starting" ? <span className="spin-svg" style={{ display: "inline-flex" }}><Refresh /></span> : (
                <button className="btn primary" onClick={startCore}><Power size={14} />{t("Start core")}</button>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="pfoot" ref={foot}>
        <button className="link" onClick={() => App.ShowMain("profiles")}>{t("Profiles")}</button>
        <button className="link" onClick={() => App.ShowMain("connections")}>{t("Connections")}</button>
        <div className="grow" />
        {running && <button className="link" onClick={restartCore}>{t("Restart")}</button>}
        <button className="link" onClick={() => App.Quit()}>{t("Quit")}</button>
      </div>
    </div>
  );
}

// TopClients lists, under the traffic chart, the apps the traffic comes
// from, as Surge's menu does: each one's icon, name and speed now.
function TopClients() {
  const t = useT();
  const [clients, setClients] = useState<ClientRate[] | null>(null);
  usePoll(async () => {
    try { setClients((await Proxy.TopClients(3)) ?? []); } catch { setClients([]); }
  }, 1000);
  if (!clients) return null;
  return (
    <div className="pclients">
      <div className="pclients-head">{t("Top Clients")}</div>
      {clients.length === 0 ? (
        <div className="pclient none">{t("No active apps")}</div>
      ) : clients.map((c) => (
        <button className="pclient" key={c.path + "\0" + c.name} onClick={() => App.ShowMain("connections")}>
          <AppIcon path={c.path} />
          <span className="pcname">{c.name}</span>
          <span className="pcspeed num">{speed(c.up + c.down)}</span>
        </button>
      ))}
    </div>
  );
}

// useFit keeps the panel window as tall as the parts' natural height; a
// change while it shows glides there on the page's curve.
function useFit(parts: React.RefObject<HTMLElement | null>[]) {
  const last = useRef(0);
  useLayoutEffect(() => {
    let raf = 0;
    const measure = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const h = Math.ceil(parts.reduce((n, r) => n + (r.current?.getBoundingClientRect().height ?? 0), 0)) + 4;
        if (Math.abs(h - last.current) < 2) return;
        const first = last.current === 0;
        last.current = h;
        const still = first || document.hidden || matchMedia("(prefers-reduced-motion: reduce)").matches;
        App.FitPanel(h, still ? 0 : 380);
      });
    };
    const ro = new ResizeObserver(measure);
    parts.forEach((r) => r.current && ro.observe(r.current));
    measure();
    return () => { ro.disconnect(); cancelAnimationFrame(raf); };
  }, []);
}
