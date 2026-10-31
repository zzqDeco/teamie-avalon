import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { DevSimulator } from "./DevSimulator";
import "./styles.css";

const isDevWorkbench =
  typeof window !== "undefined" &&
  (window.location.pathname === "/dev" || window.location.pathname.startsWith("/dev/"));

createRoot(document.getElementById("root")!).render(
  <StrictMode>{isDevWorkbench ? <DevSimulator /> : <App />}</StrictMode>,
);
