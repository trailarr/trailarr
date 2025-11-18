import React from "react";
import PropTypes from "prop-types";
import "./HealthBadge.css";

// Small reusable health/issues counter badge used in sidebar (desktop & mobile)
export default function HealthBadge({ count, style = {}, hasError = false, className = "" }) {
  if (!count || count <= 0) return null;
  const display = count > 9 ? "9+" : String(count);
  const defaultStyle = {
    background: hasError ? "#ef4444" : "#f59e0b",
    color: "#fff",
    borderRadius: 4,
    width: 20,
    height: 20,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: "12px",
    lineHeight: "20px",
    marginLeft: 8,
    textAlign: "center",
    boxSizing: "border-box",
  };
  return (
    <span
      className={`health-badge ${className}`}
      style={{ ...defaultStyle, ...style }}
      aria-label={`${count} health issues`}
    >
      {display}
    </span>
  );
}

HealthBadge.propTypes = {
  count: PropTypes.number.isRequired,
  style: PropTypes.object,
};
