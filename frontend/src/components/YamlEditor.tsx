import { useEffect, useRef, useState } from "react";
import { ChangeSet, Compartment, EditorState } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { HighlightStyle, indentUnit, syntaxHighlighting } from "@codemirror/language";
import { yaml } from "@codemirror/lang-yaml";
import { lintGutter, setDiagnostics } from "@codemirror/lint";
import { getChunks, getOriginalDoc, goToNextChunk, goToPreviousChunk, originalDocChangeEffect, unifiedMergeView } from "@codemirror/merge";
import { tags } from "@lezer/highlight";
import type { YamlProblem } from "./yamlDiagnostics";
import { useT } from "../i18n";
import { Switch } from "./Switch";
import { Arrow, Copy } from "./Icons";

const highlighting = HighlightStyle.define([
  { tag: tags.propertyName, class: "yaml-key" },
  { tag: tags.string, class: "yaml-string" },
  { tag: [tags.number, tags.bool, tags.null], class: "yaml-value" },
  { tag: tags.comment, class: "yaml-comment" },
  { tag: [tags.meta, tags.labelName], class: "yaml-meta" },
]);

interface Props {
  body: string;
  original: string;
  readOnly: boolean;
  subscription: boolean;
  cursor: { line: number; column: number };
  lines: number;
  copyDisabled: boolean;
  onCopy: () => void;
  onChange: (body: string) => void;
  onCheck: (pending: boolean, invalid: boolean) => void;
  onCursor: (cursor: { line: number; column: number }) => void;
}

export function YamlEditor(props: Props) {
  const t = useT();
  const host = useRef<HTMLDivElement>(null), editor = useRef<EditorView>();
  const callbacks = useRef(props);
  callbacks.current = props;
  const [editable] = useState(() => new Compartment());
  const [merge] = useState(() => new Compartment());
  const [showDiff, setShowDiff] = useState(true);
  const [changes, setChanges] = useState(0);
  const [check, setCheck] = useState<{ pending: boolean; unavailable: boolean; problems: YamlProblem[] }>({ pending: true, unavailable: false, problems: [] });

  useEffect(() => {
    let live = true, request = 0, timer = 0;
    let worker: Worker | undefined;
    const unavailable = () => {
      if (!live) return;
      window.clearTimeout(timer);
      worker?.terminate(); worker = undefined;
      setCheck({ pending: false, unavailable: true, problems: [] });
      callbacks.current.onCheck(false, false);
    };
    try {
      worker = new Worker(new URL("./yamlValidation.worker.ts", import.meta.url), { type: "module" });
      worker.onerror = (event) => { event.preventDefault(); unavailable(); };
    } catch { unavailable(); }
    const validate = () => {
      const id = ++request;
      window.clearTimeout(timer);
      if (!worker) return;
      setCheck({ pending: true, unavailable: false, problems: [] });
      callbacks.current.onCheck(true, false);
      timer = window.setTimeout(() => worker?.postMessage({ id, body: view.state.doc.toString() }), 250);
    };
    const view = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: callbacks.current.body,
        extensions: [
          lineNumbers(), highlightActiveLineGutter(), highlightActiveLine(), drawSelection(),
          history(), keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
          indentUnit.of("  "), EditorState.tabSize.of(2), yaml(), syntaxHighlighting(highlighting), lintGutter(),
          EditorView.contentAttributes.of({ "aria-label": t("Configuration YAML"), "aria-multiline": "true", spellcheck: "false" }),
          editable.of([EditorState.readOnly.of(callbacks.current.readOnly), EditorView.editable.of(!callbacks.current.readOnly)]),
          merge.of(unifiedMergeView({ original: callbacks.current.original, mergeControls: false })),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              callbacks.current.onChange(update.state.doc.toString());
              validate();
            }
            if (update.selectionSet || update.docChanged) {
              const at = update.state.selection.main.head, line = update.state.doc.lineAt(at);
              callbacks.current.onCursor({ line: line.number, column: at - line.from + 1 });
            }
            setChanges(getChunks(update.state)?.chunks.length ?? 0);
          }),
        ],
      }),
    });
    editor.current = view;
    if (worker) worker.onmessage = ({ data }: MessageEvent<{ id: number; problems: YamlProblem[] }>) => {
      // Invalidate responses as soon as the draft changes, including during
      // debounce. An earlier valid draft must never enable saving a new one.
      if (!live || data.id !== request) return;
      view.dispatch(setDiagnostics(view.state, data.problems.map((p) => ({ ...p, severity: "error" as const }))));
      setCheck({ pending: false, unavailable: false, problems: data.problems });
      callbacks.current.onCheck(false, data.problems.length > 0);
    };
    validate();
    view.focus();
    return () => {
      live = false; window.clearTimeout(timer); worker?.terminate(); view.destroy(); editor.current = undefined;
    };
  }, []);

  useEffect(() => {
    const view = editor.current;
    if (view && view.state.doc.toString() !== props.body) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: props.body } });
    }
  }, [props.body]);
  useEffect(() => {
    editor.current?.dispatch({ effects: editable.reconfigure([EditorState.readOnly.of(props.readOnly), EditorView.editable.of(!props.readOnly)]) });
    if (!props.readOnly) editor.current?.focus();
  }, [props.readOnly, editable]);
  useEffect(() => {
    const view = editor.current;
    if (!view) return;
    // The baseline advances only after an acknowledged save/reload. Toggling
    // diff never accepts changes or alters the user's undo history.
    if (showDiff && getChunks(view.state)) {
      const original = getOriginalDoc(view.state);
      if (original.toString() !== props.original) view.dispatch({ effects: originalDocChangeEffect(view.state, ChangeSet.of({ from: 0, to: original.length, insert: props.original }, original.length)) });
    } else {
      view.dispatch({ effects: merge.reconfigure(showDiff ? unifiedMergeView({ original: props.original, mergeControls: false }) : []) });
    }
  }, [props.original, showDiff, merge]);

  const jump = (by: number) => {
    const view = editor.current;
    if (view) { (by > 0 ? goToNextChunk : goToPreviousChunk)(view); view.focus(); }
  };
  const problem = check.problems[0];
  return <div className="yaml-editor">
    <div className="yaml-toolbar">
      {props.subscription ? <span className="yaml-readonly">{t("Subscription configurations are read-only; make an editable copy")}</span> : <>
        <label className="yaml-diff-toggle"><Switch on={showDiff} onChange={setShowDiff} label={t("Show changes")} />{t("Show changes")}</label>
        {showDiff && <span className="muted">{changes ? t("{n} changes", { n: changes }) : t("No changes")}</span>}
        {showDiff && changes > 0 && <><button type="button" className="profile-editor-icon" aria-label={t("Previous change")} title={t("Previous change")} onClick={() => jump(-1)}><Arrow dir="up" size={14} /></button><button type="button" className="profile-editor-icon" aria-label={t("Next change")} title={t("Next change")} onClick={() => jump(1)}><Arrow dir="down" size={14} /></button></>}
      </>}
      <button type="button" className="profile-editor-icon yaml-copy" aria-label={t("Copy all")} title={t("Copy all")} disabled={props.copyDisabled} onClick={props.onCopy}><Copy size={14} /></button>
    </div>
    <div ref={host} className="yaml-editor-surface" />
    <div className="yaml-statusbar">
      <div className={"yaml-check" + (problem ? " err" : "")} role="status" aria-live="polite" title={check.unavailable ? t("Live YAML checking is unavailable; saving still validates the configuration.") : undefined}>
        <span aria-hidden="true" className={"yaml-check-dot " + (check.pending ? "pending" : check.unavailable ? "unavailable" : problem ? "invalid" : "valid")} />
      {check.pending ? t("Checking YAML…") : check.unavailable ? t("Live check unavailable") : problem ? <>
        <button type="button" className="yaml-error-link" onClick={() => {
          editor.current?.dispatch({ selection: { anchor: problem.from }, scrollIntoView: true }); editor.current?.focus();
        }}>{t("{n} YAML errors", { n: check.problems.length })} · {t("Line {line}, column {column}", { line: problem.line, column: problem.column })}</button>
        <span className="yaml-error-message" title={t(problem.message)}>{t(problem.message)}</span>
      </> : t("YAML syntax valid")}
      </div>
      <span className="yaml-cursor">{t("Line {line}, column {column}", props.cursor)} · {t("{n} lines", { n: props.lines })}</span>
    </div>
  </div>;
}
