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
  if (path.startsWith("/wanted/")) {
    if (path.startsWith("/wanted/movies")) return "Movies";
    if (path.startsWith("/wanted/series")) return "Series";
    return "Movies";
  }
  if (path.startsWith("/settings/")) {
    if (path.startsWith("/settings/general")) return "General";
    if (path.startsWith("/settings/radarr")) return "Radarr";
    if (path.startsWith("/settings/sonarr")) return "Sonarr";
    if (path.startsWith("/settings/extras")) return "Extras";
    return "General";
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

export default function Sidebar({ mobile, open, onClose }) {
  const location = useLocation();
  const path = location.pathname;
  const selectedSection = getSelectedSection(path);
  const [healthCount, setHealthCount] = React.useState(0);

  // Fetch minimal system status to show counters (e.g. health issue count) in the sidebar
  React.useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await fetch("/api/system/status");
        if (!res.ok) return;
        const json = await res.json();
        if (cancelled) return;
        const hc = Array.isArray(json?.health) ? json.health.length : 0;
        setHealthCount(hc);
      } catch (e) {
        // Log errors - sidebar counters are non-critical
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

  // Local state for submenu expansion
  const [openMenus, setOpenMenus] = React.useState({});
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
        selectedSystemSub={selectedSystemSub}
        isOpen={isOpen}
        handleToggle={handleToggle}
        healthCount={healthCount}
      />
    );
  }
  return (
    <SidebarDesktop
      selectedSection={selectedSection}
      selectedSettingsSub={selectedSettingsSub}
      selectedSystemSub={selectedSystemSub}
      isOpen={isOpen}
      handleToggle={handleToggle}
      healthCount={healthCount}
    />
  );
}

Sidebar.propTypes = {
  mobile: PropTypes.bool,
  open: PropTypes.bool,
  onClose: PropTypes.func,
};
