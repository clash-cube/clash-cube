import { useEffect } from "react";
import { useStore } from "../store";
import "./native";

export function useSystemBars() {
  const theme = useStore(s => s.settings?.theme);
  useEffect(() => {
    const root = document.documentElement;
    const media = matchMedia("(prefers-color-scheme: dark)");
    const update = () => {
      const systemDark = window.clashcubeAndroid?.isSystemDark() ?? media.matches;
      const dark = theme === "dark" || (theme !== "light" && systemDark);
      // Older Android WebViews do not update prefers-color-scheme on uiMode changes.
      const resolved = dark ? "dark" : "light";
      if (root.dataset.theme !== resolved) root.dataset.theme = resolved;
      window.clashcubeAndroid?.setSystemBars(getComputedStyle(root).getPropertyValue("--bg").trim(), dark);
    };
    const observer = new MutationObserver(update);
    observer.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    media.addEventListener("change", update);
    window.addEventListener("clashcube:theme", update);
    update();
    return () => { observer.disconnect(); media.removeEventListener("change", update); window.removeEventListener("clashcube:theme", update); };
  }, [theme]);
}
