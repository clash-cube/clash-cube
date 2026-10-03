import { useRef, useState } from "react";
import { connectionColumns, clampColumnWidth, type ConnectionColumn } from "../connectionColumns";
import { useT } from "../i18n";
import { Popover } from "./Popover";

export function ConnectionColumnMenu({ selected, onChange, onReset }: {
  selected: ConnectionColumn[]; onChange: (columns: ConnectionColumn[]) => void; onReset: () => void;
}) {
  const t = useT();
  const anchor = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  return <>
    <button ref={anchor} className="btn small" aria-expanded={open} onClick={() => setOpen(!open)}>{t("Columns")}</button>
    <Popover anchor={anchor.current} open={open} onClose={() => setOpen(false)} width={220} align="end">
      <div className="conn-column-menu">
        {connectionColumns.map((c) => <label key={c.id}><input type="checkbox" checked={selected.includes(c.id)} disabled={c.id === "host"}
          onChange={() => onChange(connectionColumns.filter((candidate) => candidate.id === c.id ? !selected.includes(c.id) : selected.includes(candidate.id)).map((c) => c.id))} />{t(c.label)}</label>)}
        <span className="sub">{t("Drag column edges to resize")}</span>
        <button className="btn small" onClick={onReset}>{t("Reset columns")}</button>
      </div>
    </Popover>
  </>;
}

export function ConnectionColumnHeader({ columns, widths, sort, ascending, onSort, onPreview, onResize }: {
  columns: ConnectionColumn[]; widths: Partial<Record<ConnectionColumn, number>>;
  sort: ConnectionColumn; ascending: boolean; onSort: (id: ConnectionColumn) => void;
  onPreview: (id: ConnectionColumn, width: number | null) => void; onResize: (id: ConnectionColumn, width: number) => void;
}) {
  const t = useT();
  const drag = useRef<{ id: ConnectionColumn; x: number; width: number; next: number } | null>(null);
  return <div className="conn-column-head" role="row">
    {columns.map((id) => {
      const c = connectionColumns.find((c) => c.id === id)!;
      const width = widths[id] ?? c.width;
      return <div className="conn-column-title" key={id} role="columnheader" aria-sort={sort === id ? ascending ? "ascending" : "descending" : "none"}>
        <button onClick={() => onSort(id)} title={t(c.label)}>{t(c.label)}{sort === id && <span>{ascending ? " ↑" : " ↓"}</span>}</button>
        <span className="conn-column-resize" role="separator" tabIndex={0} aria-orientation="vertical" aria-label={t("Resize {column}", { column: t(c.label) })}
          aria-valuemin={c.min} aria-valuemax={600} aria-valuenow={width}
          onKeyDown={(e) => {
            if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
            e.preventDefault(); onResize(id, clampColumnWidth(id, width + (e.key === "ArrowLeft" ? -10 : 10)));
          }}
          onPointerDown={(e) => {
            if (e.button !== 0) return;
            e.preventDefault(); e.currentTarget.setPointerCapture(e.pointerId);
            const actualWidth = e.currentTarget.parentElement!.getBoundingClientRect().width;
            drag.current = { id, x: e.clientX, width: actualWidth, next: actualWidth };
          }}
          onPointerMove={(e) => {
            const d = drag.current;
            if (!d || d.id !== id) return;
            d.next = clampColumnWidth(id, d.width + e.clientX - d.x); onPreview(id, d.next);
          }}
          onPointerUp={() => {
            const d = drag.current; if (!d || d.id !== id) return;
            drag.current = null; onPreview(id, null); onResize(id, clampColumnWidth(id, d.next));
          }}
          onLostPointerCapture={() => { if (drag.current) { drag.current = null; onPreview(id, null); } }}
        />
      </div>;
    })}
    <span aria-label={t("Close connection")} />
  </div>;
}
