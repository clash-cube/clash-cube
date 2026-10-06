import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { Line2 } from "three/addons/lines/Line2.js";
import { LineGeometry } from "three/addons/lines/LineGeometry.js";
import { LineMaterial } from "three/addons/lines/LineMaterial.js";
import type { Globe, GlobeRoute, Place } from "../../api";
import dayURL from "../../assets/globe/earth-day.webp";
import nightURL from "../../assets/globe/earth-night.webp";

// A textured Earth lit by the real sun, the connections drawn as arcs from
// this Mac's country (through the nodes' when known) to where they go, and
// light running along the arcs that move data, toward the side receiving.

export const routeKey = (r: GlobeRoute) => `${r.to}|${r.via}|${r.direct ? "direct" : "proxy"}`;

export type Earth = {
  setData: (g: Globe, routes: GlobeRoute[]) => void;
  setHighlight: (key: string | null) => void;
  focus: (cc: string) => void;
  dispose: () => void;
};

type Opts = {
  // a country under the pointer, with where to show its details; null when none
  onHover: (cc: string | null, x: number, y: number) => void;
  label: (cc: string) => string;
};

const SEGMENTS = 48;
const MARK_R = 1.006;
const DISTANCE = 4.6;
const RESUME_MS = 6000;
// a pulse's tail: its points, and its length on the globe (radius 1)
const TRAIL = 16;
const TAIL_LEN = 0.32;
const MAX_PULSES = 6;

const vec = (p: Place, r = 1, out = new THREE.Vector3()) => {
  const la = THREE.MathUtils.degToRad(p.lat), lo = THREE.MathUtils.degToRad(p.lon);
  return out.set(r * Math.cos(la) * Math.cos(lo), r * Math.sin(la), -r * Math.cos(la) * Math.sin(lo));
};

// the arc between two places, raised with their distance
function arc(a: Place, b: Place): THREE.Vector3[] {
  const s = vec(a), e = vec(b);
  const angle = s.angleTo(e);
  const lift = 0.04 + Math.min(0.3, angle * 0.12);
  const q = new THREE.Quaternion().setFromUnitVectors(s, e), id = new THREE.Quaternion(), step = new THREE.Quaternion();
  const out: THREE.Vector3[] = [];
  for (let i = 0; i <= SEGMENTS; i++) {
    const t = i / SEGMENTS;
    step.slerpQuaternions(id, q, t);
    out.push(s.clone().applyQuaternion(step).multiplyScalar(MARK_R + Math.sin(Math.PI * t) * lift));
  }
  return out;
}

// where the sun is overhead now, for the day and night sides
function sun(now = new Date()) {
  const start = Date.UTC(now.getUTCFullYear(), 0, 1);
  const day = (now.getTime() - start) / 86400000;
  const lat = -23.44 * Math.cos((2 * Math.PI / 365) * (day + 10));
  const hours = now.getUTCHours() + now.getUTCMinutes() / 60;
  return vec({ lat, lon: (12 - hours) * 15 }).normalize();
}

function cssColor(el: Element, name: string, fallback: string) {
  const v = getComputedStyle(el).getPropertyValue(name).trim();
  return new THREE.Color(v || fallback);
}

const earthVert = /* glsl */ `
varying vec2 vUv; varying vec3 vNormal; varying vec3 vView;
void main() {
  vUv = uv;
  vNormal = normalize(mat3(modelMatrix) * normal);
  vec4 wp = modelMatrix * vec4(position, 1.0);
  vView = normalize(cameraPosition - wp.xyz);
  gl_Position = projectionMatrix * viewMatrix * wp;
}`;

const earthFrag = /* glsl */ `
uniform sampler2D day; uniform sampler2D night; uniform vec3 sun; uniform vec3 atmo;
varying vec2 vUv; varying vec3 vNormal; varying vec3 vView;
void main() {
  float d = dot(vNormal, sun);
  vec3 dayC = texture2D(day, vUv).rgb;
  vec3 lit = dayC * (0.28 + 0.9 * max(d, 0.0));
  vec3 dark = texture2D(night, vUv).rgb * 1.3 + dayC * 0.07;
  vec3 c = mix(dark, lit, smoothstep(-0.18, 0.28, d));
  float rim = pow(1.0 - max(dot(vNormal, vView), 0.0), 3.0);
  c += atmo * rim * (0.2 + 0.8 * smoothstep(-0.3, 0.6, d));
  gl_FragColor = vec4(c, 1.0);
  #include <colorspace_fragment>
}`;

const haloVert = /* glsl */ `
varying vec3 vNormal;
void main() { vNormal = normalize(normalMatrix * normal); gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }`;

const haloFrag = /* glsl */ `
uniform vec3 atmo; varying vec3 vNormal;
void main() {
  float a = pow(clamp(0.72 - dot(vNormal, vec3(0.0, 0.0, 1.0)), 0.0, 1.0), 3.0);
  gl_FragColor = vec4(atmo, a * 0.9);
  #include <colorspace_fragment>
}`;

// round, soft points with their own size, colour and opacity
const dotVert = /* glsl */ `
attribute float size; attribute float alpha; attribute vec3 tint;
varying float vAlpha; varying vec3 vTint;
uniform float scale;
void main() {
  vAlpha = alpha; vTint = tint;
  vec4 mv = modelViewMatrix * vec4(position, 1.0);
  gl_PointSize = size * scale / -mv.z;
  gl_Position = projectionMatrix * mv;
}`;

const dotFrag = /* glsl */ `
varying float vAlpha; varying vec3 vTint;
void main() {
  float r = length(gl_PointCoord - 0.5) * 2.0;
  float a = smoothstep(1.0, 0.55, r) * vAlpha;
  if (a < 0.01) discard;
  gl_FragColor = vec4(vTint, a);
  #include <colorspace_fragment>
}`;

const ringFrag = /* glsl */ `
varying float vAlpha; varying vec3 vTint;
void main() {
  float r = length(gl_PointCoord - 0.5) * 2.0;
  float a = smoothstep(0.08, 0.0, abs(r - 0.85)) * vAlpha;
  if (a < 0.01) discard;
  gl_FragColor = vec4(vTint, a);
  #include <colorspace_fragment>
}`;

// Dots is a fixed number of points rewritten every frame.
class Dots {
  readonly points: THREE.Points;
  private pos: Float32Array; private size: Float32Array; private alpha: Float32Array; private tint: Float32Array;
  private geo = new THREE.BufferGeometry();
  private mat: THREE.ShaderMaterial;
  n = 0;
  constructor(private cap: number, frag: string, order: number) {
    this.pos = new Float32Array(cap * 3); this.size = new Float32Array(cap); this.alpha = new Float32Array(cap); this.tint = new Float32Array(cap * 3);
    this.geo.setAttribute("position", new THREE.BufferAttribute(this.pos, 3).setUsage(THREE.DynamicDrawUsage));
    this.geo.setAttribute("size", new THREE.BufferAttribute(this.size, 1).setUsage(THREE.DynamicDrawUsage));
    this.geo.setAttribute("alpha", new THREE.BufferAttribute(this.alpha, 1).setUsage(THREE.DynamicDrawUsage));
    this.geo.setAttribute("tint", new THREE.BufferAttribute(this.tint, 3).setUsage(THREE.DynamicDrawUsage));
    this.mat = new THREE.ShaderMaterial({ vertexShader: dotVert, fragmentShader: frag, uniforms: { scale: { value: 1 } }, transparent: true, depthWrite: false });
    this.points = new THREE.Points(this.geo, this.mat);
    this.points.frustumCulled = false;
    this.points.renderOrder = order;
  }
  setScale(s: number) { this.mat.uniforms.scale.value = s; }
  clear() { this.n = 0; }
  push(p: THREE.Vector3, size: number, alpha: number, c: THREE.Color) {
    if (this.n >= this.cap) return;
    const i = this.n++;
    this.pos.set([p.x, p.y, p.z], i * 3); this.size[i] = size; this.alpha[i] = alpha; this.tint.set([c.r, c.g, c.b], i * 3);
  }
  flush() {
    for (const k of ["position", "size", "alpha", "tint"]) (this.geo.getAttribute(k) as THREE.BufferAttribute).needsUpdate = true;
    this.geo.setDrawRange(0, this.n);
  }
  dispose() { this.geo.dispose(); this.mat.dispose(); }
}

// A pulse runs once along its route, from where it was sent to where the
// data goes; it fades in as it leaves and its tail drains into the end.
type Pulse = { t: number; back: boolean };
// A route's arc keeps its pulses while the data changes; one that goes away
// fades out, its pulses still in flight finishing their trip.
type Drawn = {
  route: GlobeRoute; key: string; origin: string; line: Line2; mat: LineMaterial; path: THREE.Vector3[];
  trip: number; tail: number; pulses: Pulse[]; due: number; rate: number; fade: number; gone: boolean;
};

export async function createEarth(host: HTMLElement, opts: Opts): Promise<Earth> {
  const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
  renderer.setClearColor(0x000000, 0);
  renderer.domElement.className = "globe-canvas";
  host.appendChild(renderer.domElement);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(32, 1, 0.1, 50);
  camera.position.set(0, 0.6, DISTANCE);
  const controls = new OrbitControls(camera, renderer.domElement);
  controls.enablePan = false;
  controls.enableDamping = true;
  controls.rotateSpeed = 0.5;
  controls.zoomSpeed = 0.6;
  controls.minDistance = 2.4;
  controls.maxDistance = 8;
  const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  controls.autoRotate = !still;
  controls.autoRotateSpeed = 0.2;

  const loader = new THREE.TextureLoader();
  const [day, night] = await Promise.all([loader.loadAsync(dayURL), loader.loadAsync(nightURL)]);
  for (const t of [day, night]) { t.colorSpace = THREE.SRGBColorSpace; t.anisotropy = renderer.capabilities.getMaxAnisotropy(); }

  let colors = { proxy: new THREE.Color(), direct: new THREE.Color(), atmo: new THREE.Color(), white: new THREE.Color(1, 1, 1) };
  const readColors = () => {
    colors = { ...colors, proxy: cssColor(host, "--up", "#0ea5e9"), direct: cssColor(host, "--amber", "#b45309"), atmo: cssColor(host, "--up", "#0ea5e9") };
  };
  readColors();

  const sphere = new THREE.SphereGeometry(1, 96, 96);
  const earthMat = new THREE.ShaderMaterial({
    vertexShader: earthVert, fragmentShader: earthFrag,
    uniforms: { day: { value: day }, night: { value: night }, sun: { value: sun() }, atmo: { value: colors.atmo } },
  });
  const earth = new THREE.Mesh(sphere, earthMat);
  scene.add(earth);
  const haloMat = new THREE.ShaderMaterial({
    vertexShader: haloVert, fragmentShader: haloFrag, uniforms: { atmo: { value: colors.atmo } },
    side: THREE.BackSide, transparent: true, depthWrite: false,
  });
  const halo = new THREE.Mesh(sphere, haloMat);
  halo.scale.setScalar(1.09);
  scene.add(halo);

  const marks = new Dots(256, dotFrag, 5);
  const rings = new Dots(8, ringFrag, 4);
  const pulses = new Dots(6144, dotFrag, 6);
  scene.add(marks.points, rings.points, pulses.points);

  // labels for the busiest places, kept over their spots
  const labels = document.createElement("div");
  labels.className = "globe-labels";
  host.appendChild(labels);
  const labelEls = new Map<string, HTMLElement>();

  let data: Globe | null = null;
  // the countries' centres, from data
  let at: Record<string, Place> = {};
  const drawn = new Map<string, Drawn>();
  let cur: GlobeRoute[] = [];
  let highlight: string | null = null;
  let places: { cc: string; p: THREE.Vector3; weight: number; origin: boolean; direct: boolean }[] = [];
  let resumeAt = 0;
  let focusAnim: { from: THREE.Vector3; to: THREE.Vector3; t: number } | null = null;
  let width = 1, height = 1;
  let disposed = false;

  const resize = () => {
    width = Math.max(1, host.clientWidth); height = Math.max(1, host.clientHeight);
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    // keep the globe the same size in a narrow box
    camera.fov = camera.aspect < 1 ? 2 * THREE.MathUtils.radToDeg(Math.atan(Math.tan(THREE.MathUtils.degToRad(16)) / camera.aspect)) : 32;
    camera.updateProjectionMatrix();
    const scale = height * renderer.getPixelRatio() / (2 * Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)));
    for (const d of [marks, rings, pulses]) d.setScale(scale);
    for (const d of drawn.values()) d.mat.resolution.set(width, height);
  };
  const ro = new ResizeObserver(resize);
  ro.observe(host);
  resize();

  const pause = () => { controls.autoRotate = false; resumeAt = performance.now() + RESUME_MS; };
  controls.addEventListener("start", () => { focusAnim = null; pause(); resumeAt = Infinity; });
  controls.addEventListener("end", pause);

  const lineFor = (r: GlobeRoute) => (r.direct ? colors.direct : colors.proxy);

  const drop = (d: Drawn) => { scene.remove(d.line); d.line.geometry.dispose(); d.mat.dispose(); drawn.delete(d.key); };

  // sync matches the routes to their arcs by key: an arc stays as its
  // route's figures change, a new one fades in, a gone one fades out
  const sync = () => {
    const o = data?.origin ? at[data.origin] : undefined;
    const live = new Set<string>();
    if (data && o) {
      for (const r of cur) {
        const key = routeKey(r);
        const old = drawn.get(key);
        if (old && old.origin === data.origin) { old.route = r; old.gone = false; live.add(key); continue; }
        if (old) drop(old);
        const stops = [o, ...(r.via && r.via !== data.origin && r.via !== r.to ? [at[r.via]] : []), at[r.to]].filter(Boolean) as Place[];
        if (stops.length < 2 || (stops.length === 2 && r.to === data.origin)) continue;
        const path: THREE.Vector3[] = [];
        for (let i = 0; i + 1 < stops.length; i++) path.push(...arc(stops[i], stops[i + 1]).slice(i ? 1 : 0));
        let len = 0;
        for (let i = 1; i < path.length; i++) len += path[i].distanceTo(path[i - 1]);
        const geo = new LineGeometry();
        geo.setPositions(path.flatMap((v) => [v.x, v.y, v.z]));
        const mat = new LineMaterial({ color: lineFor(r).getHex(), linewidth: 1.4, transparent: true, opacity: 0, depthWrite: false });
        mat.resolution.set(width, height);
        const line = new Line2(geo, mat);
        line.renderOrder = 3;
        scene.add(line);
        // a longer arc takes longer, but not in proportion, so short ones don't crawl
        const trip = THREE.MathUtils.clamp(1.1 + len * 0.5, 1.3, 3);
        drawn.set(key, { route: r, key, origin: data.origin, line, mat, path, trip, tail: Math.min(0.3, TAIL_LEN / len), pulses: [], due: 0, rate: 0, fade: still ? 1 : 0, gone: false });
        live.add(key);
      }
    }
    for (const d of drawn.values()) if (!live.has(d.key)) d.gone = true;
  };

  const setData = (g: Globe, routes: GlobeRoute[]) => {
    const first = !data?.origin && !!g.origin;
    data = g; cur = routes;
    at = Object.fromEntries(Object.entries(g.places ?? {}).filter((e): e is [string, Place] => !!e[1]));
    readColors();
    earthMat.uniforms.sun.value = sun();
    sync();
    // the places, weighted by what went there
    // a place only direct connections reach takes their colour
    const w = new Map<string, number>(), proxied = new Set<string>();
    for (const r of routes) {
      w.set(r.to, (w.get(r.to) ?? 0) + r.total + 1);
      if (!r.direct) proxied.add(r.to);
      if (r.via) { w.set(r.via, w.get(r.via) ?? 0); proxied.add(r.via); }
    }
    places = [...w].filter(([cc]) => at[cc]).map(([cc, weight]) => ({ cc, p: vec(at[cc], MARK_R), weight, origin: false, direct: !proxied.has(cc) }));
    if (at[g.origin]) places.push({ cc: g.origin, p: vec(at[g.origin], MARK_R), weight: Infinity, origin: true, direct: false });
    if (first) faceOrigin();
    syncLabels();
  };

  const faceOrigin = () => {
    const p = data?.origin ? at[data.origin] : undefined;
    if (!p) return;
    // a little south of overhead, so the arcs going north and east show
    camera.position.copy(vec({ lat: Math.max(-30, Math.min(30, p.lat - 8)), lon: p.lon + 18 }, camera.position.length()));
    controls.update();
  };

  const syncLabels = () => {
    const top = places.filter((p) => !p.origin).sort((a, b) => b.weight - a.weight).slice(0, 6).map((p) => p.cc);
    const origin = places.find((p) => p.origin)?.cc;
    const want = new Set([...top, ...(origin ? [origin] : [])]);
    for (const [cc, el] of labelEls) if (!want.has(cc)) { el.remove(); labelEls.delete(cc); }
    for (const cc of want) {
      let el = labelEls.get(cc);
      if (!el) {
        el = document.createElement("div");
        el.className = "globe-label" + (cc === origin ? " origin" : "");
        labels.appendChild(el);
        labelEls.set(cc, el);
      }
      el.textContent = opts.label(cc);
    }
  };

  const tmp = new THREE.Vector3(), camDir = new THREE.Vector3();
  const facing = (p: THREE.Vector3) => p.clone().normalize().dot(camDir);
  const project = (p: THREE.Vector3) => {
    tmp.copy(p).project(camera);
    return { x: (tmp.x + 1) / 2 * width, y: (1 - tmp.y) / 2 * height };
  };

  const drawDots = (now: number, dt: number) => {
    marks.clear(); rings.clear(); pulses.clear();
    for (const pl of places) {
      const c = pl.origin ? colors.white : pl.direct ? colors.direct : colors.proxy;
      const size = pl.origin ? 0.05 : 0.035 + Math.min(0.04, Math.log10(pl.weight + 1) * 0.004);
      marks.push(pl.p, size, pl.origin ? 1 : 0.9, c);
      if (pl.origin && !still) {
        const t = (now / 1800) % 1;
        rings.push(pl.p, 0.08 + t * 0.22, (1 - t) * 0.9, colors.atmo);
      }
    }
    for (const d of [...drawn.values()]) {
      const r = d.route;
      const k = (rate: number) => (still ? 1 : Math.min(1, dt * rate));
      d.fade += ((d.gone ? 0 : 1) - d.fade) * k(5);
      if (d.gone && d.fade < 0.01 && !d.pulses.length) { drop(d); continue; }
      const on = highlight === d.key, dim = highlight !== null && !on;
      d.mat.opacity += ((dim ? 0.12 : on ? 0.95 : 0.55) * d.fade - d.mat.opacity) * k(8);
      d.mat.linewidth = on ? 2.4 : 1.4;
      d.mat.color.copy(lineFor(r));
      if (still) continue;
      // the speed, eased, so a quiet second doesn't stop the pulses
      d.rate += ((d.gone ? 0 : r.up + r.down) - d.rate) * k(1.2);
      if (d.rate > 256) {
        // busier routes send pulses more often
        d.due -= dt * Math.min(2.2, 0.35 + Math.log10(1 + d.rate / 1024) * 0.55);
        if (d.due <= 0 && d.pulses.length < MAX_PULSES) { d.pulses.push({ t: 0, back: r.down >= r.up }); d.due = 1; }
      }
      const c = r.direct ? colors.direct : colors.proxy;
      for (const p of d.pulses) {
        p.t += dt / d.trip;
        // in as it leaves, out as its tail reaches the end
        const env = THREE.MathUtils.smoothstep(p.t, 0, 0.08) * (1 - THREE.MathUtils.smoothstep(p.t, 0.92, 1 + d.tail)) * (dim ? 0.25 : 1);
        for (let i = 0; i < TRAIL; i++) {
          const t = Math.min(1, p.t) - (i * d.tail) / TRAIL;
          if (t < 0) break;
          const f = p.back ? 1 - t : t;
          const s = f * (d.path.length - 1), j = Math.min(d.path.length - 2, Math.floor(s));
          tmp.lerpVectors(d.path[j], d.path[j + 1], s - j);
          const fall = 1 - i / TRAIL;
          pulses.push(tmp, (i === 0 ? 0.05 : 0.036) * fall + 0.01, env * fall * fall, i === 0 ? colors.white : c);
        }
      }
      d.pulses = d.pulses.filter((p) => p.t < 1 + d.tail);
    }
    marks.flush(); rings.flush(); pulses.flush();
  };

  const placeLabels = () => {
    for (const pl of places) {
      const el = labelEls.get(pl.cc);
      if (!el) continue;
      const f = facing(pl.p);
      const { x, y } = project(pl.p);
      el.style.opacity = f > 0.25 ? String(Math.min(1, (f - 0.25) * 4)) : "0";
      el.style.transform = `translate(${x}px, ${y}px)`;
    }
  };

  const onMove = (e: PointerEvent) => {
    if (e.buttons) return;
    const b = renderer.domElement.getBoundingClientRect();
    const mx = e.clientX - b.left, my = e.clientY - b.top;
    let best: string | null = null, bestD = 14 * 14;
    for (const pl of places) {
      if (facing(pl.p) < 0.1) continue;
      const { x, y } = project(pl.p);
      const d = (x - mx) ** 2 + (y - my) ** 2;
      if (d < bestD) { bestD = d; best = pl.cc; }
    }
    renderer.domElement.style.cursor = best ? "pointer" : "";
    opts.onHover(best, e.clientX, e.clientY);
  };
  const onLeave = () => opts.onHover(null, 0, 0);
  renderer.domElement.addEventListener("pointermove", onMove);
  renderer.domElement.addEventListener("pointerleave", onLeave);

  const clock = new THREE.Clock();
  const loop = () => {
    if (disposed) return;
    const now = performance.now();
    const dt = Math.min(0.1, clock.getDelta());
    if (focusAnim) {
      focusAnim.t = Math.min(1, focusAnim.t + 1 / 50);
      const e = 1 - Math.pow(1 - focusAnim.t, 3);
      const len = THREE.MathUtils.lerp(focusAnim.from.length(), focusAnim.to.length(), e);
      camera.position.copy(focusAnim.from).normalize().lerp(tmp.copy(focusAnim.to).normalize(), e).normalize().multiplyScalar(len);
      if (focusAnim.t >= 1) focusAnim = null;
    } else if (!still && resumeAt && now > resumeAt) {
      controls.autoRotate = true; resumeAt = 0;
    }
    controls.update(dt);
    camDir.copy(camera.position).normalize();
    drawDots(now, dt);
    placeLabels();
    renderer.render(scene, camera);
  };
  const run = () => renderer.setAnimationLoop(document.hidden ? null : loop);
  document.addEventListener("visibilitychange", run);
  run();

  return {
    setData,
    setHighlight(key) { highlight = key; },
    focus(cc) {
      const p = at[cc];
      if (!p) return;
      pause();
      focusAnim = { from: camera.position.clone(), to: vec({ lat: Math.max(-50, Math.min(50, p.lat)), lon: p.lon }, Math.min(camera.position.length(), DISTANCE)), t: 0 };
    },
    dispose() {
      disposed = true;
      renderer.setAnimationLoop(null);
      document.removeEventListener("visibilitychange", run);
      ro.disconnect();
      controls.dispose();
      for (const d of [...drawn.values()]) drop(d);
      for (const d of [marks, rings, pulses]) d.dispose();
      sphere.dispose(); earthMat.dispose(); haloMat.dispose(); day.dispose(); night.dispose();
      renderer.dispose();
      renderer.domElement.remove();
      labels.remove();
    },
  };
}
