import { checkYaml } from "./yamlDiagnostics";

self.onmessage = ({ data }: MessageEvent<{ id: number; body: string }>) => {
  self.postMessage({ id: data.id, problems: checkYaml(data.body) });
};
