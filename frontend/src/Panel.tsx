import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore, type Sample } from "./store";
import { useT } from "./i18n";
import { isWindows } from "./platform";
import { App, Profiles, Proxy, Settings, type ClientRate } from "./api";
import { usePoll } from "./usePoll";
import { Segmented } from "./components/Segmented";
import { Popover, Menu, type MenuItem } from "./components/Popover";
import { Switch } from "./components/Switch";
import { Sparkline, clock } from "./components/Sparkline";
import { Arrow, Bolt, Chevron, Gear, Globe, Logo, Power, Refresh, Shield, Wifi, Window } from "./components/Icons";
import { Fold } from "./components/Fold";
import { AppIcon } from "./components/AppIcon";
import { coreLabel, coreTone, restartCore, setMode, setSystemProxy, setTun, startCore } from "./actions";
import { speed, delayClass, fmtDelay, nodeLabel } from "./format";
import { useGroups } from "./useGroups";
import { matchName } from "./components/NetworkRules";
import { toast, toastError } from "./components/Toast";

// The tray panel: the switches one reaches for most, the groups to pick a
// proxy in, and the way to the window. It grows and shrinks with what it
// shows, the window gliding to the page's height.
export function Panel() {
  const t = useT();
  const state = useStore((s) => s.state);
  const traffic = useStore((s) => s.traffic);
  const history = useStore((s) => s.history);
  const { groups, select, testGroup, testOne, testAll, testing, progress, flash } = useGroups({ testOnOpen: true });
  const [open, setOpen] = useState<string>("");
  // the sample under the pointer, shown in place of the speeds now
  const [scrub, setScrub] = useState<Sample | null>(null);
  const top = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const foot = useRef<HTMLDivElement>(null);
  const core = state?.core ?? "stopped";
  const running = core === "running";
  // the apps under the traffic, folded away until asked for; their icons
  // stand in for the list meanwhile
  const [apps, setApps] = useState(false);
  const [clients, setClients] = useState<ClientRate[] | null>(null);
  usePoll(async () => {
    if (!running) return setClients(null);
    try { setClients((await Proxy.TopClients(3)) ?? []); } catch { setClients([]); }
  }, 1000, [running]);
  // the profile's name opens the profiles to switch between
  const [profAt, setProfAt] = useState<HTMLButtonElement | null>(null);
  const [profOpen, setProfOpen] = useState(false);
  const profiles = useStore((s) => s.profiles);
  const current = state?.profile;
  const use = async (id: string, name: string) => {
    if (id === current) return;
    try { await Profiles.Use(id); toast(t("Switched to {name}", { name })); } catch (e) { toastError(e); }
  };
  const profileItems: MenuItem[] = [
    ...profiles.map((p) => ({ label: p.name, checked: p.id === current, onClick: () => use(p.id, p.name) })),
    ...(profiles.length ? ["sep" as const] : []),
    { label: t("Manage Profiles…"), onClick: () => App.ShowMain("profiles") },
  ];

  useFit([top, body, foot]);

  // the panel opens fresh: groups folded, from the top, and the logo's
  // light going round once
  const [opened, setOpened] = useState(0);
  useEffect(() => {
    const onVis = () => { if (document.hidden) { setOpen(""); setProfOpen(false); } else setOpened((n) => n + 1); };
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, []);

  const shown = (groups ?? []).filter((g) => !g.hidden && g.name !== "GLOBAL");

  return (
    <div className="app panel">
      <div className="ptop" ref={top}>
        <span className={"plogo logo" + (opened ? " spin" : "")} key={opened}><Logo size={20} /></span>
        <div className="pstatus">
          <button ref={setProfAt} className={"pname" + (profOpen ? " on" : "")} title={t("Switch profile")} onClick={() => setProfOpen((o) => !o)}>
            <span className="pname-text">{state?.profileName || t("ClashCube")}</span>
            <Chevron size={10} className="chev" />
          </button>
          <Popover anchor={profAt} open={profOpen} onClose={() => setProfOpen(false)} width={240}>
            <Menu close={() => setProfOpen(false)} items={profileItems} />
          </Popover>
          <div className="pspeed">
            <span className={"cdot " + coreTone()} />
            <span className="ptext">{running ? <>{coreLabel()} · <span className="num">127.0.0.1:{state?.mixedPort}</span></> : state?.coreError || coreLabel()}</span>
          </div>
        </div>
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
              <span className={"ic" + (state?.systemProxy ? " on" : "")}><Globe size={14} /></span>
              <div className="who">
                <div className="name">{t("System Proxy")}</div>
                {running && state?.proxyLost && <div className="sub warn">{t("Taken by another app")} · <button className="link" onClick={() => setSystemProxy(true)}>{t("Take it back")}</button></div>}
              </div>
              <Switch on={!!state?.systemProxy} onChange={setSystemProxy} />
            </div>
            <div className="row">
              <span className={"ic" + (state?.tun ? " on" : "")}><Shield size={14} /></span>
              <div className="who"><div className="name">{t("Enhanced Mode")}</div>{!state?.serviceMode && <div className="sub">{t(isWindows ? "Asks for administrator permission for this session" : "Installs a privileged helper on first use")}</div>}</div>
              <Switch on={!!state?.tun} onChange={setTun} />
            </div>
            {state?.network.match && (
              <div className="row">
                <span className="ic"><Wifi size={14} /></span>
                <div className="who">
                  <div className="name">{t("Network rule")}<span className="badge net">{matchName(state.network.match, t)}</span></div>
                  {(state.network.manual?.length ?? 0) > 0 && <div className="sub warn">{t("Changed by hand")} · <button className="link" onClick={() => Settings.ResumeNetworkAuto().catch(toastError)}>{t("Resume rule")}</button></div>}
                </div>
              </div>
            )}
          </div>

          {running ? (
            <>
              <div className="pchart">
                <div className={"prates num" + (scrub ? " scrub" : "")}>
                  <span className="rate up"><Arrow dir="up" size={11} />{speed((scrub ?? traffic).up)}</span>
                  <span className="rate down"><Arrow dir="down" size={11} />{speed((scrub ?? traffic).down)}</span>
                  {scrub && <span className="pat">{clock(scrub.at)}</span>}
                  <button className="papps" title={t("Top Clients")} aria-expanded={apps} onClick={() => setApps((a) => !a)}>
                    {!apps && clients?.map((c) => <AppIcon key={c.path + "\0" + c.name} path={c.path} />)}
                    <Chevron className={"chev" + (apps ? " open" : "")} />
                  </button>
                </div>
                <Sparkline data={history} height={34} tooltip={false} onHover={setScrub} />
                <Fold open={apps}><TopClients clients={clients} /></Fold>
              </div>
              {shown.length > 0 && (
                <div className="psect">
                  <span>{t("Proxy groups")}</span>
                  {progress && <span className="psect-note num">{progress}</span>}
                  <button className={"icon" + (progress ? " zap" : "")} disabled={!!testing["all/"]} title={t("Test all")} onClick={testAll}><Bolt size={13} /></button>
                </div>
              )}
              <div className="pgroups">
                {shown.map((g) => {
                  const isOpen = open === g.name;
                  const now = (g.members ?? []).find((m) => m.name === g.now);
                  return (
                    <div className={"pgroup" + (isOpen ? " open" : "")} key={g.name}>
                      <button className="pghead" onClick={() => setOpen(isOpen ? "" : g.name)}>
                        <Chevron className={"chev" + (isOpen ? " open" : "")} />
                        <span className="pgname">{g.name}</span>
                        <span className="pgnow">{nodeLabel(g.now)}</span>
                        {testing[g.name] || testing["#" + g.now] ? <span className="delay testing">···</span>
                          : <span className={"delay " + (now ? delayClass(now.delay) : "none")}>{now ? fmtDelay(now.delay, t) : "—"}</span>}
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
                              <span className="nname">{nodeLabel(m.name)}</span>
                              <span className={"delay " + (testing["#" + m.name] ? "testing" : delayClass(m.delay))}>{testing["#" + m.name] ? "···" : fmtDelay(m.delay, t)}</span>
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
function TopClients({ clients }: { clients: ClientRate[] | null }) {
  const t = useT();
  if (!clients) return null;
  return (
    <div className="pclients">
      {clients.length > 0 && <div className="pclients-head">{t("Top Clients")}</div>}
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
