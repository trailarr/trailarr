import React from "react";
import PropTypes from "prop-types";
import { isDark } from "../../utils/isDark";

export default function SectionHeader({ children, style = {}, ...props }) {
  const headerColor = isDark ? "#eee" : "#222";
  const defaultStyle = {
    fontWeight: 600,
    fontSize: "1.35em",
    margin: "0 0 18px 8px",
    textAlign: "left",
    textTransform: "capitalize",
    color: headerColor,
    ...style,
  };
  return (
    <h3 style={defaultStyle} {...props}>
      {children}
    </h3>
  );
}

SectionHeader.propTypes = {
  children: PropTypes.node,
  style: PropTypes.object,
};
