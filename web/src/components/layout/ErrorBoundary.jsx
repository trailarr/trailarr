import React from "react";
import PropTypes from "prop-types";

function ErrorBoundary({ children }) {
  return <>{children}</>;
}

ErrorBoundary.propTypes = {
  children: PropTypes.node.isRequired,
};

export default ErrorBoundary;
