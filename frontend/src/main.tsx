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
import { initPlatform, isAndroid } from "./platform";
import { MobileApp } from "./mobile/MobileApp";
import "./mobile/mobile.css";

const mode = new URLSearchParams(location.search).get("mode");

initPlatform().then(boot).finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      {isAndroid ? <MobileApp /> : mode === "panel" ? <Panel /> : mode === "webpage" ? <WebpageRule /> : <MainWindow />}
      <Toasts />
    </React.StrictMode>,
  );
});
