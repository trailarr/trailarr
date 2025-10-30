import React from "react";
import PropTypes from "prop-types";
import "./ActionLane.css";
import { isDark } from "../utils/isDark";

/**
 * ActionLane: A parametric lane for consistent action bar UI.
 * @param {Array} buttons - Array of button configs: { icon, label, onClick, disabled, loading, showLabel, key }
 * @param {string} error - Optional error message
 * @param {React.ReactNode} children - Optional extra content
 */
export default function ActionLane({ buttons = [], error, children }) {
  const laneBg = isDark ? "#23232a" : "var(--save-lane-bg, #f3f4f6)";
  const laneText = isDark ? "#e5e7eb" : "var(--save-lane-text, #222)";
  return (
    <div
      className="media-action-lane"
      style={{
        position: "fixed",
        top: 64,
        left: 0,
        width: "100%",
        background: laneBg,
        color: laneText,
        padding: "1rem 0rem",
        display: "flex",
        flexDirection: "column",
        alignItems: "flex-start",
        gap: "0.7rem",
        zIndex: 100,
        boxShadow: isDark ? "0 2px 8px #0008" : "0 2px 8px #0001",
        borderBottom: isDark ? "1.5px solid #444" : "1.5px solid #e5e7eb",
        transition: "background 0.2s, color 0.2s",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
        {buttons.map(
          ({
            icon,
            label,
            onClick,
            disabled,
            loading,
            showLabel = true,
            key,
          }) => (
            <button
              key={key || label}
              onClick={onClick}
              disabled={disabled || loading}
              style={{
                background: "none",
                color: laneText,
                border: "none",
                padding: "0.3rem 1rem",
                cursor: disabled || loading ? "not-allowed" : "pointer",
                opacity: disabled || loading ? 0.7 : 1,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: "0.2rem",
              }}
            >
              {icon}
              {showLabel && (
                <span
                  style={{
                    fontWeight: 500,
                    fontSize: "0.85em",
                    color: laneText,
                    marginTop: 2,
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    lineHeight: 1.1,
                  }}
                >
                  {label}
                </span>
              )}
            </button>
          ),
        )}
      </div>
      {children}
      {error && (
        <div
          style={{
            marginLeft: 16,
            color: error.toLowerCase().includes("success") ? "#0a0" : "#f44",
            fontWeight: 500,
          }}
        >
          {error}
        </div>
      )}
    </div>
  );
}

ActionLane.propTypes = {
  buttons: PropTypes.arrayOf(
    PropTypes.shape({
      icon: PropTypes.node.isRequired,
      label: PropTypes.string,
      onClick: PropTypes.func,
      disabled: PropTypes.bool,
      loading: PropTypes.bool,
      showLabel: PropTypes.bool,
      key: PropTypes.string,
    }),
  ),
  error: PropTypes.string,
  children: PropTypes.node,
};
