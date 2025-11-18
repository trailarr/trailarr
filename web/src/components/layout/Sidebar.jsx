import React from "react";
import PropTypes from "prop-types";
import { useLocation } from "react-router-dom";
import SidebarDesktop from "./SidebarDesktop.jsx";
import SidebarMobile from "./SidebarMobile.jsx";

function getSelectedSection(path) {
  if (path === "/" || path.startsWith("/movies")) return "Movies";
  if (path.startsWith("/series")) return "Series";
  if (path.startsWith("/history")) return "History";
  if (path.startsWith("/wanted")) return "Wanted";
  if (path.startsWith("/blacklist")) return "Blacklist";
  if (path.startsWith("/settings")) return "Settings";
  if (path.startsWith("/system")) return "System";
  return "";
}

function getSelectedSettingsSub(path) {
  const settingsMap = {
    "/settings/general": "General",
    "/settings/radarr": "Radarr",
    "/settings/sonarr": "Sonarr",
    "/settings/plex": "Plex",
    "/settings/extras": "Extras",
    "/settings/ytdlp": "Ytdlp",
  };
  for (const [route, label] of Object.entries(settingsMap)) {
    if (path.startsWith(route)) return label;
  }
  return "";
}

function getSelectedSystemSub(path) {
  if (path.startsWith("/system/")) {
    if (path.startsWith("/system/status")) return "Status";
    if (path.startsWith("/system/tasks")) return "Tasks";
    if (path.startsWith("/system/logs")) return "Logs";
  }
  return "";
}

function getSelectedWantedSub(path) {
  if (path.startsWith("/wanted/")) {
    if (path.startsWith("/wanted/movies")) return "Movies";
    if (path.startsWith("/wanted/series")) return "Series";
  }
  return "";
}

export default function Sidebar({ mobile, open, onClose }) {
  const location = useLocation();
  const path = location.pathname;
  const selectedSection = getSelectedSection(path);
  const [healthCount, setHealthCount] = React.useState(0);

  React.useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await fetch("/api/system/status");
        if (!res.ok) return;
        const json = await res.json();
        if (cancelled) return;
        const hc = Array.isArray(json?.health) ? json.health.length : 0;
        const hasError = Array.isArray(json?.health)
          ? json.health.some(
              (h) =>
                (h.level || h.Level || "").toString().toLowerCase() === "error",
            )
          : false;
        setHealthCount(hc);
        setHasHealthError(hasError);
      } catch (e) {
        console.error("Failed to load system status for sidebar:", e);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [location.pathname]);
  const selectedSettingsSub = getSelectedSettingsSub(path);
  const selectedSystemSub = getSelectedSystemSub(path);
  const selectedWantedSub = getSelectedWantedSub(path);

  const [openMenus, setOpenMenus] = React.useState({});
  const [hasHealthError, setHasHealthError] = React.useState(false);
  React.useEffect(() => {
    let menuToOpen = null;
    if (selectedSection === "Wanted") menuToOpen = "Wanted";
    else if (selectedSection === "Settings") menuToOpen = "Settings";
    else if (selectedSection === "System") menuToOpen = "System";
    if (menuToOpen) {
      setOpenMenus({ [menuToOpen]: true });
    } else {
      setOpenMenus({});
    }
  }, [selectedSection]);

  const isOpen = (menu) => !!openMenus[menu];
  const handleToggle = (menu) => {
    setOpenMenus((prev) => {
      const isOpening = !prev[menu];
      const newState = {};
      if (isOpening) {
        newState[menu] = true;
      }
      return newState;
    });
  };

  if (mobile) {
    return (
      <SidebarMobile
        open={open}
        onClose={onClose}
        selectedSection={selectedSection}
        selectedSettingsSub={selectedSettingsSub}
        selectedWantedSub={selectedWantedSub}
        selectedSystemSub={selectedSystemSub}
        isOpen={isOpen}
        handleToggle={handleToggle}
        healthCount={healthCount}
        hasHealthError={hasHealthError}
      />
    );
  }
  return (
    <SidebarDesktop
      selectedSection={selectedSection}
      selectedSettingsSub={selectedSettingsSub}
      selectedWantedSub={selectedWantedSub}
      selectedSystemSub={selectedSystemSub}
      isOpen={isOpen}
      handleToggle={handleToggle}
      healthCount={healthCount}
      hasHealthError={hasHealthError}
    />
  );
}

Sidebar.propTypes = {
  mobile: PropTypes.bool,
  open: PropTypes.bool,
  onClose: PropTypes.func,
};
