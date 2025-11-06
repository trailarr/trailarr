import React, { useState, useEffect, useRef } from "react";
import MediaInfoLane from "./MediaInfoLane.jsx";
import ActionLane from "./ActionLane.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSearch, faSpinner } from "@fortawesome/free-solid-svg-icons";
import ExtrasList from "./ExtrasList";
import YoutubeModal from "./YoutubeModal.jsx";
import Container from "./Container.jsx";
import Toast from "./Toast.jsx";
import "./MediaDetails.css";
import { useParams } from "react-router-dom";
import PropTypes from "prop-types";
import { getExtras } from "../api";
import { searchYoutubeStream } from "../api.youtube.sse";
import { isDark } from "../utils/isDark.js";

// Top-level WebSocket message handler for extras queue updates
function handleExtrasQueueUpdate(msg, mediaId, setExtras, setError) {
  if (msg.type === "download_queue_update" && Array.isArray(msg.queue)) {
    setExtras((prev) =>
      prev.map((ex) =>
        updateExtraWithQueueStatus(ex, msg.queue, mediaId, setError),
      ),
    );
  }
}

// Helper to update extras with queue status (moved to outer scope)
function updateExtraWithQueueStatus(ex, queue, mediaId, setError) {
  const found = queue.find(
    (q) => q.MediaId == mediaId && q.YouTubeID === ex.YoutubeId,
  );
  if (found?.Status) {
    // Only show toast if status transitions to 'failed' or 'rejected'
    if (
      (found.Status === "failed" || found.Status === "rejected") &&
      (found.reason || found.Reason) &&
      ex.Status !== found.Status
    ) {
      setError(found.reason || found.Reason);
    }
    return {
      ...ex,
      Status: found.Status,
      reason: found.reason || found.Reason,
      Reason: found.reason || found.Reason,
    };
  }
  return ex;
}
// Helper to convert YouTube search results to extras format for Trailers
function ytResultsToExtras(ytResults) {
  return ytResults
    .map((item) => ({
      YoutubeId: item.id?.videoId || "",
      ExtraType: "Trailers",
      ExtraTitle: item.snippet?.title || "YouTube Trailer",
      Status: "", // Not downloaded yet
      Thumb: item.snippet?.thumbnails?.medium?.url || "",
      ChannelTitle: item.snippet?.channelTitle || "",
      PublishedAt: item.snippet?.publishedAt || "",
      Description: item.snippet?.description || "",
      reason: "",
      Reason: "",
      Source: "YouTubeSearch",
      // Add all fields that ExtraCard expects, with safe defaults
      Downloaded: false,
      Exists: false,
      // ...add more if needed
    }))
    .filter((e) => e.YoutubeId);
}

function Spinner() {
  return (
    <div className="md-spinner-overlay">
      <svg
        width="48"
        height="48"
        viewBox="0 0 48 48"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <circle
          cx="24"
          cy="24"
          r="20"
          stroke="#a855f7"
          strokeWidth="4"
          opacity="0.2"
        />
        <path
          d="M44 24c0-11.046-8.954-20-20-20"
          stroke="#a855f7"
          strokeWidth="4"
          strokeLinecap="round"
        />
      </svg>
    </div>
  );
}

function YoutubeEmbed({ videoId }) {
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    setLoading(true);
    console.log("YoutubeEmbed mounted", videoId);
    return () => {
      console.log("YoutubeEmbed unmounted", videoId);
    };
  }, [videoId]);
  return (
    <div className="md-youtube-embed">
      {loading && <Spinner />}
      <iframe
        src={`https://www.youtube.com/embed/${videoId}`}
        title="YouTube video player"
        frameBorder="0"
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowFullScreen
        loading="lazy"
        className="md-youtube-iframe"
        onLoad={() => setLoading(false)}
      />
    </div>
  );
}
YoutubeEmbed.propTypes = {
  videoId: PropTypes.string.isRequired,
};

export default function MediaDetails({ mediaItems, loading, mediaType }) {
  const { id } = useParams();
  const media = mediaItems.find((m) => String(m.id) === id);

  // Mobile detection local to this component (affects skeleton layout)
  const [isMobile, setIsMobile] = useState(
    globalThis.window ? window.innerWidth < 900 : false,
  );
  useEffect(() => {
    const onResize = () => setIsMobile(window.innerWidth < 900);
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  // --- Cast state and fetch logic moved from MediaInfoLane ---
  const [cast, setCast] = useState([]);
  const [castLoading, setCastLoading] = useState(false);
  const [castError, setCastError] = useState("");

  // Fetch cast when media or mediaType changes
  useEffect(() => {
    if (!media?.id || !mediaType) {
      setCast([]);
      setCastError("");
      return;
    }
    setCastLoading(true);
    setCastError("");
    let url = "";
    if (mediaType === "movie") {
      url = `/api/movies/${media.id}/cast`;
    } else if (mediaType === "series" || mediaType === "tv") {
      url = `/api/series/${media.id}/cast`;
    } else {
      setCast([]);
      setCastError("Unknown media type");
      setCastLoading(false);
      return;
    }
    fetch(url)
      .then((res) => {
        if (!res.ok) throw new Error("Failed to fetch cast");
        return res.json();
      })
      .then((data) => {
        setCast(Array.isArray(data.cast) ? data.cast : []);
        setCastLoading(false);
      })
      .catch(() => {
        setCast([]);
        setCastError("Failed to load cast");
        setCastLoading(false);
      });
  }, [media, mediaType]);
  // Scroll to top when id (route) changes
  useEffect(() => {
    const tid = setTimeout(() => {
      // Try window scroll
      globalThis.window.scrollTo({ top: 0, left: 0, behavior: "auto" });
      // Try scrolling main container if present
      const main = globalThis.document.querySelector("main");
      if (main && typeof main.scrollTo === "function") {
        main.scrollTo({ top: 0, left: 0, behavior: "auto" });
      }
      // Try to find the first scrollable container
      const all = globalThis.document.querySelectorAll("body *");
      for (let el of all) {
        const style = globalThis.window.getComputedStyle(el);
        if (
          (style.overflowY === "auto" || style.overflowY === "scroll") &&
          el.scrollHeight > el.clientHeight
        ) {
          el.scrollTo({ top: 0, left: 0, behavior: "auto" });
          break;
        }
      }
    }, 0);
    return () => clearTimeout(tid);
  }, [id]);
  const [youtubeModal, setYoutubeModal] = useState({
    open: false,
    videoId: "",
  });
  // Store YouTube search results for merging into Trailers group
  const [ytResults, setYtResults] = useState([]);

  // Close modal on outside click or Escape
  useEffect(() => {
    if (!youtubeModal.open) return;
    const handleKey = (e) => {
      if (e.key === "Escape") setYoutubeModal({ open: false, videoId: "" });
    };
    const handleClick = (e) => {
      if (e.target.classList.contains("youtube-modal-backdrop"))
        setYoutubeModal({ open: false, videoId: "" });
    };
    globalThis.window.addEventListener("keydown", handleKey);
    globalThis.window.addEventListener("mousedown", handleClick);
    return () => {
      globalThis.window.removeEventListener("keydown", handleKey);
      globalThis.window.removeEventListener("mousedown", handleClick);
    };
  }, [youtubeModal.open]);
  // (removed duplicate declaration)
  const [extras, setExtras] = useState([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [error, setError] = useState("");
  const [modalMsg, setModalMsg] = useState("");
  const [showModal, setShowModal] = useState(false);
  useEffect(() => {
    if (!media) return;
    setSearchLoading(true);
    setError("");
    getExtras({ mediaType, id: media.id })
      .then((res) => {
        setExtras(res.extras || []);
      })
      .catch(() => setError("Failed to fetch extras"))
      .finally(() => setSearchLoading(false));
  }, [media, mediaType]);

  // WebSocket for real-time extras status
  const wsRef = useRef(null);
  useEffect(() => {
    if (!media) return;
    const wsUrl =
      (globalThis.window.location.protocol === "https:" ? "wss://" : "ws://") +
      globalThis.window.location.host +
      "/ws/download-queue";
    const ws = new globalThis.window.WebSocket(wsUrl);
    wsRef.current = ws;
    ws.onopen = () => {
      console.debug("[WebSocket] Connected to download queue (MediaDetails)");
    };
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleExtrasQueueUpdate(msg, media.id, setExtras, setError);
      } catch (err) {
        console.debug("[WebSocket] Error parsing message", err);
      }
    };
    ws.onerror = (e) => {
      console.debug("[WebSocket] Error", e);
    };
    ws.onclose = () => {
      console.debug("[WebSocket] Closed (MediaDetails)");
    };
    return () => {
      ws.close();
    };
  }, [media]);

  const renderSkeleton = () => {
    return (
      <Container
        className={`md-skeleton-container ${isDark ? "md-dark" : ""} ${isMobile ? "md-mobile" : ""}`}
      >
        <div
          className={`md-skeleton-main ${isDark ? "md-dark" : ""} ${isMobile ? "md-mobile" : ""}`}
        >
          {!isMobile && (
            <div className={`md-skel-poster ${isDark ? "md-dark" : ""}`} />
          )}
          <div className="md-skel-info" style={{ flex: 1 }}>
            <div className="md-skel-title" />
            <div className="md-skel-sub" />
            <div className="md-skel-actions">
              <div className="md-skel-action" />
              <div className="md-skel-action" />
            </div>
            <div className="md-skel-line" />
            <div className="md-skel-paragraph" />
            <div className="md-skel-paragraph" style={{ width: "90%" }} />
            <div className="md-skel-paragraph" style={{ width: "80%" }} />
          </div>
        </div>

        {/* Extras skeleton */}
        <div className="md-extras-skeleton">
          {["group-a", "group-b", "group-c"].map((gKey) => (
            <div key={gKey} className="md-extras-group">
              <div className="md-extras-group-title" />
              <div className="md-extras-cards">
                {["s1", "s2", "s3", "s4"].map((sKey) => (
                  <div key={sKey} className="md-extras-card-skel" />
                ))}
              </div>
            </div>
          ))}
        </div>
      </Container>
    );
  };

  if (loading) return renderSkeleton();
  if (!media) {
    return (
      <div>
        Media not found
        <pre
          style={{
            background: "#eee",
            color: "#222",
            padding: 8,
            marginTop: 12,
            fontSize: 13,
          }}
        >
          Debug info: id: {String(id)}
          mediaItems.length: {mediaItems ? mediaItems.length : "undefined"}
          mediaItems: {JSON.stringify(mediaItems, null, 2)}
        </pre>
      </div>
    );
  }

  // Group extras by type
  const extrasByType = extras.reduce((acc, extra) => {
    const type = extra.ExtraType || "Other";
    if (!acc[type]) acc[type] = [];
    acc[type].push(extra);
    return acc;
  }, {});

  // --- Preserve manual search (ytResults) order in Trailers group ---
  if (ytResults.length > 0) {
    const ytExtras = ytResultsToExtras(ytResults);
    const backend = extrasByType["Trailers"] || [];
    // Map backend extras by YoutubeId for quick lookup
    const backendMap = Object.fromEntries(backend.map((e) => [e.YoutubeId, e]));
    // For each manual search card, if a backend extra exists, merge status/fields, else use manual
    const merged = ytExtras.map((yt) => {
      const be = backendMap[yt.YoutubeId];
      return be ? { ...yt, ...be } : yt;
    });
    // Append backend-only extras (not in ytResults) after manual search cards
    const ytIds = new Set(ytExtras.map((e) => e.YoutubeId));
    for (const be of backend) {
      if (!ytIds.has(be.YoutubeId)) merged.push(be);
    }
    extrasByType["Trailers"] = merged;
  }

  return (
    <Container
      style={{
        minHeight: "100vh",
        background: isDark ? "#18181b" : "#f7f8fa",
        fontFamily: "Roboto, Arial, sans-serif",
        padding: 0,
      }}
    >
      {/* Floating Modal for Download Error */}
      {showModal && (
        <div
          style={{
            position: "fixed",
            top: 24,
            left: "50%",
            transform: "translateX(-50%)",
            background: "#ef4444",
            color: "#fff",
            padding: "12px 32px",
            borderRadius: 8,
            boxShadow: "0 2px 12px rgba(0,0,0,0.18)",
            zIndex: 9999,
            fontWeight: 500,
            fontSize: 16,
            minWidth: 260,
            textAlign: "center",
          }}
        >
          {modalMsg}
        </div>
      )}
      <ActionLane
        buttons={[
          {
            icon: searchLoading ? (
              <FontAwesomeIcon icon={faSpinner} spin />
            ) : (
              <FontAwesomeIcon icon={faSearch} />
            ),
            label: "Search",
            onClick: () => {
              if (!media) return;
              if (!mediaType || !media.id) {
                setError?.("Missing media info for YouTube search");
                return;
              }
              setSearchLoading(true);
              setError?.("");
              setYtResults([]);
              let results = [];
              searchYoutubeStream({
                mediaType,
                mediaId: media.id,
                onResult: (item) => {
                  results.push(item);
                  setYtResults([...results]);
                },
                onDone: () => setSearchLoading(false),
                onError: () => {
                  setError?.("YouTube search failed");
                  setSearchLoading(false);
                },
              });
            },
            disabled: searchLoading,
            loading: searchLoading,
            showLabel:
              globalThis.window !== undefined &&
              globalThis.window.innerWidth > 900,
          },
        ]}
      />
      <MediaInfoLane
        media={{ ...media, mediaType }}
        mediaType={mediaType}
        error={error}
        cast={cast}
        castLoading={castLoading}
        castError={castError}
      />
      <Toast message={error} onClose={() => setError("")} />
      {/* Grouped extras by type, with 'Trailers' first */}
      {Object.keys(extrasByType).length > 0 && (
        <div
          style={{
            width: "100%",
            background: isDark ? "#23232a" : "#f3e8ff",
            overflow: "hidden",
            padding: "10px 10px", // Increased left/right padding
            margin: 0,
          }}
        >
          <ExtrasList
            extrasByType={extrasByType}
            media={media}
            mediaType={mediaType}
            setExtras={setExtras}
            setModalMsg={setModalMsg}
            setShowModal={setShowModal}
            YoutubeEmbed={YoutubeEmbed}
            setYoutubeModal={setYoutubeModal}
          />
        </div>
      )}
      {/* Render YouTube modal only once at the page level */}
      <YoutubeModal
        open={youtubeModal.open}
        videoId={youtubeModal.videoId}
        onClose={() => setYoutubeModal({ open: false, videoId: "" })}
      />
    </Container>
  );
}

MediaDetails.propTypes = {
  mediaItems: PropTypes.arrayOf(PropTypes.object).isRequired,
  loading: PropTypes.bool.isRequired,
  mediaType: PropTypes.oneOf(["movie", "series", "tv"]).isRequired,
};
