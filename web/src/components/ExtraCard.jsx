import React, { useState, useMemo, useCallback } from "react";
import { deleteExtra } from "../api";
import IconButton from "./IconButton.jsx";
import PropTypes from "prop-types";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faTrashCan, faCheckSquare } from "@fortawesome/free-regular-svg-icons";
import {
  faPlay,
  faDownload,
  faBan,
  faCircleXmark,
  faClock,
} from "@fortawesome/free-solid-svg-icons";
import { isDark } from "../utils/isDark.js";

// Export PosterImage at the end for SonarLint compliance
// Avoid re-renders of individual cards when unrelated props change.
function areEqual(prevProps, nextProps) {
  const prev = prevProps.extra || {};
  const next = nextProps.extra || {};
  if (prev.YoutubeId !== next.YoutubeId) return false;
  if (prev.Status !== next.Status) return false;
  if (prev.Reason !== next.Reason) return false;
  if (prevProps.rejected !== nextProps.rejected) return false;
  if (prevProps.idx !== nextProps.idx) return false;
  const prevMediaId = prevProps.media?.id;
  const nextMediaId = nextProps.media?.id;
  if (prevMediaId !== nextMediaId) return false;
  return true;
}

// Extracted action buttons to reduce cognitive complexity
function ExtraCardActions({
  extra,
  imgError,
  isFallback,
  downloaded,
  isDownloading,
  isQueued,
  rejected,
  onPlay,
  showToast,
  setExtras,
  baseType,
  baseTitle,
  mediaType,
  media,
  handleDownloadClick,
  handleDeleteClick,
}) {
  return (
    <>
      {/* Play button overlay (with image) */}
      {extra.YoutubeId && !imgError && !isFallback && (
        <div
          style={{
            position: "absolute",
            top: "50%",
            left: "50%",
            transform: "translate(-50%, -50%)",
            zIndex: 2,
          }}
        >
          <IconButton
            icon={
              <FontAwesomeIcon
                icon={faPlay}
                color="#fff"
                size="lg"
                style={{ filter: "drop-shadow(0 2px 8px #000)" }}
              />
            }
            title="Play"
            onClick={(e) => {
              e.stopPropagation();
              if (onPlay) onPlay(extra.YoutubeId);
            }}
          />
        </div>
      )}
      {/* Failed/Rejected Icon (always show for failed/rejected) */}
      {(extra.Status === "failed" ||
        extra.Status === "rejected" ||
        extra.Status === "unknown" ||
        extra.Status === "error") && (
        <div style={{ position: "absolute", top: 8, left: 8, zIndex: 2 }}>
          <IconButton
            icon={
              <FontAwesomeIcon icon={faCircleXmark} color="#ef4444" size="lg" />
            }
            title={
              extra.Status === "failed" ? "Remove failed status" : "Remove ban"
            }
            onClick={(event) =>
              handleRemoveBan({
                event,
                extra,
                baseType,
                baseTitle,
                mediaType,
                media,
                setExtras,
                showToast,
              })
            }
            aria-label={
              extra.Status === "failed" ? "Remove failed status" : "Remove ban"
            }
          />
        </div>
      )}
      {/* Download or Delete Buttons */}
      {extra.YoutubeId && !downloaded && !imgError && !isFallback && (
        <div style={{ position: "absolute", top: 8, right: 8, zIndex: 2 }}>
          <IconButton
            icon={
              <DownloadIcon isDownloading={isDownloading} isQueued={isQueued} />
            }
            title={getDownloadButtonTitle({
              rejected,
              extra,
              isDownloading,
              isQueued,
            })}
            onClick={
              rejected || isDownloading || isQueued
                ? undefined
                : (e) => {
                    e.stopPropagation();
                    handleDownloadClick();
                  }
            }
            disabled={rejected || isDownloading || isQueued}
            aria-label="Download"
            style={(() => {
              let opacity = 1;
              let borderRadius = 0;
              if (rejected) opacity = 0.5;
              else if (isDownloading || isQueued) {
                opacity = 0.7;
                borderRadius = 8;
              }
              return {
                opacity,
                background: "transparent",
                borderRadius,
                transition: "background 0.2s, opacity 0.2s",
              };
            })()}
          />
        </div>
      )}
      {/* Downloaded Checkmark and Delete Button */}
      {downloaded && (
        <>
          <div style={{ position: "absolute", top: 8, right: 8, zIndex: 2 }}>
            <IconButton
              icon={
                <FontAwesomeIcon
                  icon={faCheckSquare}
                  color="#22c55e"
                  size="lg"
                />
              }
              title="Downloaded"
              disabled
            />
          </div>
          <div style={{ position: "absolute", bottom: 8, right: 8, zIndex: 2 }}>
            <IconButton
              icon={
                <FontAwesomeIcon icon={faTrashCan} color="#ef4444" size="md" />
              }
              title="Delete"
              onClick={handleDeleteClick}
            />
          </div>
        </>
      )}
    </>
  );
}

// Helper for download button title (SonarLint: move out of render)
function getDownloadButtonTitle({ rejected, extra, isDownloading, isQueued }) {
  if (rejected) return extra.Reason;
  if (isDownloading) return "Downloading...";
  if (isQueued) return "Queued";
  return "Download";
}

// Helper for display title
function getDisplayTitle(typeExtras, baseTitle, idx) {
  const totalCount = typeExtras.filter(
    (e) => e.ExtraTitle === baseTitle,
  ).length;
  let title =
    totalCount > 1
      ? `${baseTitle} (${typeExtras.slice(0, idx + 1).filter((e) => e.ExtraTitle === baseTitle).length})`
      : baseTitle;
  const maxLen = 40;
  if (title.length > maxLen) {
    title = title.slice(0, maxLen - 3) + "...";
  }
  return title;
}

// Helper for border color
function getBorderColor({ rejected, failed, downloaded, exists }) {
  if (rejected || failed) return "2.5px solid #ef4444";
  if (downloaded) return "2px solid #22c55e";
  if (exists) return "2px solid #8888";
  return "2px solid transparent";
}

// Poster image or fallback factory (top-level, moved out for SonarLint)
function PosterImage({ src, alt, onError, onLoad, fallbackIcon, imgError }) {
  let content;
  if (src) {
    content = (
      <img
        src={src}
        alt={alt}
        onLoad={onLoad}
        onError={onError}
        style={{
          display: "block",
          margin: "0 0",
          maxHeight: 135,
          maxWidth: "100%",
          objectFit: "contain",
          background: "#222222",
        }}
      />
    );
  } else if (!src || imgError) {
    content = (
      <span
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          width: "100%",
          height: 135,
          background: "#222222",
        }}
      >
        <FontAwesomeIcon icon={fallbackIcon} color="#888" size="4x" />
      </span>
    );
  } else {
    content = (
      <span
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          width: "100%",
          height: 135,
          background: "#333",
          animation: "pulse 1.2s infinite",
          color: "#444",
        }}
      >
        Loading...
      </span>
    );
  }
  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      {content}
    </div>
  );
}

// Top-level error modal
function ErrorModal({ message, onClose }) {
  return (
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
      {message}
      <button
        onClick={onClose}
        style={{
          marginLeft: 16,
          background: "transparent",
          color: "#fff",
          border: "none",
          fontSize: 18,
          cursor: "pointer",
        }}
      >
        ×
      </button>
    </div>
  );
}

// Top-level spinner icon for download
function SpinnerIcon() {
  return (
    <span
      className="download-spinner"
      style={{
        display: "inline-block",
        width: 22,
        height: 22,
        background: "transparent",
      }}
    >
      <svg
        viewBox="0 0 50 50"
        style={{ width: 22, height: 22, background: "transparent" }}
      >
        <circle
          cx="25"
          cy="25"
          r="20"
          fill="none"
          stroke="#fff"
          strokeWidth="5"
          strokeDasharray="31.4 31.4"
          strokeLinecap="round"
        >
          <animateTransform
            attributeName="transform"
            type="rotate"
            from="0 25 25"
            to="360 25 25"
            dur="0.8s"
            repeatCount="indefinite"
          />
        </circle>
      </svg>
    </span>
  );
}

function DownloadIcon({ isDownloading, isQueued }) {
  if (isDownloading) return <SpinnerIcon />;
  if (isQueued)
    return <FontAwesomeIcon icon={faClock} color="#fff" size="lg" />;
  return <FontAwesomeIcon icon={faDownload} color="#fff" size="lg" />;
}

DownloadIcon.propTypes = {
  isDownloading: PropTypes.bool,
  isQueued: PropTypes.bool,
};

function handleRemoveBan({
  event,
  extra,
  baseType,
  baseTitle,
  mediaType,
  media,
  setExtras,
  showToast,
}) {
  event.stopPropagation();
  // Always call backend to remove from blacklist for both rejected and failed statuses
  fetch("/api/blacklist/extras/remove", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      mediaType,
      mediaId: media.id,
      extraType: baseType,
      extraTitle: baseTitle,
      youtubeId: extra.YoutubeId,
    }),
  })
    .then(() => {
      if (typeof setExtras === "function") {
        setExtras((prev) => {
          // Only operate on array states; otherwise leave as-is
          if (!Array.isArray(prev)) return prev;
          // Normalize matching for both blacklist (lowercase keys) and media extras (PascalCase keys)
          const next = prev
            .map((ex) => {
              const y = ex.YoutubeId || ex.youtubeId || "";
              const t = ex.ExtraType || ex.extraType || "";
              const ti = ex.ExtraTitle || ex.extraTitle || "";
              if (y === extra.YoutubeId && t === baseType && ti === baseTitle) {
                // If this array looks like the blacklist (lowercase keys), remove the item.
                // Otherwise, clear the Status fields for media extras lists.
                if (
                  ex.youtubeId !== undefined ||
                  ex.mediaId !== undefined ||
                  ex.mediaTitle !== undefined
                ) {
                  // blacklist-style item -> remove from list
                  return null;
                }
                // media extras-style item -> clear status
                return { ...ex, Status: "", status: "" };
              }
              return ex;
            })
            .filter(Boolean);
          return next;
        });
      }
    })
    .catch(() => {
      if (typeof showToast === "function") {
        showToast("Failed to remove ban.");
      }
    });
}

function ExtraCard({
  extra,
  idx,
  typeExtras,
  media,
  mediaType,
  setExtras,
  rejected: rejectedProp,
  onPlay,
  showToast, // new prop for toast/modal
}) {
  const [imgError, setImgError] = useState(false);
  const [isFallback, setIsFallback] = useState(false);
  const baseTitle = extra.ExtraTitle || "";
  const baseType = extra.ExtraType || "";
  // Memoize frequently computed values to avoid per-render recomputation
  const displayTitle = useMemo(
    () => getDisplayTitle(typeExtras, baseTitle, idx),
    [typeExtras, baseTitle, idx],
  );

  const posterUrl = useMemo(
    () =>
      extra.YoutubeId ? `/api/proxy/youtube-image/${extra.YoutubeId}` : null,
    [extra.YoutubeId],
  );
  React.useEffect(() => {
    // Reset states when posterUrl changes
    setImgError(false);
    setIsFallback(false);
  }, [posterUrl]);
  const titleFontSize = useMemo(() => {
    if (displayTitle.length > 32) return 12;
    if (displayTitle.length > 22) return 14;
    return 16;
  }, [displayTitle]);
  const downloaded = extra.Status === "downloaded";
  const isDownloading = extra.Status === "downloading";
  const isQueued = extra.Status === "queued";
  const failed =
    extra.Status === "failed" ||
    extra.Status === "rejected" ||
    extra.Status === "unknown" ||
    extra.Status === "error";
  const exists = extra.Status === "exists";
  const [downloading, setDownloading] = useState(false);
  // Use the rejected prop if provided, otherwise fallback to extra.Status
  const [unbanned] = useState(false);
  // Treat 'failed' as 'rejected' for UI
  const rejected =
    !unbanned &&
    (typeof rejectedProp === "boolean"
      ? rejectedProp
      : extra.Status === "rejected" || extra.Status === "failed");
  // Removed errorCard/modal state; error display is now handled at the page level

  // showErrorModal removed; use showToast for error display

  function revertStatus() {
    if (typeof setExtras === "function") {
      setExtras((prev) =>
        prev.map((ex) =>
          ex.YoutubeId === extra.YoutubeId &&
          ex.ExtraType === baseType &&
          ex.ExtraTitle === baseTitle
            ? { ...ex, Status: "" }
            : ex,
        ),
      );
    }
  }

  function handleError(msg) {
    if (typeof showToast === "function") {
      showToast(msg);
    }
    revertStatus();
  }

  const handleDownloadClick = useCallback(async () => {
    if (downloaded || downloading) return;
    // Optimistic UI: mark as queued immediately so the icon updates right away.
    // Update existing extra if present, otherwise append a queued entry.
    let didOptimisticallyAdd = false;
    if (
      typeof setExtras === "function" &&
      !downloaded &&
      !isDownloading &&
      !exists
    ) {
      setExtras((prev) => {
        let found = false;
        const updated = prev.map((ex) => {
          if (
            ex.YoutubeId === extra.YoutubeId &&
            ex.ExtraType === baseType &&
            ex.ExtraTitle === baseTitle
          ) {
            found = true;
            // If already queued, leave as-is
            if (ex.Status === "queued") return ex;
            didOptimisticallyAdd = true;
            return { ...ex, Status: "queued", Reason: "" };
          }
          return ex;
        });
        if (found) return updated;
        didOptimisticallyAdd = true;
        return [
          ...prev,
          {
            ...extra,
            Status: "queued",
            Reason: "",
            ExtraType: baseType,
            ExtraTitle: baseTitle,
            YoutubeId: extra.YoutubeId,
          },
        ];
      });
    }

    setDownloading(true);
    try {
      const res = await fetch(`/api/extras/download`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          mediaType,
          mediaId: media.id,
          extraType: baseType,
          extraTitle: baseTitle,
          youtubeId: extra.YoutubeId,
        }),
      });
      if (!res.ok) {
        // Backend failed: revert optimistic change and show error
        if (didOptimisticallyAdd && typeof setExtras === "function") {
          setExtras((prev) =>
            prev.filter(
              (ex) =>
                !(
                  ex.YoutubeId === extra.YoutubeId &&
                  ex.ExtraType === baseType &&
                  ex.ExtraTitle === baseTitle
                ),
            ),
          );
        }
        const data = await res.json().catch(() => ({}));
        handleError(data?.error || "Download failed");
      }
      // On success: backend will broadcast queue changes and the store will reflect queued state;
      // we keep the optimistic queued state until updates arrive from server.
    } catch (error) {
      if (didOptimisticallyAdd && typeof setExtras === "function") {
        setExtras((prev) =>
          prev.filter(
            (ex) =>
              !(
                ex.YoutubeId === extra.YoutubeId &&
                ex.ExtraType === baseType &&
                ex.ExtraTitle === baseTitle
              ),
          ),
        );
      }
      handleError(error.message || error);
    } finally {
      setDownloading(false);
    }
  }, [downloaded, downloading, extra, setExtras, media, baseType, baseTitle]);

  const handleDeleteClick = useCallback(
    async (event) => {
      event.stopPropagation();
      if (!globalThis.confirm("Delete this extra?")) return;
      // Optimistic UI: mark as deleting so the card does not render as 'downloaded' (green)
      const prevStatus = extra.Status;
      if (typeof setExtras === "function") {
        setExtras((prev) =>
          prev.map((ex) =>
            ex.ExtraTitle === baseTitle && ex.ExtraType === baseType
              ? { ...ex, Status: "deleting" }
              : ex,
          ),
        );
      }
      try {
        const payload = {
          mediaType,
          mediaId: media.id,
          youtubeId: extra.YoutubeId,
        };
        await deleteExtra(payload);
        // On success set to missing so UI reflects absence
        setExtras((prev) =>
          prev.map((ex) =>
            ex.ExtraTitle === baseTitle && ex.ExtraType === baseType
              ? { ...ex, Status: "missing" }
              : ex,
          ),
        );
      } catch (error) {
        // Revert optimistic state on failure
        if (typeof setExtras === "function") {
          setExtras((prev) =>
            prev.map((ex) =>
              ex.ExtraTitle === baseTitle && ex.ExtraType === baseType
                ? { ...ex, Status: prevStatus }
                : ex,
            ),
          );
        }
        let msg = error?.message || error;
        if (error?.detail) msg += `\n${error.detail}`;
        if (typeof showToast === "function") showToast(msg || "Delete failed");
      }
    },
    [extra, baseTitle, baseType, media, setExtras, showToast],
  );

  const borderColor = getBorderColor({ rejected, failed, downloaded, exists });
  return (
    <div
      title={rejected ? extra.Reason : undefined}
      style={{
        width: 180,
        height: 210,
        background: isDark ? "#18181b" : "#fff",
        borderRadius: 12,
        boxShadow: isDark
          ? "0 2px 12px rgba(0,0,0,0.22)"
          : "0 2px 12px rgba(0,0,0,0.10)",
        overflow: "hidden",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        padding: "0 0 0 0",
        position: "relative",
        border: borderColor,
      }}
    >
      <div
        style={{
          width: "100%",
          height: 135,
          background: "#222",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          position: "relative",
        }}
      >
        {/* Poster Image or Fallback */}
        {!imgError && posterUrl ? (
          <PosterImage
            key={posterUrl}
            src={posterUrl}
            alt={displayTitle}
            fallbackIcon={faBan}
            onLoad={(event) => {
              // If the image loads but is very small (our SVG fallback is 64x64), treat as fallback
              try {
                const img = event.target;
                if (img.naturalWidth <= 64 && img.naturalHeight <= 64) {
                  setIsFallback(true);
                  setImgError(true);
                }
              } catch {
                // ignore
              }
            }}
            onError={() => {
              setIsFallback(true);
              setImgError(true);
            }}
            imgError={imgError}
          />
        ) : (
          <PosterImage
            src={null}
            alt="Denied"
            fallbackIcon={faBan}
            imgError={imgError}
          />
        )}
        <ExtraCardActions
          extra={extra}
          imgError={imgError}
          isFallback={isFallback}
          downloaded={downloaded}
          isDownloading={isDownloading}
          isQueued={isQueued}
          rejected={rejected}
          onPlay={onPlay}
          showToast={showToast}
          setExtras={setExtras}
          baseType={baseType}
          baseTitle={baseTitle}
          mediaType={mediaType}
          media={media}
          handleDownloadClick={handleDownloadClick}
          handleDeleteClick={handleDeleteClick}
        />
      </div>
      <div
        style={{
          width: "100%",
          padding: "12px 10px 0 10px",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
        }}
      >
        <div
          style={{
            fontWeight: 600,
            fontSize: titleFontSize,
            color: isDark ? "#e5e7eb" : "#222",
            textAlign: "center",
            marginBottom: 4,
            height: 50,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            overflow: "hidden",
            width: "100%",
          }}
        >
          {displayTitle}
        </div>
        <div
          style={{
            width: "100%",
            display: "flex",
            justifyContent: "flex-end",
            alignItems: "center",
            gap: 18,
            position: "absolute",
            bottom: 12,
            left: 0,
          }}
        ></div>
        {/* YouTube modal is now rendered at the page level */}
      </div>
    </div>
  );
}

ExtraCard.propTypes = {
  extra: PropTypes.shape({
    Reason: PropTypes.string,
    ExtraTitle: PropTypes.string,
    ExtraType: PropTypes.string,
    YoutubeId: PropTypes.string,
    Status: PropTypes.string,
  }).isRequired,
  idx: PropTypes.number,
  typeExtras: PropTypes.array,
  media: PropTypes.object,
  mediaType: PropTypes.string,
  setExtras: PropTypes.func,
  // setModalMsg and setShowModal removed (unused props)
  rejected: PropTypes.bool,
  onPlay: PropTypes.func,
  showToast: PropTypes.func,
};

PosterImage.propTypes = {
  src: PropTypes.string,
  alt: PropTypes.string.isRequired,
  onError: PropTypes.func,
  onLoad: PropTypes.func,
  fallbackIcon: PropTypes.object.isRequired,
  imgError: PropTypes.bool,
};

ErrorModal.propTypes = {
  message: PropTypes.string.isRequired,
  onClose: PropTypes.func.isRequired,
};

// PropTypes for ExtraCardActions
ExtraCardActions.propTypes = {
  extra: PropTypes.shape({
    YoutubeId: PropTypes.string,
    Status: PropTypes.string,
  }).isRequired,
  imgError: PropTypes.bool,
  isFallback: PropTypes.bool,
  downloaded: PropTypes.bool,
  isDownloading: PropTypes.bool,
  isQueued: PropTypes.bool,
  rejected: PropTypes.bool,
  onPlay: PropTypes.func,
  showToast: PropTypes.func,
  setExtras: PropTypes.func,
  baseType: PropTypes.string,
  baseTitle: PropTypes.string,
  mediaType: PropTypes.string,
  media: PropTypes.object,
  handleDownloadClick: PropTypes.func,
  handleDeleteClick: PropTypes.func,
};

export default React.memo(ExtraCard, areEqual);

export { PosterImage };
