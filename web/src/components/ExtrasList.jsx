import React, { useState, useEffect, useRef } from "react";
import ExtraCard from "./ExtraCard.jsx";
import SectionHeader from "./SectionHeader.jsx";
import Toast from "./Toast";
import PropTypes from "prop-types";
import "./ExtrasList.css";

function ExtrasList({
  extrasByType,
  media,
  mediaType,
  setExtras,
  setModalMsg,
  setShowModal,
  setYoutubeModal,
  YoutubeEmbed,
}) {
  const [toastMsg, setToastMsg] = useState("");
  const [toastSuccess, setToastSuccess] = useState(false);
  const wsRef = useRef(null);

  // WebSocket: Listen for download queue updates
  useEffect(() => {
    const wsUrl =
      (globalThis.location.protocol === "https:" ? "wss://" : "ws://") +
      globalThis.location.host +
      "/ws/download-queue";
    const ws = new globalThis.WebSocket(wsUrl);
    wsRef.current = ws;
    ws.onopen = () => {
      console.debug("[WebSocket] Connected to download queue");
    };
    // Extracted to reduce nesting
    function updateExtraWithQueue(ex, queue) {
      const queueItem = queue.find((q) => q.youtubeId === ex.YoutubeId);
      if (queueItem) {
        // Only show toast if status transitions to 'failed' or 'rejected'
        if (
          (queueItem.status === "failed" || queueItem.status === "rejected") &&
          ex.Status !== "failed" &&
          ex.Status !== "rejected" &&
          (queueItem.reason || queueItem.Reason)
        ) {
          setToastMsg(queueItem.reason || queueItem.Reason);
          setToastSuccess(false);
        }
        // Always update Status and Reason fields
        return {
          ...ex,
          Status: queueItem.status,
          reason: queueItem.reason || queueItem.Reason,
          Reason: queueItem.reason || queueItem.Reason,
        };
      }
      return ex;
    }

    function updateExtrasWithQueue(prev, queue) {
      return prev.map((ex) => updateExtraWithQueue(ex, queue));
    }

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === "download_queue_update" && Array.isArray(msg.queue)) {
          if (typeof setExtras === "function") {
            setExtras((prev) => updateExtrasWithQueue(prev, msg.queue));
          }
        }
      } catch (err) {
        console.debug("[WebSocket] Error parsing message", err);
      }
    };
    ws.onerror = (e) => {
      console.debug("[WebSocket] Error", e);
    };
    ws.onclose = () => {
      console.debug("[WebSocket] Closed");
    };
    return () => {
      ws.close();
    };
  }, [setExtras]);

  // Helper for rendering a group of extras
  const renderExtrasGroup = (type, typeExtras) => (
    <div key={type} style={{ marginBottom: 32 }}>
      <SectionHeader>{type}</SectionHeader>
      <div
        className="extras-list-group"
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(180px, 0px))",
          gap: "32px",
          justifyItems: "start",
          alignItems: "start",
          width: "100%",
          justifyContent: "start",
        }}
      >
        {typeExtras.map((extra, idx) => (
          <ExtraCard
            key={extra.YoutubeId || idx}
            extra={extra}
            idx={idx}
            type={type}
            typeExtras={typeExtras}
            media={media}
            mediaType={mediaType}
            setExtras={setExtras}
            // setModalMsg and setShowModal are still passed for legacy compatibility, but showModal/modalMsg are not used in ExtraCard
            setModalMsg={setModalMsg}
            setShowModal={setShowModal}
            YoutubeEmbed={YoutubeEmbed}
            onPlay={(videoId) => setYoutubeModal({ open: true, videoId })}
            showToast={setToastMsg}
          />
        ))}
      </div>
    </div>
  );

  // Render 'Trailers' first, then others except 'Other', then 'Other' last
  return (
    <>
      <Toast
        message={toastMsg}
        onClose={() => setToastMsg("")}
        success={toastSuccess}
      />
      {extrasByType["Trailers"] &&
        renderExtrasGroup("Trailers", extrasByType["Trailers"])}
      {Object.entries(extrasByType)
        .filter(([type]) => type !== "Trailers" && type !== "Other")
        .map(([type, typeExtras]) => renderExtrasGroup(type, typeExtras))}
      {extrasByType["Other"] &&
        renderExtrasGroup("Other", extrasByType["Other"])}
    </>
  );
}

ExtrasList.propTypes = {
  extrasByType: PropTypes.object.isRequired,
  media: PropTypes.object,
  mediaType: PropTypes.string,
  setExtras: PropTypes.func,
  setModalMsg: PropTypes.func,
  setShowModal: PropTypes.func,
  setYoutubeModal: PropTypes.func,
  YoutubeEmbed: PropTypes.oneOfType([PropTypes.func, PropTypes.object]),
};

export default ExtrasList;
