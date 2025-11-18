import React, { useEffect, useRef } from "react";
import PropTypes from "prop-types";
import YoutubePlayer from "./YoutubePlayer.jsx";

// Accessible modal dedicated to Youtube playback. Handles Escape key,
// backdrop activation, and moves focus into the dialog when opened.
export default function YoutubeModal({ open, videoId, onClose }) {
  const closeBtnRef = useRef(null);
  const contentRef = useRef(null);

  useEffect(() => {
    if (!open) return;
    // Focus the close button when modal opens for keyboard users
    if (closeBtnRef.current && typeof closeBtnRef.current.focus === "function") {
      closeBtnRef.current.focus();
    }
    const handleKey = (e) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open, onClose]);

  if (!open || !videoId) return null;

  return (
    <div
      aria-modal="true"
      aria-label="YouTube modal dialog"
      className="md-youtube-modal-backdrop youtube-modal-backdrop"
      role="dialog"
    >
      {/* Invisible full-viewport button to provide a real interactive target
          for backdrop clicks/keyboard activation. */}
      <button
        type="button"
        aria-label="Close YouTube modal"
        onClick={onClose}
        ref={closeBtnRef}
        className="md-youtube-modal-backdrop-button"
        style={{
          position: "absolute",
          inset: 0,
          width: "100%",
          height: "100%",
          border: 0,
          background: "transparent",
          padding: 0,
          margin: 0,
        }}
      />

      <div className="md-youtube-modal-content" role="document" ref={contentRef}>
        <YoutubePlayer videoId={videoId} />
      </div>
    </div>
  );
}

YoutubeModal.propTypes = {
  open: PropTypes.bool.isRequired,
  videoId: PropTypes.string.isRequired,
  onClose: PropTypes.func.isRequired,
};
