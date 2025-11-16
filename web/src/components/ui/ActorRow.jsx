import React from "react";
import PropTypes from "prop-types";
import "../media/MediaInfo.css";

// ActorRow: horizontally scrollable row of actors, no wrapping
export default function ActorRow({ actors = [] }) {
  return (
    <div className="actor-row-scroll">
      {actors.map((actor) => (
        <div
          key={actor.id}
          style={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            minWidth: 56,
            scrollSnapAlign: "start",
          }}
        >
          {actor.profile_path ? (
            <img
              src={`https://image.tmdb.org/t/p/w185${actor.profile_path}`}
              alt={actor.name}
              style={{
                width: 56,
                height: 80,
                objectFit: "cover",
                borderRadius: 4,
                background: "#2222",
                marginBottom: 2,
              }}
              onError={(e) => {
                e.target.onerror = null;
                e.target.src = "/icons/logo.svg";
              }}
            />
          ) : (
            <div
              style={{
                width: 56,
                height: 80,
                background: "#4444",
                borderRadius: 4,
                marginBottom: 2,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <svg
                width="32"
                height="32"
                viewBox="0 0 32 32"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
              >
                <circle cx="16" cy="12" r="7" fill="#888" />
                <ellipse cx="16" cy="25" rx="11" ry="7" fill="#888" />
              </svg>
            </div>
          )}
          <span style={{ fontWeight: 500, fontSize: "0.68em", color: "#fff", whiteSpace: "nowrap", textOverflow: "ellipsis", overflow: "hidden", maxWidth: 80, textAlign: "center" }}>{actor.name}</span>
          <span style={{ fontSize: "0.60em", color: "#fff", whiteSpace: "nowrap", textOverflow: "ellipsis", overflow: "hidden", maxWidth: 80, textAlign: "center" }}>{actor.character}</span>
        </div>
      ))}
    </div>
  );
}

ActorRow.propTypes = {
  actors: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
      name: PropTypes.string,
      character: PropTypes.string,
      profile_path: PropTypes.string,
    }),
  ),
};
