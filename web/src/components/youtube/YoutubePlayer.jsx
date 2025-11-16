import React from "react";
import PropTypes from "prop-types";

export default function YoutubePlayer({ videoId }) {
  return (
    <iframe
      src={`https://www.youtube.com/embed/${videoId}`}
      title="YouTube video player"
      frameBorder="0"
      allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
      allowFullScreen
      loading="lazy"
      className="md-youtube-iframe"
      style={{ width: "100%", height: "100%" }}
    />
  );
}

YoutubePlayer.propTypes = { videoId: PropTypes.string.isRequired };
