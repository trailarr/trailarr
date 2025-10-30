import React, { useState, useEffect, useRef } from "react";
import PropTypes from "prop-types";
import "./BlacklistPage.css";
import ExtraCard from "./ExtraCard.jsx";
import YoutubePlayer from "./YoutubePlayer.jsx";
import Container from "./Container.jsx";
import SectionHeader from "./SectionHeader.jsx";
import { isDark } from "../utils/isDark";

// Helper to normalize reason string for grouping (moved to outer scope)
function normalizeReason(reason) {
  if (!reason) return "Other";
  // Extract main error type for grouping
  // 1. Private video
  if (reason.includes("Private video. Sign in if you")) {
    return "Private video. Sign in if you've been granted access to this video.";
  }
  // 2. Not available in your country
  if (
    reason.includes(
      "The uploader has not made this video available in your country",
    )
  ) {
    return "The uploader has not made this video available in your country.";
  }
  // 3. Age-restricted video
  if (reason.includes("Sign in to confirm your age.")) {
    return "Sign in to confirm your age.";
  }
  // 4. Did not get any data blocks
  if (reason.includes("Did not get any data blocks")) {
    return "Did not get any data blocks";
  }
  // 4. Fallback: first line of error
  const firstLine = reason.split("\n")[0];
  // Remove YouTube ID and video ID
  return firstLine
    .replace(/\[youtube\] [\w-]+:/, "[youtube] <id>:")
    .replace(/ERROR: \[youtube\] [\w-]+:/, "ERROR: [youtube] <id>:")
    .trim();
}

// Subcomponent to render a single group item (reduces nesting in main render)
function BlacklistGroupItem({ item, idx, setYoutubeModal, setBlacklist }) {
  const extra = {
    ExtraTitle: item.extraTitle || "",
    ExtraType: item.extraType || "",
    YoutubeId: item.youtubeId || "",
    reason: item.reason || item.message || "",
    Status: item.Status || item.status || "",
  };
  // Ensure `media` matches the shape expected by ExtraCard (media.id)
  const media = {
    id: item.mediaId ? Number(item.mediaId) : 0,
    title: item.mediaTitle || "",
  };
  const mediaType = item.mediaType || "";
  const uniqueKey = `${extra.YoutubeId || ""}-${media.id || ""}-${mediaType}`;
  let mediaHref = "";
  if (mediaType === "movie") mediaHref = `/movies/${media.id}`;
  else if (mediaType === "tv") mediaHref = `/series/${media.id}`;

  const handleDownloaded = () => {
    setBlacklist((prev) => markBlacklistItemDownloaded(prev, extra.YoutubeId));
  };

  return (
    <div
      key={uniqueKey}
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "stretch",
      }}
    >
      <ExtraCard
        extra={extra}
        idx={idx}
        typeExtras={[]}
        media={media}
        mediaType={mediaType}
        // Pass the page-level setter so ExtraCard's unban handler can refresh
        // the blacklist state after removing an item.
        setExtras={setBlacklist}
        setModalMsg={() => {}}
        setShowModal={() => {}}
        YoutubeEmbed={null}
        rejected={true}
        onPlay={(videoId) => setYoutubeModal({ open: true, videoId })}
        onDownloaded={handleDownloaded}
      />
      {media.title &&
        !!media.id &&
        (mediaHref ? (
          <a
            href={mediaHref}
            style={{
              marginTop: 8,
              fontSize: "0.97em",
              color: isDark ? "#f3f4f6" : "#23232a",
              textDecoration: "none",
              textAlign: "center",
              wordBreak: "break-word",
              display: "block",
              fontWeight: 500,
            }}
          >
            {media.title}
          </a>
        ) : (
          <button
            type="button"
            disabled
            style={{
              marginTop: 8,
              fontSize: "0.97em",
              color: isDark ? "#f3f4f6" : "#23232a",
              background: "none",
              border: "none",
              textDecoration: "none",
              textAlign: "center",
              wordBreak: "break-word",
              display: "block",
              fontWeight: 500,
              cursor: "not-allowed",
              opacity: 0.7,
            }}
          >
            {media.title}
          </button>
        ))}
    </div>
  );
}

BlacklistGroupItem.propTypes = {
  item: PropTypes.shape({
    extraTitle: PropTypes.string,
    extraType: PropTypes.string,
    youtubeId: PropTypes.string,
    reason: PropTypes.string,
    message: PropTypes.string,
    Status: PropTypes.string,
    status: PropTypes.string,
    mediaId: PropTypes.string,
    mediaTitle: PropTypes.string,
    mediaType: PropTypes.string,
  }).isRequired,
  idx: PropTypes.number.isRequired,
  setYoutubeModal: PropTypes.func.isRequired,
  setBlacklist: PropTypes.func.isRequired,
};

// Helper to mark a blacklist item as downloaded
function markBlacklistItemDownloaded(prev, youtubeId) {
  if (!prev) return prev;
  const update = (arr) =>
    arr.map((item2) => {
      if (item2.youtubeId === youtubeId) {
        return { ...item2, status: "downloaded", Status: "downloaded" };
      }
      return item2;
    });
  if (Array.isArray(prev)) return update(prev);
  const updated = {};
  for (const k in prev) updated[k] = update(prev[k]);
  return updated;
}

// Helper to update blacklist items with queue status
function updateBlacklistWithQueue(prev, queue) {
  if (!prev) return prev;
  const update = (arr) =>
    arr.map((item2) => {
      const found = queue.find((q) => q.YouTubeID === item2.youtubeId);
      if (found?.Status && item2.Status !== found.Status) {
        return { ...item2, status: found.Status, Status: found.Status };
      }
      return item2;
    });
  if (Array.isArray(prev)) return update(prev);
  const updated = {};
  for (const k in prev) updated[k] = update(prev[k]);
  return updated;
}

// Helper to preload images (outer scope)
function preloadImages(urls) {
  return Promise.all(
    urls.map(
      (url) =>
        new Promise((resolve) => {
          if (!url) return resolve();
          const img = new globalThis.Image();
          img.onload = img.onerror = () => resolve();
          img.src = url;
        }),
    ),
  );
}

export default function BlacklistPage() {
  const [blacklist, setBlacklist] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [youtubeModal, setYoutubeModal] = useState({
    open: false,
    videoId: "",
  });

  useEffect(() => {
    fetch("/api/blacklist/extras")
      .then((res) => {
        if (!res.ok) throw new Error("Failed to fetch blacklist");
        return res.json();
      })
      .then((data) => {
        setBlacklist(data);
        const items = Array.isArray(data) ? data : Object.values(data).flat();
        const urls = items
          .map((item) => item.thumbnail || item.poster || item.image || null)
          .filter(Boolean);
        if (urls.length > 0) {
          const MAX_PRELOAD = 40;
          preloadImages(urls.slice(0, MAX_PRELOAD)).catch(() => {});
        }
        setLoading(false);
      })
      .catch((e) => {
        setError(e.message);
        setLoading(false);
      });
  }, []);

  // WebSocket for real-time blacklist status
  const wsRef = useRef(null);
  useEffect(() => {
    const wsUrl =
      (globalThis.location.protocol === "https:" ? "wss://" : "ws://") +
      globalThis.location.host +
      "/ws/download-queue";
    const ws = new globalThis.WebSocket(wsUrl);
    wsRef.current = ws;
    ws.onopen = () => {
      console.debug("[WebSocket] Connected to download queue (BlacklistPage)");
    };
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === "download_queue_update" && Array.isArray(msg.queue)) {
          setBlacklist((prev) => updateBlacklistWithQueue(prev, msg.queue));
        }
      } catch (err) {
        console.debug("[WebSocket] Error parsing message", err);
      }
    };
    ws.onerror = (e) => {
      console.debug("[WebSocket] Error", e);
    };
    ws.onclose = () => {
      console.debug("[WebSocket] Closed (BlacklistPage)");
    };
    return () => {
      ws.close();
    };
  }, []);

  if (loading) {
    const skeletonKeys = Array.from({ length: 8 }).map(
      () => `skeleton-${Math.random().toString(36).slice(2, 9)}`,
    );
    return (
      <div style={{ padding: 32 }}>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, 220px)",
            gap: 24,
            padding: 0,
            margin: 0,
            width: "100%",
            boxSizing: "border-box",
          }}
        >
          {skeletonKeys.map((key) => (
            <div
              key={key}
              style={{
                width: 220,
                minHeight: 280,
                borderRadius: 12,
                background: isDark ? "#23232a" : "#f3f4f6",
                padding: 12,
                boxSizing: "border-box",
                display: "flex",
                flexDirection: "column",
                gap: 10,
              }}
            >
              <div
                style={{
                  height: 140,
                  borderRadius: 8,
                  background: isDark ? "#1f2937" : "#e5e7eb",
                }}
              />
              <div
                style={{
                  height: 14,
                  width: "70%",
                  borderRadius: 6,
                  background: isDark ? "#111827" : "#e9ecef",
                }}
              />
              <div
                style={{
                  height: 12,
                  width: "50%",
                  borderRadius: 6,
                  background: isDark ? "#111827" : "#e9ecef",
                }}
              />
              <div style={{ flex: 1 }} />
            </div>
          ))}
        </div>
      </div>
    );
  }
  if (error) return <div style={{ color: "red", padding: 32 }}>{error}</div>;
  if (!blacklist || (Array.isArray(blacklist) && blacklist.length === 0))
    return <div style={{ padding: 32 }}>No blacklisted extras found.</div>;

  // Normalize to array for rendering
  let items = null;
  if (Array.isArray(blacklist)) items = blacklist;
  else if (blacklist && typeof blacklist === "object")
    items = Object.values(blacklist);
  if (!Array.isArray(items)) {
    return (
      <div style={{ padding: 32, color: "red" }}>
        Unexpected data format
        <br />
        <pre>{JSON.stringify(blacklist, null, 2)}</pre>
      </div>
    );
  }

  // Group items by normalized reason
  const groups = {};
  for (const item of items) {
    const rawReason = item.reason || item.message || "Other";
    const normReason = normalizeReason(rawReason);
    if (!groups[normReason]) groups[normReason] = [];
    groups[normReason].push(item);
  }

  const totalItems = Object.values(groups).reduce(
    (acc, arr) => acc + arr.length,
    0,
  );
  if (totalItems === 0)
    return <div style={{ padding: 32 }}>No blacklisted extras found.</div>;

  const gridStyle = {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill, 220px)",
    gap: 24,
    padding: 32,
    margin: 0,
    width: "100%",
    boxSizing: "border-box",
    justifyItems: "start",
    alignItems: "start",
  };

  return (
    <Container
      style={{
        minHeight: "calc(100vh - 64px)",
        padding: 0,
        background: isDark ? "#18181b" : "#fff",
        color: isDark ? "#f3f4f6" : "#18181b",
      }}
    >
      {Object.entries(groups).map(([reason, groupItems]) => {
        let displayReason = reason;
        if (
          reason.includes("Did not get any data blocks") &&
          reason.length > 40
        ) {
          displayReason = reason.slice(0, 1000) + "...";
        }
        const groupKey = reason.replaceAll(/[^a-zA-Z0-9_-]/g, "_").slice(0, 40);
        return (
          <div
            key={groupKey}
            style={{
              marginBottom: 40,
              background: isDark ? "#23232a" : "#f3f4f6",
              borderRadius: 12,
              boxShadow: isDark ? "0 2px 8px #0004" : "0 2px 8px #0001",
              padding: 12,
            }}
          >
            <SectionHeader
              style={{
                fontWeight: 600,
                fontSize: "1.1em",
                margin: "0 0 16px 8px",
                color: "#ef4444",
                textAlign: "left",
                wordBreak: "break-word",
              }}
            >
              {displayReason}
            </SectionHeader>
            <div
              className="BlacklistExtrasGrid"
              style={{ ...gridStyle, justifyContent: "start" }}
            >
              {groupItems.map((item, idx) => (
                <BlacklistGroupItem
                  key={`${item.youtubeId || ""}-${item.mediaId || ""}-${item.mediaType || ""}`}
                  item={item}
                  idx={idx}
                  setYoutubeModal={setYoutubeModal}
                  setBlacklist={setBlacklist}
                />
              ))}
            </div>
          </div>
        );
      })}

      {youtubeModal.open && youtubeModal.videoId && (
        <dialog
          open
          aria-modal="true"
          className="blacklist-youtube-backdrop"
          tabIndex={0}
          onClick={() => setYoutubeModal({ open: false, videoId: "" })}
          onKeyDown={(e) => {
            if (e.key === "Escape" || e.key === "Enter")
              setYoutubeModal({ open: false, videoId: "" });
          }}
          style={{
            position: "fixed",
            top: 0,
            left: 0,
            width: "100vw",
            height: "100vh",
            background: "rgba(0,0,0,0.7)",
            zIndex: 99999,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              position: "relative",
              background: "#18181b",
              borderRadius: 16,
              boxShadow: "0 2px 24px #000",
              padding: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              overflow: "visible",
            }}
          >
            {/* Removed visible close button; clicking backdrop will close the modal */}
            <YoutubePlayer videoId={youtubeModal.videoId} />
          </div>
        </dialog>
      )}
    </Container>
  );
}
