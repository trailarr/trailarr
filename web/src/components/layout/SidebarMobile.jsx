import React from "react";
import PropTypes from "prop-types";
import IconButton from "../ui/IconButton.jsx";
import HealthBadge from "../ui/HealthBadge.jsx";
import { Link } from "react-router-dom";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faChevronLeft } from "@fortawesome/free-solid-svg-icons";
import { isDark } from "../../utils/isDark";
import "./SidebarMobile.css";

export default function SidebarMobile({
  open,
  onClose,
  selectedSection,
  selectedSettingsSub,
  selectedWantedSub,
  selectedSystemSub,
  isOpen,
  handleToggle,
  healthCount = 0,
  hasHealthError = false,
}) {
  const closeStyle = {
    display: "flex",
    alignItems: "center",
    gap: 12,
    padding: "0.75em",
    marginBottom: 8,
  };
  const style = {
    padding: "0.5em 0.5em 0.5em 1em",
    width: "100%",
    border: "none",
    textAlign: "left",
    display: "flex",
    alignItems: "center",
    gap: 8,
    cursor: "pointer",
    background: "none",
  };
  const linkStyle = (isActive) => ({
    ...style,
    fontWeight: isActive ? "bold" : "normal",
  });
  if (!open) return null;
  return (
    <div className="sidebar-mobile">
      <div style={closeStyle}>
        <IconButton
          icon={
            <FontAwesomeIcon
              icon={faChevronLeft}
              color={isDark ? "#e5e7eb" : "#23232a"}
            />
          }
          onClick={onClose}
          title="Close"
        />
        <span style={{ fontWeight: 700 }}>Menu</span>
      </div>
      <nav>
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          <li>
            <Link
              to="/"
              onClick={onClose}
              style={linkStyle(selectedSection === "Movies")}
            >
              Movies
            </Link>
          </li>
          <li>
            <Link
              to="/series"
              onClick={onClose}
              style={linkStyle(selectedSection === "Series")}
            >
              Series
            </Link>
          </li>
          <li>
            <Link to="/history" onClick={onClose} style={style}>
              History
            </Link>
          </li>
          <li>
            <button
              type="button"
              style={style}
              onClick={() => handleToggle("Wanted")}
            >
              Wanted
            </button>
            {isOpen("Wanted") && (
              <ul style={{ listStyle: "none", paddingLeft: 12 }}>
                <li>
                  <Link
                    to="/wanted/movies"
                    onClick={onClose}
                    style={linkStyle(selectedWantedSub === "Movies")}
                  >
                    Movies
                  </Link>
                </li>
                <li>
                  <Link
                    to="/wanted/series"
                    onClick={onClose}
                    style={linkStyle(selectedWantedSub === "Series")}
                  >
                    Series
                  </Link>
                </li>
              </ul>
            )}
          </li>
          <li>
            <button
              type="button"
              style={style}
              onClick={() => handleToggle("Settings")}
            >
              Settings
            </button>
            {isOpen("Settings") && (
              <ul style={{ listStyle: "none", paddingLeft: 12 }}>
                <li>
                  <Link
                    to="/settings/general"
                    onClick={onClose}
                    style={linkStyle(selectedSettingsSub === "General")}
                  >
                    General
                  </Link>
                </li>
                <li>
                  <Link
                    to="/settings/extras"
                    onClick={onClose}
                    style={linkStyle(selectedSettingsSub === "Extras")}
                  >
                    Extras
                  </Link>
                </li>
                <li>
                  <Link
                    to="/settings/ytdlp"
                    onClick={onClose}
                    style={linkStyle(selectedSettingsSub === "Ytdlp")}
                  >
                    Ytdlp
                  </Link>
                </li>
              </ul>
            )}
          </li>
          <li>
            <button
              type="button"
              style={style}
              onClick={() => handleToggle("System")}
            >
              System
            </button>
            {isOpen("System") && (
              <ul style={{ listStyle: "none", paddingLeft: 12 }}>
                <li>
                  <Link
                    to="/system/status"
                    onClick={onClose}
                    style={linkStyle(selectedSystemSub === "Status")}
                  >
                    Status
                  </Link>
                </li>
                <li>
                  <Link
                    to="/system/tasks"
                    onClick={onClose}
                    style={linkStyle(selectedSystemSub === "Tasks")}
                  >
                    Tasks
                  </Link>
                </li>
                <li>
                  <Link
                    to="/system/logs"
                    onClick={onClose}
                    style={linkStyle(selectedSystemSub === "Logs")}
                  >
                    Logs
                  </Link>
                </li>
              </ul>
            )}
          </li>
          <li>
            <Link
              to="/blacklist"
              onClick={onClose}
              style={linkStyle(selectedSection === "Blacklist")}
            >
              Blacklist
            </Link>
          </li>
        </ul>
      </nav>
      <div style={{ padding: "1em 0.5em" }}>
        <div>
          Health Issues:{" "}
          <HealthBadge count={healthCount} hasError={hasHealthError} />
        </div>
      </div>
    </div>
  );
}

SidebarMobile.propTypes = {
  open: PropTypes.bool,
  onClose: PropTypes.func,
  selectedSection: PropTypes.string,
  selectedSettingsSub: PropTypes.string,
  selectedWantedSub: PropTypes.string,
  selectedSystemSub: PropTypes.string,
  isOpen: PropTypes.func,
  handleToggle: PropTypes.func,
  healthCount: PropTypes.number,
  hasHealthError: PropTypes.bool,
};
