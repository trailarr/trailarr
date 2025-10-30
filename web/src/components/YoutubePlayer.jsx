import React, { useEffect, useRef, useState } from "react";
import PropTypes from "prop-types";

// Loads the YouTube IFrame API if not already loaded
let ytApiPromise = null;
function loadYouTubeAPI() {
  if (globalThis?.YT?.Player) return Promise.resolve();
  if (ytApiPromise) return ytApiPromise;
  ytApiPromise = new Promise((resolve) => {
    if (globalThis?.YT?.Player) return resolve();
    const tag = document.createElement("script");
    tag.src = "https://www.youtube.com/iframe_api";
    // expose the readiness callback on globalThis (preferred over window)
    globalThis.onYouTubeIframeAPIReady = () => resolve();
    document.body.appendChild(tag);
  });
  return ytApiPromise;
}

export default function YoutubePlayer({ videoId, onReady }) {
  const playerRef = useRef();
  const ytPlayer = useRef();
  const [error, setError] = useState("");

  useEffect(() => {
    let destroyed = false;
    let pollId;
    let playerCreated = false;
    let timeoutId;
    setError("");
    function tryCreatePlayer() {
      if (destroyed || playerCreated) return;
      if (!videoId) {
        setError("No videoId provided.");
        return;
      }
      if (globalThis?.YT?.Player && playerRef.current) {
        playerCreated = true;
        try {
          ytPlayer.current = new globalThis.YT.Player(playerRef.current, {
            videoId,
            events: {
              onReady: (event) => {
                event.target.playVideo();
                if (onReady) onReady(event);
              },
              onError: (e) => {
                // YouTube error 150: embedding not allowed
                if (e.data === 150) {
                  setError(""); // Do not show error, do not hide
                } else {
                  setError("YouTube Player error: " + JSON.stringify(e.data));
                }
                console.error("YouTube Player error", e);
              },
            },
            playerVars: {
              autoplay: 1,
              rel: 0,
              modestbranding: 1,
            },
          });
          console.log("YouTube Player created for", videoId);
        } catch (err) {
          setError("Failed to create YouTube Player: " + err.message);
          console.error("Failed to create YouTube Player", err);
        }
      } else {
        pollId = setTimeout(tryCreatePlayer, 50);
      }
    }
    loadYouTubeAPI().then(() => {
      console.log("YouTube IFrame API loaded");
      tryCreatePlayer();
    });
    // Fallback: if not created after 5s, show error
    timeoutId = setTimeout(() => {
      if (!playerCreated && !destroyed) {
        setError("Failed to create YouTube Player after 5 seconds.");
      }
    }, 5000);
    return () => {
      destroyed = true;
      clearTimeout(pollId);
      clearTimeout(timeoutId);
      if (ytPlayer.current) {
        ytPlayer.current.destroy();
        ytPlayer.current = null;
      }
    };
  }, [videoId, onReady]);

  return (
    <div
      style={{
        position: "relative",
        width: "80vw",
        height: "45vw",
        maxWidth: 900,
        maxHeight: 506,
      }}
    >
      <div
        ref={playerRef}
        style={{
          width: "100%",
          height: "100%",
          background: "#000",
          borderRadius: 12,
          boxShadow: "0 2px 24px #000",
          border: "none",
          display: "block",
        }}
      />
      {error && (
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            width: "100%",
            height: "100%",
            background: "rgba(0,0,0,0.85)",
            color: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderRadius: 12,
            zIndex: 2,
            fontSize: 18,
            textAlign: "center",
            padding: 24,
          }}
        >
          {error}
        </div>
      )}
    </div>
  );
}

YoutubePlayer.propTypes = {
  videoId: PropTypes.string.isRequired,
  onReady: PropTypes.func,
};
