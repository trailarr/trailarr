import React from "react";
import PropTypes from "prop-types";
import IconButton from "./IconButton.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Link } from "react-router-dom";
import {
  faCog,
  faFilm,
  faHistory,
  faStar,
  faBan,
  faServer,
} from "@fortawesome/free-solid-svg-icons";
import { isDark } from "../utils/isDark";
import "./SidebarMobile.css";
import HealthBadge from "./HealthBadge.jsx";

export default function SidebarMobile({
  open,
  onClose,
  selectedSection,
  selectedSettingsSub,
  selectedSystemSub,
  isOpen,
  handleToggle,
  healthCount = 0,
}) {
  function renderSubmenu(items, selectedSub, getRoute, opts = {}) {
    const { includeHealth = false, parentSelected = false } = opts;
    // Keep the submenu UL border transparent — the parent <li> now owns the
    // continuous purple border so the UL must not draw its own border to
    // avoid a double line on mobile.
    const ulBorderLeft = "3px solid transparent";

    const baseLinkStyle = {
      textDecoration: "none",
      display: "block",
      width: "100%",
      textAlign: "left",
      background: "none",
      border: "none",
      cursor: "pointer",
    };

    const renderItem = (submenu) => {
      const selected = selectedSub === submenu;
      const isStatusWithHealth = includeHealth && submenu === "Status";
      const liBorderLeft =
        selected && !parentSelected
          ? "3px solid #a855f7"
          : "3px solid transparent";
      let color;
      if (selected) {
        color = isDark ? "#a855f7" : "#6d28d9";
      } else {
        color = isDark ? "#e5e7eb" : "#333";
      }
      const fontWeight = selected ? "bold" : "normal";
      const toRoute = getRoute(submenu);

      const styleLink = {
        ...baseLinkStyle,
        color,
        fontWeight,
      };

      const liStyle = {
        padding: "0.5em 1em",
        borderLeft: liBorderLeft,
        background: "none",
        color,
        fontWeight,
        cursor: "pointer",
        textAlign: "left",
        marginBottom: 0,
        display: isStatusWithHealth ? "flex" : undefined,
        alignItems: isStatusWithHealth ? "center" : undefined,
        justifyContent: isStatusWithHealth ? "space-between" : undefined,
      };

      return (
        <li key={submenu} style={liStyle}>
          <Link
            to={toRoute}
            style={isStatusWithHealth ? { ...styleLink, flex: 1 } : styleLink}
            onClick={onClose}
          >
            {submenu}
          </Link>
          {isStatusWithHealth && <HealthBadge count={healthCount} />}
        </li>
      );
    };

    return (
      <ul
        style={{
          listStyle: "none",
          padding: 0,
          margin: 0,
          borderLeft: ulBorderLeft,
          background: "transparent",
          borderRadius: 0,
          color: isDark ? "#e5e7eb" : "#222",
          textAlign: "left",
        }}
      >
        {items.map(renderItem)}
      </ul>
    );
  }
  const menuItems = [
    { name: "Movies", icon: faFilm, route: "/" },
    { name: "Series", icon: faCog, route: "/series" },
    { name: "History", icon: faHistory, route: "/history" },
    { name: "Wanted", icon: faStar },
    { name: "Blacklist", icon: faBan, route: "/blacklist" },
    { name: "Settings", icon: faCog },
    { name: "System", icon: faServer },
  ];
  return (
    <>
      {open && (
        <button
          type="button"
          className="sidebar-mobile__backdrop"
          onClick={onClose}
          aria-label="Close sidebar"
          style={{
            background: "transparent",
            border: "none",
            padding: 0,
            margin: 0,
            position: "fixed",
            top: 0,
            left: 0,
            width: "100vw",
            height: "100vh",
            zIndex: 1000,
          }}
        />
      )}
      <div
        className={`sidebar-mobile${open ? " open" : ""}`}
        style={{
          "--sidebar-bg": isDark ? "#23232a" : "#fff",
          position: "fixed",
          zIndex: 1001,
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
                // borderLeft moved to the li wrapper so submenu and parent share a single continuous border
                color,
                fontWeight,
                width: "100%",
                minHeight: 50,
                boxSizing: "border-box",
                textAlign: "left",
                padding: "0.5em 1em",
                borderRadius: 0,
                cursor: "pointer",
                display: "flex",
                alignItems: "center",
                gap: "0.75em",
              };
              if (route) {
                return (
                  <li
                    key={name}
                    style={{ marginBottom: 0, borderLeft: parentBorderLeft }}
                  >
                    <Link to={route} style={styleCommon} onClick={onClose}>
                      <IconButton
                        icon={
                          <FontAwesomeIcon
                            icon={icon}
                            color={isDark ? "#e5e7eb" : "#333"}
                          />
                        }
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
              // Render menu toggle and submenus
              return (
                <li
                  key={name}
                  style={{ marginBottom: 0, borderLeft: parentBorderLeft }}
                >
                  <button
                    type="button"
                    style={styleCommon}
                    onClick={() => handleToggle(name)}
                  >
                    <IconButton
                      icon={
                        <FontAwesomeIcon
                          icon={icon}
                          color={isDark ? "#e5e7eb" : "#333"}
                        />
                      }
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
                          <HealthBadge count={healthCount} />
                        )}
                    </span>
                  </button>
                  {name === "Wanted" &&
                    isOpen("Wanted") &&
                    renderSubmenu(
                      ["Movies", "Series"],
                      selectedSettingsSub,
                      (s) => `/wanted/${s.toLowerCase()}`,
                      {
                        parentSelected:
                          selectedSection === name || isOpen(name),
                      },
                    )}
                  {name === "Settings" &&
                    isOpen("Settings") &&
                    renderSubmenu(
                      ["General", "Radarr", "Sonarr", "Extras"],
                      selectedSettingsSub,
                      (s) => `/settings/${s.toLowerCase()}`,
                      {
                        parentSelected:
                          selectedSection === name || isOpen(name),
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
                        parentSelected:
                          selectedSection === name || isOpen(name),
                      },
                    )}
                </li>
              );
            })}
          </ul>
        </nav>
      </div>
    </>
  );
}

SidebarMobile.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  selectedSection: PropTypes.string.isRequired,
  selectedSettingsSub: PropTypes.string,
  selectedSystemSub: PropTypes.string,
  isOpen: PropTypes.func.isRequired,
  handleToggle: PropTypes.func.isRequired,
  healthCount: PropTypes.number,
};
