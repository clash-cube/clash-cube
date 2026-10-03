// Line icons at 16px, stroked in the text colour.
type P = { size?: number };
const s = (size = 16) => ({ width: size, height: size, viewBox: "0 0 16 16", fill: "none", stroke: "currentColor", strokeWidth: 1.5, strokeLinecap: "round" as const, strokeLinejoin: "round" as const });

export const Logo = ({ size = 18 }: P) => (
  <svg width={size} height={size} viewBox="0 0 22 22" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinejoin="round">
    <path d="M11 2.2 18.6 6.6v8.8L11 19.8 3.4 15.4V6.6Z" />
    <path d="M3.8 6.8 11 11l7.2-4.2M11 11v8.4" />
    <path d="M11 11 18.2 6.8v8.4L11 19.4Z" fill="currentColor" stroke="none" />
  </svg>
);
// Lucide's "settings" icon, drawn on a 24px grid.
export const Gear = ({ size }: P) => (
  <svg {...s(size)} viewBox="0 0 24 24" strokeWidth={2}>
    <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
    <circle cx="12" cy="12" r="3" />
  </svg>
);
export const Bolt = ({ size }: P) => (<svg {...s(size)}><path d="M9 1.5 3.5 9H8l-1 5.5L12.5 7H8z" /></svg>);
export const Refresh = ({ size }: P) => (<svg {...s(size)}><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" /><path d="M13.5 2.5v3h-3" /></svg>);
export const Chevron = ({ size = 12, className = "" }: P & { className?: string }) => (<svg {...s(size)} className={className}><path d="m6 3.5 4.5 4.5L6 12.5" /></svg>);
export const Plus = ({ size }: P) => (<svg {...s(size)}><path d="M8 3v10M3 8h10" /></svg>);
export const More = ({ size }: P) => (<svg {...s(size)} fill="currentColor" stroke="none"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>);
export const Close = ({ size }: P) => (<svg {...s(size)}><path d="m4 4 8 8M12 4l-8 8" /></svg>);
export const Search = ({ size = 14 }: P) => (<svg {...s(size)}><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3 3" /></svg>);
export const Window = ({ size }: P) => (<svg {...s(size)}><rect x="2" y="3" width="12" height="10" rx="2" /><path d="M2 6h12" /></svg>);
export const Power = ({ size }: P) => (<svg {...s(size)}><path d="M8 2v6" /><path d="M4.5 4.2a5 5 0 1 0 7 0" /></svg>);
export const Play = ({ size }: P) => (<svg {...s(size)}><path d="M5 3.5v9l7-4.5z" /></svg>);
export const Stop = ({ size }: P) => (<svg {...s(size)}><rect x="4" y="4" width="8" height="8" rx="1.5" /></svg>);
export const Globe = ({ size }: P) => (<svg {...s(size)}><circle cx="8" cy="8" r="6" /><path d="M2 8h12M8 2c1.8 2 2.6 4 2.6 6S9.8 12 8 14c-1.8-2-2.6-4-2.6-6S6.2 4 8 2z" /></svg>);
export const File = ({ size }: P) => (<svg {...s(size)}><path d="M4 1.8h5L12.5 5v9.2H4z" /><path d="M9 1.8V5h3.5" /></svg>);
export const Shield = ({ size }: P) => (<svg {...s(size)}><path d="M8 1.8 13 3.8v4c0 3-2.2 5.2-5 6.4-2.8-1.2-5-3.4-5-6.4v-4z" /></svg>);
export const Arrow = ({ size, dir }: P & { dir: "up" | "down" }) => (<svg {...s(size)} style={{ transform: dir === "down" ? "rotate(180deg)" : undefined }}><path d="M8 13V3M4 7l4-4 4 4" /></svg>);
export const Sort = ({ size }: P) => (<svg {...s(size)}><path d="M4 3v10M2 11l2 2 2-2M10 4h4M10 8h3M10 12h2" /></svg>);
