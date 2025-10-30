import React from "react";
import PropTypes from "prop-types";

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error, info) {
    console.error("ErrorBoundary caught error:", error, info);
  }

  render() {
    if (this.state.hasError) {
      const msg = this.props.message || "Failed to load component";
      return (
        <div style={{ padding: 48 }}>
          <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 8 }}>
            {msg}
          </div>
          <div>
            <button
              onClick={() => globalThis.location.reload()}
              style={{ padding: "8px 12px", cursor: "pointer" }}
            >
              Reload page
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

ErrorBoundary.propTypes = {
  message: PropTypes.string,
  children: PropTypes.node.isRequired,
};

export default ErrorBoundary;
