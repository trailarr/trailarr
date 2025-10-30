import React, { useState, useEffect, lazy, Suspense } from "react";
import PropTypes from "prop-types";
import BlacklistPage from "./components/BlacklistPage";
import MediaRouteComponent from "./MediaRouteComponent";
import Toast from "./components/Toast";
import { Routes, Route, useLocation } from "react-router-dom";
// Lazy-load heavy pages to reduce initial bundle while keeping vendor_react grouping
const MediaDetails = lazy(() => import("./components/MediaDetails"));
import Header from "./components/Header";
import Sidebar from "./components/Sidebar";
import GeneralSettings from "./components/GeneralSettings";
import Tasks from "./components/Tasks";
import HistoryPage from "./components/HistoryPage";
import Wanted from "./components/Wanted";
const ProviderSettingsPage = lazy(() => import("./components/ProviderSettingsPage"));
const ExtrasSettings = lazy(() => import("./components/ExtrasSettings"));
const YtdlpFlagsSettings = lazy(() => import("./components/YtdlpFlagsSettings"));
import LogsPage from "./components/LogsPage";
import StatusPage from "./components/StatusPage";
import {
  getSeries,
  getMovies,
  getRadarrSettings,
  getMoviesWanted,
  getSeriesWanted,
} from "./api";
import ErrorBoundary from "./components/ErrorBoundary";
import { isDark } from "./utils/isDark";

// Small helper element to avoid repeating Suspense + ErrorBoundary
// Use a minimal fallback (null) so the inner component's own skeleton
// is the visible placeholder when `loading` is true. This prevents
// showing two skeletons (Suspense fallback + internal skeleton).
const MediaDetailsElement = ({ items, loading, mediaType }) => (
  <Suspense fallback={null}>
    <ErrorBoundary>
      <MediaDetails
        mediaItems={items}
        loading={loading}
        mediaType={mediaType}
      />
    </ErrorBoundary>
  </Suspense>
);

// Helper functions to avoid deep nesting
function filterAndSortMedia(items) {
  return (items || [])
    .filter((item) => item?.title)
    .sort((a, b) => a.title.localeCompare(b.title));
}
// Static imports are used instead of dynamic loading

function App() {
  const location = useLocation();
  const [search, setSearch] = useState("");
  const [selectedSection, setSelectedSection] = useState("Movies");
  const [selectedSystemSub, setSelectedSystemSub] = useState("Status");

  // Toast state
  const [toastMessage, setToastMessage] = useState("");

  // Reset search when changing main section (Movies/Series)
  useEffect(() => {
    setSearch("");
  }, [selectedSection]);
  const [selectedSettingsSub, setSelectedSettingsSub] = useState("General");

  // Sonarr series state
  const [series, setSeries] = useState([]);
  const [seriesError, setSeriesError] = useState("");
  const [seriesLoading, setSeriesLoading] = useState(true);

  // Sync sidebar state and page title with route on every navigation
  useEffect(() => {
    const path = location.pathname;
    if (path.startsWith("/settings/")) {
      setSelectedSection("Settings");
      const sub = path.split("/")[2];
      if (sub) {
        setSelectedSettingsSub(sub.charAt(0).toUpperCase() + sub.slice(1));
      }
    } else if (path.startsWith("/settings")) {
      setSelectedSection("Settings");
      setSelectedSettingsSub("General");
    } else if (path.startsWith("/wanted/movies")) {
      setSelectedSection("Wanted");
      setSelectedSettingsSub("Movies");
    } else if (path.startsWith("/wanted/series")) {
      setSelectedSection("Wanted");
      setSelectedSettingsSub("Series");
    } else if (path.startsWith("/blacklist")) {
      setSelectedSection("Blacklist");
    } else if (path === "/" || /^\/[0-9a-zA-Z_-]+$/.exec(path)) {
      setSelectedSection("Movies");
    } else if (path.startsWith("/series")) {
      setSelectedSection("Series");
    } else if (path.startsWith("/history")) {
      setSelectedSection("History");
    } else if (path.startsWith("/system/status")) {
      setSelectedSection("System");
      setSelectedSystemSub("Status");
    } else if (path.startsWith("/system/tasks")) {
      setSelectedSection("System");
      setSelectedSystemSub("Tasks");
    } else if (path.startsWith("/system/logs")) {
      setSelectedSection("System");
      setSelectedSystemSub("Logs");
    }
  }, [location.pathname]);

  useEffect(() => {
    setSeriesLoading(true);
    getSeries()
      .then((data) => {
        setSeries(filterAndSortMedia(data.series));
        setSeriesLoading(false);
        setSeriesError("");
      })
      .catch((e) => {
        setSeries([]);
        setSeriesLoading(false);
        setSeriesError(e.message || "Sonarr series API not available");
      });
  }, []);

  // Prefetch wanted lists so Wanted page behaves like Movies/Series (no separate loading)
  const [moviesWanted, setMoviesWanted] = useState([]);
  const [moviesWantedLoading, setMoviesWantedLoading] = useState(true);
  const [moviesWantedError, setMoviesWantedError] = useState("");

  const [seriesWanted, setSeriesWanted] = useState([]);
  const [seriesWantedLoading, setSeriesWantedLoading] = useState(true);
  const [seriesWantedError, setSeriesWantedError] = useState("");

  useEffect(() => {
    setMoviesWantedLoading(true);
    getMoviesWanted()
      .then((res) => {
        setMoviesWanted(filterAndSortMedia(res.items || []));
        setMoviesWantedLoading(false);
      })
      .catch((e) => {
        setMoviesWanted([]);
        setMoviesWantedLoading(false);
        setMoviesWantedError(e.message || "Failed to fetch wanted movies");
      });

    setSeriesWantedLoading(true);
    getSeriesWanted()
      .then((res) => {
        setSeriesWanted(filterAndSortMedia(res.items || []));
        setSeriesWantedLoading(false);
      })
      .catch((e) => {
        setSeriesWanted([]);
        setSeriesWantedLoading(false);
        setSeriesWantedError(e.message || "Failed to fetch wanted series");
      });
  }, []);

  const [movies, setMovies] = useState([]);
  const [moviesError, setMoviesError] = useState("");
  const [moviesLoading, setMoviesLoading] = useState(true);

  useEffect(() => {
    getRadarrSettings()
      .then((res) => {
        localStorage.setItem("radarrUrl", res.url || "");
        localStorage.setItem("radarrApiKey", res.apiKey || "");
      })
      .catch(() => {
        localStorage.setItem("radarrUrl", "");
        localStorage.setItem("radarrApiKey", "");
      });
    // Sonarr settings fetch fallback
    async function getSonarrSettings() {
      try {
        const res = await fetch("/api/settings/sonarr");
        if (!res.ok) throw new Error("Failed to fetch Sonarr settings");
        return await res.json();
      } catch {
        return { url: "", apiKey: "" };
      }
    }
    getSonarrSettings()
      .then((res) => {
        localStorage.setItem("sonarrUrl", res.url || "");
        localStorage.setItem("sonarrApiKey", res.apiKey || "");
      })
      .catch(() => {
        localStorage.setItem("sonarrUrl", "");
        localStorage.setItem("sonarrApiKey", "");
      });
  }, []);

  useEffect(() => {
    setMoviesLoading(true);
    getMovies()
      .then((res) => {
        setMovies(filterAndSortMedia(res.movies));
        setMoviesLoading(false);
      })
      .catch((e) => {
        setMoviesError(e.message);
        setMoviesLoading(false);
      });
  }, []);

  // Separate search results into title and overview matches
  const getSearchSections = (items) => {
    if (!search.trim()) return { titleMatches: items, overviewMatches: [] };
    const q = search.trim().toLowerCase();
    const titleMatches = items.filter((item) =>
      item.title?.toLowerCase().includes(q),
    );
    const overviewMatches = items.filter(
      (item) =>
        !titleMatches.includes(item) &&
        item.overview?.toLowerCase().includes(q),
    );
    return { titleMatches, overviewMatches };
  };

  // Compute dynamic page title
  let pageTitle = selectedSection;
  if (selectedSection === "Settings") {
    pageTitle = `${selectedSettingsSub || ""} Settings`;
  } else if (selectedSection === "Wanted") {
    pageTitle = `Wanted${selectedSettingsSub ? " " + selectedSettingsSub : ""}`;
  } else if (selectedSection === "System") {
    pageTitle = `System${selectedSystemSub ? " " + selectedSystemSub : ""}`;
  }

  // Update document title dynamically
  useEffect(() => {
    if (globalThis.setTrailarrTitle) {
      globalThis.setTrailarrTitle(pageTitle);
    }
  }, [pageTitle]);

  // Components are statically imported at module top

  // Mobile detection
  const [isMobile, setIsMobile] = useState(window.innerWidth < 900);
  useEffect(() => {
    const handleResize = () => setIsMobile(window.innerWidth < 900);
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  // Sidebar open state for mobile
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const handleSidebarToggle = () => setSidebarOpen((v) => !v);
  const handleSidebarClose = () => setSidebarOpen(false);

  return (
    <div
      className="app-container"
      style={{ width: "100vw", minHeight: "100vh", overflowX: "hidden" }}
    >
      <Header
        search={search}
        setSearch={setSearch}
        pageTitle={pageTitle}
        mobile={isMobile}
        sidebarOpen={sidebarOpen}
        onSidebarToggle={handleSidebarToggle}
      />
      <div
        style={{
          display: "flex",
          width: "100vw",
          height: "100%",
          position: "relative",
        }}
      >
        <Sidebar
          selectedSection={selectedSection}
          setSelectedSection={setSelectedSection}
          selectedSettingsSub={selectedSettingsSub}
          setSelectedSettingsSub={setSelectedSettingsSub}
          selectedSystemSub={selectedSystemSub}
          setSelectedSystemSub={setSelectedSystemSub}
          mobile={isMobile}
          open={sidebarOpen}
          onClose={handleSidebarClose}
          onToggle={handleSidebarToggle}
        />
        <main
          style={{
            flex: 1,
            padding: "0em",
            height: "100%",
            boxSizing: "border-box",
            overflowY: "auto",
            overflowX: "hidden",
            display: "flex",
            flexDirection: "column",
            alignItems: "flex-start",
            justifyContent: "stretch",
            maxWidth: "100vw",
            background: isDark ? "#18181b" : "#fff",
            color: isDark ? "#e5e7eb" : "#222",
          }}
        >
          <div
            style={{
              background: isDark ? "#23232a" : "#fff",
              boxShadow: isDark ? "0 1px 4px #222" : "0 1px 4px #e5e7eb",
              padding: "0em",
              width: `calc(100% - ${window.innerWidth > 900 ? 220 : 0}px)`,
              maxWidth: `calc(100% - ${window.innerWidth > 900 ? 220 : 0}px)`,
              flex: 1,
              overflowY: "auto",
              overflowX: "hidden",
              color: isDark ? "#e5e7eb" : "#222",
              marginLeft: window.innerWidth > 900 ? 220 : 0,
              marginTop: 64,
            }}>
            <Routes>
              <Route
                path="/series"
                element={
                  <MediaRouteComponent
                    items={series}
                    search={search}
                    error={seriesError}
                    getSearchSections={getSearchSections}
                    type="series"
                    loading={seriesLoading}
                  />
                }
              />
              <Route
                path="/"
                element={
                  <MediaRouteComponent
                    items={movies}
                    search={search}
                    error={moviesError}
                    getSearchSections={getSearchSections}
                    type="movie"
                    loading={moviesLoading}
                  />
                }
              />

              {/* Media details routes use a small shared element to avoid repeating Suspense + ErrorBoundary */}
              <Route
                path="/movies/:id"
                element={
                  <MediaDetailsElement
                    items={movies}
                    loading={moviesLoading}
                    mediaType="movie"
                  />
                }
              />
              <Route
                path="/series/:id"
                element={
                  <MediaDetailsElement
                    items={series}
                    loading={seriesLoading}
                    mediaType="tv"
                  />
                }
              />
              <Route
                path="/wanted/movies/:id"
                element={
                  <MediaDetailsElement
                    items={movies}
                    loading={moviesLoading}
                    mediaType="movie"
                  />
                }
              />
              <Route
                path="/wanted/series/:id"
                element={
                  <MediaDetailsElement
                    items={series}
                    loading={seriesLoading}
                    mediaType="tv"
                  />
                }
              />
              <Route
                path="/history/movies/:id"
                element={
                  <MediaDetailsElement
                    items={movies}
                    loading={moviesLoading}
                    mediaType="movie"
                  />
                }
              />
              <Route
                path="/history/series/:id"
                element={
                  <MediaDetailsElement
                    items={series}
                    loading={seriesLoading}
                    mediaType="tv"
                  />
                }
              />

              <Route path="/history" element={<HistoryPage />} />
              <Route
                path="/wanted/movies"
                element={
                  <Wanted
                    type="movie"
                    items={moviesWanted}
                    loading={moviesWantedLoading}
                    error={moviesWantedError}
                  />
                }
              />
              <Route
                path="/wanted/series"
                element={
                  <Wanted
                    type="series"
                    items={seriesWanted}
                    loading={seriesWantedLoading}
                    error={seriesWantedError}
                  />
                }
              />
              <Route
                path="/settings/radarr"
                element={
                  <Suspense fallback={null}>
                    <ProviderSettingsPage type="radarr" />
                  </Suspense>
                }
              />
              <Route
                path="/settings/sonarr"
                element={
                  <Suspense fallback={null}>
                    <ProviderSettingsPage type="sonarr" />
                  </Suspense>
                }
              />
              <Route path="/settings/general" element={<GeneralSettings />} />
              <Route
                path="/settings/extras"
                element={
                  <Suspense fallback={null}>
                    <ExtrasSettings />
                  </Suspense>
                }
              />
              <Route
                path="/settings/ytdlp"
                element={
                  <Suspense fallback={null}>
                    <YtdlpFlagsSettings />
                  </Suspense>
                }
              />
              <Route path="/system/tasks" element={<Tasks />} />
              <Route path="/system/status" element={<StatusPage />} />
              <Route path="/system/logs" element={<LogsPage />} />
              <Route path="/blacklist" element={<BlacklistPage />} />
            </Routes>
          </div>
        </main>
      </div>

      {/* Toast Modal */}
      <Toast message={toastMessage} onClose={() => setToastMessage("")} />
    </div>
  );
}

MediaDetailsElement.propTypes = {
  items: PropTypes.array,
  loading: PropTypes.bool,
  mediaType: PropTypes.string,
};

export default App;
