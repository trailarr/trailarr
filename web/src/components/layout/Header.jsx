import React from "react";
import IconButton from "../ui/IconButton.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBars } from "@fortawesome/free-solid-svg-icons";
import PropTypes from "prop-types";
import { isDark } from "../../utils/isDark";

export default function Header({
  search,
  setSearch,
  mobile,
  sidebarOpen,
  onSidebarToggle,
}) {
  return (
    <header
      style={{
        width: "100%",
        height: 64,
        background: isDark ? "#23232a" : "#fff",
        display: "flex",
        alignItems: "center",
        boxShadow: isDark ? "0 1px 4px #222" : "0 1px 4px #e5e7eb",
        padding: "0 24px",
        position: "fixed",
        top: 0,
        left: 0,
        zIndex: 110,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 16, flex: 1 }}>
        <img
          src="/icons/logo.svg"
          alt="Logo"
          style={{
            width: mobile ? 28 : 40,
            height: mobile ? 28 : 40,
            marginRight: mobile ? 0 : 12,
          }}
        />
        {mobile && (
          <IconButton
            icon={
              <FontAwesomeIcon
                icon={faBars}
                color={isDark ? "#e5e7eb" : "#23232a"}
              />
            }
            onClick={onSidebarToggle}
            style={{ marginLeft: 8, fontSize: 24 }}
            title={sidebarOpen ? "Close menu" : "Open menu"}
          />
        )}
        {!mobile && (
          <span
            style={{
              fontWeight: "bold",
              fontSize: 22,
              color: isDark ? "#e5e7eb" : "#23232a",
              letterSpacing: 0.5,
            }}
          >
            Trailarr
          </span>
        )}
      </div>
      <div
        style={{
          position: "absolute",
          right: 0,
          top: 0,
          height: "100%",
          display: "flex",
          alignItems: "center",
          paddingRight: 72,
        }}
      >
        <div style={{ position: "relative", width: 200 }}>
          <input
            type="search"
            name="search"
            placeholder="Search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              padding: "0.5em 0.5em 0.5em 2em",
              borderRadius: 6,
              border: "1px solid #e5e7eb",
              width: "100%",
              textAlign: "left",
              color: isDark ? "#e5e7eb" : "#222",
              background: isDark ? "#23232a" : "#fff",
              boxSizing: "border-box",
            }}
          />
          <span
            style={{
              position: "absolute",
              left: 8,
              top: "50%",
              transform: "translateY(-50%)",
              pointerEvents: "none",
              color: isDark ? "#e5e7eb" : "#888",
              fontSize: 16,
              display: "flex",
              alignItems: "center",
            }}
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 20 20"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
            >
              <circle
                cx="9"
                cy="9"
                r="7"
                stroke="currentColor"
                strokeWidth="2"
              />
              <line
                x1="15"
                y1="15"
                x2="19"
                y2="19"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
            </svg>
          </span>
        </div>
      </div>
    </header>
  );
}
Header.propTypes = {
  search: PropTypes.string.isRequired,
  setSearch: PropTypes.func.isRequired,
  mobile: PropTypes.bool.isRequired,
  sidebarOpen: PropTypes.bool.isRequired,
  onSidebarToggle: PropTypes.func.isRequired,
};
