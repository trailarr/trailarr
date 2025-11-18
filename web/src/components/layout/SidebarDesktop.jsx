import React from "react";
import PropTypes from "prop-types";
import IconButton from "../ui/IconButton.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Link, useNavigate } from "react-router-dom";
import {
  faCog,
  faFilm,
  faHistory,
  faStar,
  faBan,
  faServer,
} from "@fortawesome/free-solid-svg-icons";
import "./SidebarDesktop.css";
import { isDark } from "../../utils/isDark";
import HealthBadge from "../ui/HealthBadge.jsx";

export default function SidebarDesktop({
  selectedSection,
  selectedSettingsSub,
  selectedWantedSub,
  selectedSystemSub,
  isOpen,
  handleToggle,
  healthCount = 0,
  hasHealthError = false,
}) {
  // Generic submenu renderer used by Wanted, Settings and System
  function renderSubmenu(items, selectedSub, getRoute, opts = {}) {
    const { includeHealth = false, parentSelected = false } = opts;
    const ulBorderLeft = "3px solid transparent";
    return (
      <ul
        style={{
          listStyle: "none",
          padding: 0,
          margin: 0,
          background: "transparent",
          borderRadius: 0,
          color: isDark ? "#e5e7eb" : "#222",
          textAlign: "left",
          borderLeft: ulBorderLeft,
        }}
      >
        {items.map((submenu) => {
          const selected = selectedSub === submenu;
          let liBorderLeft;
          if (parentSelected) {
            liBorderLeft = "3px solid transparent";
          } else if (selected) {
            liBorderLeft = "3px solid #a855f7";
          } else {
            liBorderLeft = "3px solid transparent";
          }
          let color;
          if (selected) {
            color = isDark ? "#a855f7" : "#6d28d9";
          } else {
            color = isDark ? "#e5e7eb" : "#333";
          }
          const fontWeight = selected ? "bold" : "normal";
          const styleLink = {
            color,
            textDecoration: "none",
            display: "block",
            width: "100%",
            padding: "0em 1.8em",
            textAlign: "left",
            background: "none",
            border: "none",
            fontWeight,
            cursor: "pointer",
          };
          const toRoute = getRoute(submenu);
          const liStyle = {
            padding: "0.5em 1em",
            borderLeft: liBorderLeft,
            background: "none",
            color,
            fontWeight,
            cursor: "pointer",
            textAlign: "left",
            marginBottom: 0,
          };
          if (includeHealth && submenu === "Status") {
            Object.assign(liStyle, {
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            });
          }
          return (
            <li key={submenu} style={liStyle}>
              <Link
                to={toRoute}
                style={includeHealth ? { ...styleLink, flex: 1 } : styleLink}
              >
                {submenu}
              </Link>
              {includeHealth && submenu === "Status" && (
                <HealthBadge count={healthCount} hasError={hasHealthError} />
              )}
            </li>
          );
        })}
      </ul>
    );
  }
  const navigate = useNavigate();
  const navTimersRef = React.useRef({});
  const NAV_DELAY = 150;

  const firstSubmenuRoute = {
    Wanted: "/wanted/movies",
    Settings: "/settings/general",
    System: "/system/status",
  };

  const menuItems = [
    { name: "Movies", icon: faFilm, route: "/" },
    { name: "Series", icon: faCog, route: "/series" },
    { name: "History", icon: faHistory, route: "/history" },
    { name: "Wanted", icon: faStar },
    { name: "Blacklist", icon: faBan, route: "/blacklist" },
    { name: "Settings", icon: faCog },
    { name: "System", icon: faServer },
  ];

  const handleMenuClick = (name) => {
    if (navTimersRef.current[name]) {
      clearTimeout(navTimersRef.current[name]);
      delete navTimersRef.current[name];
    }
    if (firstSubmenuRoute[name]) {
      if (isOpen(name)) {
        navigate(firstSubmenuRoute[name]);
      } else {
        handleToggle(name);
        navTimersRef.current[name] = setTimeout(() => {
          navigate(firstSubmenuRoute[name]);
          delete navTimersRef.current[name];
        }, NAV_DELAY);
      }
    } else {
      handleToggle(name);
    }
  };
  return (
    <aside
      className="sidebar-desktop"
      style={{
        width: 220,
        background: isDark ? "#23232a" : "#fff",
        borderRight: isDark ? "1px solid #333" : "1px solid #e5e7eb",
        padding: "0em 0",
        height: "calc(100vh - 64px)",
        boxSizing: "border-box",
        position: "fixed",
        top: 64,
        left: 0,
        zIndex: 105,
      }}
    >
      <nav>
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {menuItems.map(({ name, icon, route }) => {
            let background, color, fontWeight;
            if (selectedSection === name) {
              background = isDark ? "#333" : "#f3f4f6";
              color = isDark ? "#a855f7" : "#6d28d9";
              fontWeight = "bold";
            } else {
              background = "none";
              color = isDark ? "#e5e7eb" : "#333";
              fontWeight = "normal";
            }
            const parentBorderLeft =
              selectedSection === name || isOpen(name)
                ? "3px solid #a855f7"
                : "3px solid transparent";
            const styleCommon = {
              textDecoration: "none",
              background,
              border: "none",
              color,
              fontWeight,
              textAlign: "left",
              padding: "0.5em 1em",
              borderRadius: 0,
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              gap: "0.75em",
              outline: "none",
              boxShadow: "none",
              WebkitTapHighlightColor: "transparent",
              transition: "box-shadow 0.1s",
            };
            if (route) {
              return (
                <li
                  key={name}
                  style={{ marginBottom: 0, borderLeft: parentBorderLeft }}
                >
                  <Link
                    to={route}
                    style={styleCommon}
                    className="sidebar-menu-link"
                  >
                    <IconButton
                      icon={
                        <FontAwesomeIcon
                          icon={icon}
                          color={isDark ? "#e5e7eb" : "#333"}
                        />
                      }
                      as="span"
                      style={{
                        background: "none",
                        padding: 0,
                        margin: 0,
                        border: "none",
                      }}
                    />
                    {name}
                  </Link>
                </li>
              );
            }
            return (
              <li
                key={name}
                style={{ marginBottom: 0, borderLeft: parentBorderLeft }}
              >
                <button
                  type="button"
                  style={{ ...styleCommon, width: "100%", borderRadius: 0 }}
                  className="sidebar-menu-btn"
                  onClick={() => handleMenuClick(name)}
                >
                  <IconButton
                    icon={
                      <FontAwesomeIcon
                        icon={icon}
                        color={isDark ? "#e5e7eb" : "#333"}
                      />
                    }
                    as="span"
                    style={{
                      background: "none",
                      padding: 0,
                      margin: 0,
                      border: "none",
                    }}
                  />
                  <span
                    style={{
                      flex: 1,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                    }}
                  >
                    <span>{name}</span>
                    {name === "System" &&
                      healthCount > 0 &&
                      !isOpen("System") && (
                        <HealthBadge count={healthCount} hasError={hasHealthError} />
                      )}
                  </span>
                </button>
                {name === "Wanted" &&
                  isOpen("Wanted") &&
                  renderSubmenu(
                    ["Movies", "Series"],
                    selectedWantedSub,
                    (s) => `/wanted/${s.toLowerCase()}`,
                    {
                      parentSelected: selectedSection === name || isOpen(name),
                    },
                  )}
                {name === "Settings" &&
                  isOpen("Settings") &&
                  renderSubmenu(
                    ["General", "Radarr", "Sonarr", "Plex", "Extras", "Ytdlp"],
                    selectedSettingsSub,
                    (s) => `/settings/${s.toLowerCase()}`,
                    {
                      parentSelected: selectedSection === name || isOpen(name),
                    },
                  )}
                {name === "System" &&
                  isOpen("System") &&
                  renderSubmenu(
                    ["Status", "Tasks", "Logs"],
                    selectedSystemSub,
                    (s) => {
                      if (s === "Status") return "/system/status";
                      if (s === "Tasks") return "/system/tasks";
                      return "/system/logs";
                    },
                    {
                      includeHealth: true,
                      parentSelected: selectedSection === name || isOpen(name),
                    },
                  )}
              </li>
            );
          })}
        </ul>
      </nav>
    </aside>
  );
}

SidebarDesktop.propTypes = {
  selectedSection: PropTypes.string.isRequired,
  selectedSettingsSub: PropTypes.string,
  selectedWantedSub: PropTypes.string,
  selectedSystemSub: PropTypes.string,
  isOpen: PropTypes.func.isRequired,
  handleToggle: PropTypes.func.isRequired,
  healthCount: PropTypes.number,
  hasHealthError: PropTypes.bool,
};
