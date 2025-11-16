import React from "react";
import PropTypes from "prop-types";
import "./MediaInfo.css";

export default function MediaInfoLane({ children }) {
  return <div className="media-info-lane">{children}</div>;
}

MediaInfoLane.propTypes = {
  children: PropTypes.node,
};
