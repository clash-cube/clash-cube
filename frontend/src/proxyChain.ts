import type { Group } from "./api";

// Include the terminal name, but never repeat a name or exceed eight segments.
export function chain(start: string, groups: Group[]): string[] {
  const result = [start];
  const seen = new Set(result);
  while (result.length < 8) {
    const group = groups.find((g) => g.name === result[result.length - 1]);
    if (!group?.now || seen.has(group.now)) break;
    result.push(group.now);
    seen.add(group.now);
  }
  return result;
}
