import React from "react";
import ReactDOM from "react-dom/client";
import "./styles/tokens.css";
import "./styles/app.css";
import "./styles/panel.css";
import { boot } from "./store";
import { MainWindow } from "./MainWindow";
import { Panel } from "./Panel";
import { Toasts } from "./components/Toast";

const panel = new URLSearchParams(location.search).get("mode") === "panel";

boot().finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      {panel ? <Panel /> : <MainWindow />}
      <Toasts />
    </React.StrictMode>,
  );
});
