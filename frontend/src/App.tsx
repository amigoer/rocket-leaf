import type { JSX } from "react";
import { DesignApp } from "@/design/DesignApp";

/**
 * The frontend is the design canvas (mq-studio-design.dc.html) rebuilt one to
 * one. Every board reads Go now: the fifteen families the connection form
 * offers are exactly the ones that have a driver, so nothing is left drawing
 * the canvas's own figures.
 */
function App(): JSX.Element {
  return <DesignApp />;
}

export default App;
