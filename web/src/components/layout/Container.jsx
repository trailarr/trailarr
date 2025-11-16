import React from "react";
import PropTypes from "prop-types";
import { isDark } from "../../utils/isDark";

export default function Container({ children, style = {}, ...props }) {
  const defaultStyle = {
    width: "100%",
    margin: 0,
    height: "100%",
    padding: "0",
    background: isDark ? "#23232a" : "#f3f4f6",
    borderRadius: 0,
    boxShadow: "0 2px 12px #0002",
    color: isDark ? "#f3f4f6" : "#23232a",
    boxSizing: "border-box",
    overflowX: "hidden",
    overflowY: "auto",
    position: "relative",
    ...style,
  };
  return (
    <div style={defaultStyle} {...props}>
      {children}
    </div>
  );
}

Container.propTypes = {
  children: PropTypes.node,
  style: PropTypes.object,
};
