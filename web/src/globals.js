/* Globals bridge — the design files (components/cards/store/…) were authored
   for a Babel-in-browser setup where React, its hooks, and every component
   live on the global scope. Rather than rewrite each file, we expose the same
   globals here. This module is imported FIRST so React exists before any
   design file (or the Soroban SDK module) evaluates. */
import React from "react";
import { createRoot } from "react-dom/client";

window.React = React;
Object.assign(window, React); // useState, useEffect, useRef, useCallback, useContext, useMemo, createContext, …
window.ReactDOM = { createRoot };
