export function Switch({ on, onChange, busy, disabled, label }: { on: boolean; onChange: (v: boolean) => void; busy?: boolean; disabled?: boolean; label?: string }) {
  return (
    <button
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      className={"switch" + (on ? " on" : "") + (busy ? " busy" : "")}
      onClick={(e) => { e.stopPropagation(); if (!busy) onChange(!on); }}
    />
  );
}
