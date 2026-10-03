import { useEffect, useState } from "react";
import { App } from "../api";
import { Logo } from "./Icons";

const icons = new Map<string, Promise<string>>();

// Helpers inside an app bundle should show the owning application's icon.
export function AppIcon({ path, core = false }: { path: string; core?: boolean }) {
  const bundle = path.indexOf(".app/");
  const iconPath = bundle < 0 ? path : path.slice(0, bundle + 4);
  const [image, setImage] = useState({ path: "", src: "" });
  useEffect(() => {
    if (core) return;
    let request = icons.get(iconPath);
    if (!request) {
      request = App.AppIcon(iconPath);
      icons.set(iconPath, request);
      request.catch(() => icons.delete(iconPath));
    }
    let live = true;
    request.then((src) => { if (live) setImage({ path: iconPath, src }); }, () => {});
    return () => { live = false; };
  }, [iconPath, core]);
  if (core) return <span className="pcicon"><Logo size={16} /></span>;
  return image.path === iconPath && image.src
    ? <img className="pcicon" src={image.src} alt="" /> : <span className="pcicon" />;
}
