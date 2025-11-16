import React from "react";
import PropTypes from "prop-types";

// Small reusable health/issues counter badge used in sidebar (desktop & mobile)
export default function HealthBadge({ count, style = {} }) {
  if (!count || count <= 0) return null;
  const display = count > 9 ? "9+" : String(count);
  const defaultStyle = {
    background: "#ef4444",
    color: "#fff",
    borderRadius: 3,
    width: 20,
    height: 20,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: "0.75em",
    lineHeight: 1,
    marginLeft: 8,
    textAlign: "center",
    boxSizing: "border-box",
  };
  return (
    <span
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
