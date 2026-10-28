import { parseDocument } from "yaml";

export interface YamlProblem {
  from: number;
  to: number;
  line: number;
  column: number;
  message: string;
}

// Parse only the document tree. Converting aliases to JS would needlessly
// expand user input, and syntax checking does not require its values.
export function checkYaml(source: string): YamlProblem[] {
  const doc = parseDocument(source, { prettyErrors: false, uniqueKeys: true, logLevel: "silent" });
  return doc.errors.map((error) => {
    const from = Math.min(source.length, Math.max(0, error.pos[0]));
    const before = source.slice(0, from);
    return {
      from, to: Math.min(source.length, Math.max(from, error.pos[1])),
      line: before.split("\n").length, column: from - before.lastIndexOf("\n"),
      message: error.message,
    };
  });
}
