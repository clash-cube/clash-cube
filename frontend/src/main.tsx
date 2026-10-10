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
import { isWindows } from "./platform";

document.documentElement.dataset.platform = isWindows ? "windows" : "macos";

const mode = new URLSearchParams(location.search).get("mode");

boot().finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      {mode === "panel" ? <Panel /> : mode === "webpage" ? <WebpageRule /> : <MainWindow />}
      <Toasts />
    </React.StrictMode>,
  );
});
