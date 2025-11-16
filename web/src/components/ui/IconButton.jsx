import React from "react";
import PropTypes from "prop-types";

export default function IconButton({
  icon,
  onClick,
  title,
  disabled = false,
  style = {},
  as = "button",
  ...props
}) {
  // Render a <button> by default. If `as` is provided and not "button",
  // render a non-interactive element with role="button" and keyboard support.
  if (as === "button") {
    return (
      <button
        onClick={onClick}
        title={title}
        disabled={disabled}
        style={{
          background: "none",
          border: "none",
          outline: "none",
          boxShadow: "none",
          padding: 0,
          margin: 0,
          cursor: disabled ? "not-allowed" : "pointer",
          opacity: disabled ? 0.6 : 1,
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          ...style,
        }}
        onFocus={(e) => {
          e.target.style.outline = "none";
          e.target.style.boxShadow = "none";
        }}
        onMouseDown={(e) => {
          e.target.style.outline = "none";
          e.target.style.boxShadow = "none";
        }}
        {...props}
      >
        {icon}
      </button>
    );
  }

  // Render a span with role=button when nested inside a button to avoid invalid HTML.
  const handleKeyDown = (e) => {
    if (disabled) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick?.(e);
    }
  };
  return (
    <span
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-disabled={disabled}
      onClick={disabled ? undefined : onClick}
      onKeyDown={handleKeyDown}
      title={title}
      style={{
        background: "none",
        border: "none",
        outline: "none",
        boxShadow: "none",
        padding: 0,
        margin: 0,
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.6 : 1,
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        ...style,
      }}
      {...props}
    >
      {icon}
    </span>
  );
}

IconButton.propTypes = {
  icon: PropTypes.node.isRequired,
  onClick: PropTypes.func.isRequired,
  title: PropTypes.string,
  disabled: PropTypes.bool,
  style: PropTypes.object,
};
