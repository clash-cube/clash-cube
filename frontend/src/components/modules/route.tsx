import { useEffect, useState } from "react";
import { useT } from "../../i18n";
import { Profiles as P } from "../../api";
import type { Module, Route, Service } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { RegionNodes } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import { useStore } from "../../store";
import { delayClass, fmtDelay } from "../../format";
import { Segmented } from "../Segmented";
import { Fold } from "../Fold";
import { Close, Search } from "../Icons";
import { toastError } from "../Toast";
import { asYAML, FormOrYAML, NodePicker, NodeSelect, refusing, useNodes, type FormProps } from "./shared";
import type { ModuleKind, RowCtx } from "./kinds";

type T = ReturnType<typeof useT>;

const POLICIES = ["select", "url-test", "DIRECT"] as const;
const policyLabel = (p: string, t: T) => (p === "url-test" ? t("Fastest") : p === "DIRECT" ? t("Direct") : t("Choose a node"));

type Scope = "all" | "region" | "pick";
const scopeOf = (r: Route): Scope => (r.region ? "region" : r.pick ? "pick" : "all");
// the nodes picked for a profile; picks are a profile's, keywords all's
const picksOf = (r: Route, profile: string) => r.nodes?.[profile] ?? [];

// "Choose a node · Hong Kong": how the route goes, for its row
function routeSummary(r: Route, profile: string, regions: RegionNodes[], t: T) {
  const reg = regions.find((x) => x.key === r.region);
  const n = picksOf(r, profile).length + (r.keywords?.length ?? 0);
  const scope = reg ? t(reg.name) : r.pick ? (n ? t("{n} picked", { n }) : t("None picked here")) : t("All nodes");
  return [policyLabel(r.policy, t), r.policy !== "DIRECT" && scope,
    r.policy !== "DIRECT" && r.upstream?.[profile] && t("Via {node}", { node: r.upstream[profile] })].filter(Boolean).join(" · ");
}

// takes is Route.Takes in Go, for the nodes the editor lists
function takes(picks: string[], keywords: string[], name: string) {
  const low = name.toLowerCase();
  return picks.includes(name) || keywords.some((k) => low.includes(k.toLowerCase()));
}

// useRegions is the regions a route's group can take, each with how many
// of the running profile's nodes it would.
function useRegions(want = true) {
  const [regions, setRegions] = useState<RegionNodes[]>([]);
  useEffect(() => { if (want) P.RouteRegions().then((r) => setRegions(r ?? [])).catch(() => {}); }, [want]);
  return regions;
}

// RouteEditor writes a route as choices, not YAML: how the service goes,
// and through which nodes: all, a region's, or ones picked by name and by
// keyword. The YAML it makes can be taken to edit freely.
// RouteForm is a route editor's state, kept while its YAML is edited
type RouteForm = { name: string; route: Route; scope: Scope };

function RouteEditor({ module, form, onCancel, onSave, onYAML }: FormProps<RouteForm>) {
  const t = useT();
  const [name, setName] = useState(form?.name ?? module.name);
  const [r, setR] = useState<Route>(form?.route ?? module.route!);
  const [scope, setScope] = useState<Scope>(form?.scope ?? scopeOf(module.route!));
  const [busy, setBusy] = useState(false);
  const [query, setQuery] = useState("");
  const [keyword, setKeyword] = useState("");
  const regions = useRegions();
  const nodes = useNodes();
  // the nodes are picked for the profile in use; other profiles' stay
  const profile = useStore((s) => s.state?.profile ?? "");
  const picks = picksOf(r, profile);
  const upstream = r.upstream?.[profile] ?? "";
  const keywords = r.keywords ?? [];
  const setPicks = (next: string[]) => setR({ ...r, nodes: { ...r.nodes, [profile]: next } });
  const setUpstream = (name: string) => {
    const next = { ...r.upstream };
    if (name) next[profile] = name; else delete next[profile];
    setR({ ...r, upstream: next });
  };
  // what is saved: only the chosen scope's part
  const pick = scope === "pick";
  const out: Route = {
    service: r.service, policy: r.policy,
    region: scope === "region" ? r.region || regions[0]?.key || "" : "",
    pick,
    nodes: pick ? Object.fromEntries(Object.entries(r.nodes ?? {}).filter(([, v]) => v?.length)) : {},
    keywords: pick ? keywords : [],
    upstream: r.upstream ?? {},
  };
  const was = module.route!;
  const dirty = name.trim() !== module.name || JSON.stringify(out) !== JSON.stringify({
    service: was.service, policy: was.policy, region: was.region ?? "", pick: !!was.pick, nodes: was.nodes ?? {}, keywords: was.keywords ?? [], upstream: was.upstream ?? {},
  });
  const empty = r.policy !== "DIRECT" && pick && !picks.length && !keywords.length;
  const submit = async () => {
    if (busy || !name.trim() || empty) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), route: out }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  const toYAML = async () => {
    try { onYAML(await asYAML({ ...module, name: name.trim() || module.name, route: out }), { name, route: r, scope }); } catch (e) { toastError(e); }
  };
  // picking starts from none: most routes want one node or a few, and
  // the search with "Tick shown" takes a whole region when wanted
  const pickScope = setScope;
  const toggle = (n: string) => setPicks(picks.includes(n) ? picks.filter((x) => x !== n) : [...picks, n]);
  const addKeyword = () => {
    const k = keyword.trim();
    if (k && !keywords.includes(k)) setR({ ...r, keywords: [...keywords, k] });
    setKeyword("");
  };
  const reg = regions.find((x) => x.key === out.region);
  const names = new Set(nodes?.map((n) => n.name));
  // picked for this profile, but gone from it since (renamed or dropped)
  const gone = picks.filter((n) => nodes && !names.has(n));
  const missing = !!upstream && !!nodes && !names.has(upstream);
  const exits = nodes?.filter((n) => n.name !== upstream);
  const count = exits ? exits.filter((n) => scope === "region" ? n.region === reg?.key : !pick || takes(picks, keywords, n.name)).length : -1;
  const shown = (nodes ?? []).filter((n) => !query || n.name.toLowerCase().includes(query.toLowerCase()));
  const tickable = shown.map((n) => n.name).filter((n) => n !== upstream && !picks.includes(n));
  return (
    <form className="module-editor route-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && e.metaKey) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input className="input" placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
      <div className="field">
        <span className="label">{t("Goes")}</span>
        <Segmented className="track small" value={r.policy} onChange={(policy) => setR({ ...r, policy })}
          options={POLICIES.map((p) => ({ value: p, label: policyLabel(p, t) }))} />
      </div>
      <Fold open={r.policy !== "DIRECT"}>
        <div className="field">
          <span className="label">{t("Through")}</span>
          <div className="scope">
            <Segmented className="track small" value={scope} onChange={pickScope} options={[
              { value: "all", label: t("All nodes") }, { value: "region", label: t("A region") }, { value: "pick", label: t("Picked nodes") },
            ]} />
            <Fold open={scope === "region"}>
              <div className="chips">
                {regions.map((x) => (
                  <button type="button" key={x.key} className={"chip" + (out.region === x.key ? " on" : "") + (x.count === 0 ? " none" : "")}
                    onClick={() => setR({ ...r, region: x.key })}>
                    {t(x.name)}{x.count >= 0 && <span className="count">{x.count}</span>}
                  </button>
                ))}
              </div>
            </Fold>
            <Fold open={scope === "pick"}>
              <div className="pick">
                {nodes === null ? <div className="pick-note">{t("Start the core to list the profile's nodes. Keywords work without it.")}</div> : (
                  <>
                    <div className="pick-note">{t("Ticked for this profile only; each profile has its own. Keywords below hold for all.")}</div>
                    <div className="pick-bar">
                      <label className="search"><Search size={13} /><input placeholder={t("Search nodes")} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") e.preventDefault(); }} /></label>
                      <span className="pick-count">{t("{n} picked", { n: picks.length })}</span>
                      <button type="button" className="link" disabled={!tickable.length} onClick={() => setPicks([...picks, ...tickable])}>{query ? t("Tick shown") : t("Tick all")}</button>
                      <button type="button" className="link" disabled={!picks.length} onClick={() => setPicks([])}>{t("Clear")}</button>
                    </div>
                    <div className="pick-list">
                      {shown.map((n) => {
                        const isUpstream = n.name === upstream;
                        const on = !isUpstream && picks.includes(n.name);
                        const byKeyword = !isUpstream && !on && takes([], keywords, n.name);
                        return (
                          <label key={n.name} className={"pick-row" + (on || byKeyword ? " on" : "")}>
                            <input type="checkbox" checked={on || byKeyword} disabled={byKeyword || isUpstream} onChange={() => toggle(n.name)} />
                            <span className="nname">{n.name}</span>
                            {byKeyword && <span className="badge muted">{t("By keyword")}</span>}
                            {isUpstream && <span className="badge muted">{t("Upstream")}</span>}
                            <span className={"delay " + delayClass(n.delay)}>{fmtDelay(n.delay, t)}</span>
                          </label>
                        );
                      })}
                      {gone.map((n) => (
                        <label key={n} className="pick-row gone" title={t("The profile no longer has this node")}>
                          <input type="checkbox" checked onChange={() => toggle(n)} />
                          <span className="nname">{n}</span>
                          <span className="badge muted">{t("Gone")}</span>
                        </label>
                      ))}
                    </div>
                  </>
                )}
                <div className="keywords">
                  <span className="klabel">{t("Or names containing")}</span>
                  {keywords.map((k) => (
                    <span key={k} className="kw">{k}<button type="button" title={t("Delete")} onClick={() => setR({ ...r, keywords: keywords.filter((x) => x !== k) })}><Close size={9} /></button></span>
                  ))}
                  <input className="kw-input" placeholder={t("IPLC, 专线 …")} value={keyword} onChange={(e) => setKeyword(e.target.value)}
                    onBlur={addKeyword} onKeyDown={(e) => { if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addKeyword(); } else if (e.key === "Backspace" && !keyword && keywords.length) setR({ ...r, keywords: keywords.slice(0, -1) }); }} />
                </div>
              </div>
            </Fold>
          </div>
        </div>
        <div className="field">
          <span className="label">{t("Via")}</span>
          <div className="scope">
            <NodeSelect value={upstream} nodes={nodes} none={t("No upstream")} placeholder={t("None")} title={t("Exits dial through this node, for this profile only")} onChange={setUpstream} />
            {upstream && <div className={"route-path" + (missing ? " err" : "")}>
              {missing ? t("The upstream node is gone, so {service} is refused until it returns or you choose another.", { service: r.service })
                : <>{t("This Mac")}<span className="sep">→</span><b>{upstream}</b><span className="sep">→</span>{t("exit node")}<span className="sep">→</span>{r.service}</>}
            </div>}
          </div>
        </div>
      </Fold>
      <div className="foot">
        <span className={"hint" + ((count === 0 || empty) && r.policy !== "DIRECT" ? " err" : "")}>
          {r.policy === "DIRECT" ? t("{service} skips the proxy.", { service: r.service })
            : empty ? t("Pick nodes for this profile, or add a keyword. Until then {service} is refused.", { service: r.service })
            : count === 0 ? t("None of these nodes is in the profile in use, so {service} is refused rather than going direct.", { service: r.service })
            : count > 0 && scope !== "all" ? t("{n} nodes of the profile in use. With none of them left, {service} is refused rather than going direct.", { n: count, service: r.service })
            : r.policy === "select" ? t("A {service} group of its own; choose its node on this row, or on the Proxies page.", { service: r.service })
            : t("A {service} group of its own, on the node with the lowest latency.", { service: r.service })}
        </span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={toYAML}>{t("Edit as YAML")}</button>
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || !name.trim() || empty || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}

function Summary({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const regions = useRegions();
  const r = m.route!;
  // a front that left the profile refuses the route, as an empty scope does
  const front = r.policy !== "DIRECT" ? r.upstream?.[ctx.profile] ?? "" : "";
  const frontGone = !!front && !!ctx.nodes && !ctx.nodes.some((n) => n.name === front);
  return frontGone
    ? <div className="sub warn">{t("Upstream {node} is gone; connections are refused", { node: front })}</div>
    : <div className="sub">{routeSummary(r, ctx.profile, regions, t)}</div>;
}

function End({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const { group } = ctx;
  if (!group) return null;
  return refusing(group)
    ? <button className="node-pick warn" title={t("Its connections are refused until it has a node in the profile in use.")} onClick={() => !ctx.open && ctx.onOpen()}>
        {m.route?.pick && !picksOf(m.route, ctx.profile).length ? t("Pick nodes for this profile") : t("No node")}
      </button>
    : <NodePicker group={group} onPick={(n) => ctx.onPick(group, n)} />;
}

// A route sends a service through a group of its own; its row picks the
// group's node.
export const routeKind: ModuleKind = {
  id: "route",
  is: (m) => !!m.route,
  Summary,
  End,
  Editor: (props) => <FormOrYAML<RouteForm> {...props} Form={RouteEditor} />,
  useNew: (mods) => {
    const t = useT();
    const [services, setServices] = useState<Service[]>([]);
    useEffect(() => { P.RouteServices().then((ss) => setServices(ss ?? [])).catch(() => {}); }, []);
    const routed = new Set(mods.flatMap((m) => (m.route ? [m.route.service] : [])));
    return [{
      title: t("Route a service"),
      entries: services.map((svc) => ({
        key: "route:" + svc.name, name: t(svc.name), hint: t(svc.hint), added: routed.has(svc.name),
        make: (profile) => ({ id: "", name: t(svc.name), enabled: true, body: "", profile, route: { service: svc.name, policy: "select", region: svc.region ?? "" } }),
      })),
    }];
  },
  // its group is named after the service, which two can't share
  duplicate: false,
};
