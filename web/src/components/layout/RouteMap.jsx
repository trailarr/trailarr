import React from "react";
import MediaList from "../media/MediaList";
import MediaDetails from "../media/MediaDetails";
import GeneralSettings from "../settings/GeneralSettings";
import ExtrasSettings from "../settings/ExtrasSettings";
import HistoryPage from "../pages/HistoryPage";
import LogsPage from "../pages/LogsPage";
import ProviderSettingsPage from "../pages/ProviderSettingsPage";
import Tasks from "../pages/Tasks";
import Wanted from "../pages/Wanted";

export const RouteMap = [
  { pattern: /^\/$/, section: "Movies" },
  { pattern: /^\/movies/, section: "Movies" },
  { pattern: /^\/series/, section: "Series" },
  { pattern: /^\/history/, section: "History" },
  { pattern: /^\/blacklist/, section: "Blacklist" },
  { pattern: /^\/wanted\/movies/, section: "Wanted", submenu: "Movies" },
  { pattern: /^\/wanted\/series/, section: "Wanted", submenu: "Series" },
  { pattern: /^\/wanted/, section: "Wanted", submenu: "Movies" },
  {
    pattern: /^\/settings\/(radarr|sonarr|general|extras)/,
    section: "Settings",
  },
  { pattern: /^\/settings\/radarr/, section: "Settings", submenu: "Radarr" },
  { pattern: /^\/settings\/sonarr/, section: "Settings", submenu: "Sonarr" },
  { pattern: /^\/settings\/general/, section: "Settings", submenu: "General" },
  { pattern: /^\/settings\/extras/, section: "Settings", submenu: "Extras" },
  { pattern: /^\/settings/, section: "Settings", submenu: "General" },
  { pattern: /^\/system\/tasks/, section: "System", systemSub: "Tasks" },
  { pattern: /^\/system\/logs/, section: "System", systemSub: "Logs" },
  { pattern: /^\/system/, section: "System", systemSub: "Tasks" },
];

export const appRoutes = [
  // Dynamic routes (functions)
  {
    path: "/series",
    dynamic: true,
    render: (props) => {
      const { series, search, getSearchSections, seriesError } = props;
      const { titleMatches, overviewMatches } = getSearchSections(series);
      return (
        <>
          {search.trim() ? (
            <>
              {(() =>
                React.createElement(MediaList, {
                  items: titleMatches,
                  type: "series",
                }))()}
              <div
                style={{
                  margin: "1.5em 0 0.5em 1em",
                  fontWeight: 700,
                  fontSize: 26,
                  textAlign: "left",
                  width: "100%",
                  letterSpacing: 0.5,
                }}
              >
                Other Results
              </div>
              {(() =>
                React.createElement(MediaList, {
                  items: overviewMatches,
                  type: "series",
                }))()}
            </>
          ) : (
            (() =>
              React.createElement(MediaList, {
                items: series,
                type: "series",
              }))()
          )}
          {seriesError && (
            <div style={{ color: "red", marginTop: "1em" }}>{seriesError}</div>
          )}
        </>
      );
    },
  },
  {
    path: "/",
    dynamic: true,
    render: (props) => {
      const { movies, search, getSearchSections, moviesError } = props;
      const { titleMatches, overviewMatches } = getSearchSections(movies);
      return (
        <>
          {search.trim() ? (
            <>
              {(() =>
                React.createElement(MediaList, {
                  items: titleMatches,
                  type: "movie",
                }))()}
              <div
                style={{
                  margin: "1.5em 0 0.5em 1em",
                  fontWeight: 700,
                  fontSize: 26,
                  textAlign: "left",
                  width: "100%",
                  letterSpacing: 0.5,
                }}
              >
                Other Results
              </div>
              {(() =>
                React.createElement(MediaList, {
                  items: overviewMatches,
                  type: "movie",
                }))()}
            </>
          ) : (
            (() =>
              React.createElement(MediaList, {
                items: movies,
                type: "movie",
              }))()
          )}
          {moviesError && (
            <div style={{ color: "red", marginTop: "1em" }}>{moviesError}</div>
          )}
        </>
      );
    },
  },
  {
    path: "/movies/:id",
    dynamic: true,
    render: (props) =>
      React.createElement(MediaDetails, {
        mediaItems: props.movies,
        loading: props.moviesLoading,
        mediaType: "movie",
      }),
  },
  {
    path: "/series/:id",
    dynamic: true,
    render: (props) =>
      React.createElement(MediaDetails, {
        mediaItems: props.series,
        loading: props.seriesLoading,
        mediaType: "tv",
      }),
  },
  // Static routes
  {
    path: "/history",
    element: React.createElement(HistoryPage),
  },
  {
    path: "/wanted/movies",
    dynamic: true,
    render: (props) =>
      React.createElement(Wanted, {
        type: "movie",
        items: props.movies,
      }),
  },
  {
    path: "/wanted/series",
    dynamic: true,
    render: (props) =>
      React.createElement(Wanted, {
        type: "series",
        items: props.series,
      }),
  },
  {
    path: "/settings/radarr",
    element: React.createElement(ProviderSettingsPage, { type: "radarr" }),
  },
  {
    path: "/settings/sonarr",
    element: React.createElement(ProviderSettingsPage, { type: "sonarr" }),
  },
  {
    path: "/settings/general",
    element: React.createElement(GeneralSettings),
  },
  {
    path: "/settings/extras",
    element: React.createElement(ExtrasSettings),
  },
  {
    path: "/system/tasks",
    element: React.createElement(Tasks),
  },
  {
    path: "/system/logs",
    element: React.createElement(LogsPage),
  },
];
