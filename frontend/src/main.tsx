import React from "react";
import ReactDOM from "react-dom/client";
import "./styles/tokens.css";
import "./styles/app.css";
import "./styles/panel.css";
import { boot } from "./store";
import { MainWindow } from "./MainWindow";
import { Panel } from "./Panel";
import { WebpageRule } from "./WebpageRule";
import { Toasts } from "./components/Toast";
import { isWindows, isWeb } from "./platform";

document.documentElement.dataset.platform = isWeb ? "web" : isWindows ? "windows" : "macos";
if (isWeb) { document.documentElement.classList.remove("is-panel"); document.documentElement.classList.add("is-window"); }

const mode = isWeb ? null : new URLSearchParams(location.search).get("mode");

boot().finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      {mode === "panel" ? <Panel /> : mode === "webpage" ? <WebpageRule /> : <MainWindow />}
      <Toasts />
    </React.StrictMode>,
  );
});
