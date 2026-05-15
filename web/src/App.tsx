import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route } from "react-router";
import { Sidebar } from "./components/Sidebar";
import { AuthGate } from "./components/AuthGate";
import { CommandPalette } from "./components/CommandPalette";
import { HelpOverlay } from "./components/HelpOverlay";
import { SearchView } from "./views/SearchView";
import { uiBase } from "./utils/base";
import styles from "./App.module.css";

const QueriesView = lazy(() => import("./views/QueriesView"));
const SettingsView = lazy(() => import("./views/SettingsView"));

export function App() {
  return (
    <AuthGate>
      <BrowserRouter basename={uiBase || "/"}>
        <div className={styles.shell}>
          <Sidebar />
          <main className={styles.content}>
            <Suspense fallback={null}>
              <Routes>
                <Route path="/" element={<SearchView />} />
                <Route path="/queries" element={<QueriesView />} />
                <Route path="/settings" element={<SettingsView />} />
              </Routes>
            </Suspense>
          </main>
          <CommandPalette />
          <HelpOverlay />
        </div>
      </BrowserRouter>
    </AuthGate>
  );
}
