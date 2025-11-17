import React from "react";
import PropTypes from "prop-types";
import IconButton from "../ui/IconButton.jsx";
import { getProviderSettings } from "../../api";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBookmark } from "@fortawesome/free-regular-svg-icons";
import { faLanguage } from "@fortawesome/free-solid-svg-icons";
import ActorRow from "../ui/ActorRow.jsx";
import { isDark } from "../../utils/isDark";
import "./MediaInfo.css";

export default function MediaInfoLane({
  media,
  mediaType,
  cast = [],
  castLoading = false,
  castError = "",
}) {
  // The 'error' prop was intentionally omitted from the params to avoid an unused-variable lint warning.
  const [showAlt, setShowAlt] = React.useState(false);

  // Fetch the provider URL on-demand (component-local state)
  const [providerUrl, setProviderUrl] = React.useState("");

  React.useEffect(() => {
    let mounted = true;
    // Fetch only the provider required for this mediaType and store it in providerUrl
    if (mediaType === "movie") {
      getProviderSettings("radarr")
        .then((res) => {
          if (mounted) setProviderUrl(res.url || "");
        })
        .catch(() => {
          if (mounted) setProviderUrl("");
        });
    } else if (mediaType === "tv" || mediaType === "series") {
      getProviderSettings("sonarr")
        .then((res) => {
          if (mounted) setProviderUrl(res.url || "");
        })
        .catch(() => {
          if (mounted) setProviderUrl("");
        });
    }
    return () => {
      mounted = false;
    };
  }, [mediaType]);

  if (!media) return null;
  let background;
  if (mediaType === "tv") {
    // Position background slightly below the top (around 30%) to show upper-to-middle of the fanart
    background = `url(/mediacover/Series/${media.id}/fanart-1280.jpg) center 10%/cover no-repeat`;
  } else {
    // Position background slightly below the top (around 30%) to show upper-to-middle of the fanart
    background = `url(/mediacover/Movies/${media.id}/fanart-1280.jpg) center 10%/cover no-repeat`;
  }

  return (
    <div className="media-info-lane-outer" style={{ background }}>
      <div className="media-info-poster">
        <img
          className="media-info-poster-img"
          src={
            mediaType === "tv"
              ? `/mediacover/Series/${media.id}/poster-500.jpg`
              : `/mediacover/Movies/${media.id}/poster-500.jpg`
          }
          alt={`${media?.title ?? "Media"} poster`}
          onError={(e) => {
            e.target.onerror = null;
            e.target.src = "/icons/logo.svg";
          }}
        />
      </div>
      <div className="media-info-content">
        <h2 className="media-info-title">
          <IconButton
            className="media-info-bookmark"
            icon={<FontAwesomeIcon icon={faBookmark} color="#eee" />}
            disabled
          />
          {/* provider buttons are rendered with the rating below */}
          <span className="media-info-title-group">
            <span>{media.title}</span>
            {(() => {
              const raw = media.alternateTitles || [];
              const altArr = raw.map((item) =>
                typeof item === "string"
                  ? item
                  : item.title ||
                    item.name ||
                    item.Title ||
                    JSON.stringify(item),
              );
              const original =
                media.original_title ||
                media.originalTitle ||
                media.OriginalTitle ||
                "";
              const norm = (s) => (s || "").toString().trim();
              const displayed = norm(media.title || "");
              const hasAlts =
                Array.isArray(media.alternateTitles) &&
                media.alternateTitles.length > 0;
              const seen = new Set();
              const filteredAlt = altArr
                .map((a) => norm(a))
                .filter((a) => {
                  if (!a) return false;
                  if (a === displayed) return false;
                  if (seen.has(a)) return false;
                  seen.add(a);
                  return true;
                });
              const origNorm = norm(original);
              const showOriginal =
                origNorm && origNorm !== displayed && !seen.has(origNorm);
              const showIcon = hasAlts || showOriginal;
              if (!showIcon) return null;
              return (
                <span className="media-info-alt-wrapper">
                  <button
                    className="media-info-alt-button"
                    aria-label={`${altArr.length} alternate titles`}
                    onMouseEnter={() => setShowAlt(true)}
                    onMouseLeave={() => setShowAlt(false)}
                    onFocus={() => setShowAlt(true)}
                    onBlur={() => setShowAlt(false)}
                  >
                    <FontAwesomeIcon
                      icon={faLanguage}
                      className="media-info-alt-icon"
                    />
                  </button>
                  {showAlt && (
                    <div
                      role="tooltip"
                      className={`media-info-alt-tooltip ${isDark ? "dark" : "light"}`}
                    >
                      {showOriginal && (
                        <div className="media-info-section">
                          <div className="media-info-section-title">
                            Original title
                          </div>
                          <ul className="media-info-list media-info-list-with-gap">
                            <li className="media-info-list-item">{original}</li>
                          </ul>
                        </div>
                      )}
                      {filteredAlt.length > 0 && (
                        <div className="media-info-section">
                          <div className="media-info-section-title">
                            Alternate titles
                          </div>
                          <ul className="media-info-list">
                            {filteredAlt.map((t) => (
                              <li key={t} className="media-info-list-item">
                                {t}
                              </li>
                            ))}
                          </ul>
                        </div>
                      )}
                    </div>
                  )}
                </span>
              );
            })()}
          </span>
        </h2>
        {media.overview && (
          <div className="media-info-overview">{media.overview}</div>
        )}
        <div className="media-info-meta">
          {media.year} &bull; {media.path}
        </div>
        {/* IMDb rating (if available) — emulate IMDb badge with CSS */}
        {(() => {
          // For series (tv) some items expose a top-level `ratings.value` instead of `ratings.imdb.value`.
          // Use that when mediaType is 'tv' or 'series'; otherwise prefer `ratings.imdb.value`.
          const isSeries = mediaType === "tv" || mediaType === "series";
          const imdbRating = isSeries
            ? media?.ratings?.value ?? media?.ratings?.imdb?.value
            : media?.ratings?.imdb?.value ?? media?.ratings?.value;
          if (!imdbRating) return null;
          // Build provider buttons to appear beside the rating
          const providerButtons = [];
          if (mediaType === "movie" && providerUrl) {
            providerButtons.push(
              <IconButton
                key="radarr"
                icon={
                  <img
                    src="/icons/radarr.svg"
                    alt="Radarr"
                    className="media-info-provider-icon"
                  />
                }
                onClick={() => {
                  const base = providerUrl.replace(/\/$/, "");
                  // Prefer titleSlug for provider links; fall back to id or title
                  const movieSlug = media?.titleSlug ?? media?.id ?? media?.title ?? "";
                  const url = `${base}/movie/${encodeURIComponent(movieSlug)}`;
                  window.open(url, "_blank", "noopener");
                }}
                title="Open in Radarr"
              />,
            );
          }
          if ((mediaType === "tv" || mediaType === "series") && providerUrl) {
            providerButtons.push(
              <IconButton
                key="sonarr"
                icon={
                  <img
                    src="/icons/sonarr.svg"
                    alt="Sonarr"
                    className="media-info-provider-icon"
                  />
                }
                onClick={() => {
                  const base = providerUrl.replace(/\/$/, "");
                  const seriesId = media?.titleSlug ?? media?.id ?? media?.title ?? "";
                  const url = `${base}/series/${encodeURIComponent(seriesId)}`;
                  window.open(url, "_blank", "noopener");
                }}
                title="Open in Sonarr"
              />,
            );
          }

          return (
            <div className="media-info-rating" aria-label={`IMDb rating ${imdbRating}`}>
              <img src="/icons/imdb.svg" alt="IMDb" className="media-info-imdb-img" />
              <span className="media-info-rating-value">{imdbRating}</span>
              {providerButtons.length > 0 && (
                <div className="media-info-providers">{providerButtons}</div>
              )}
            </div>
          );
        })()}
        <div className="media-info-cast">
          <div className="media-info-spacer" />
          {castLoading && (
            <div className="media-info-muted-text">Loading cast...</div>
          )}
          {castError && (
            <div className="media-info-error-text">{castError}</div>
          )}
          {!castLoading && !castError && cast && cast.length > 0 && (
            <ActorRow actors={cast.slice(0, 10)} />
          )}
          {!castLoading && !castError && (!cast || cast.length === 0) && (
            <div className="media-info-muted-text">
              No cast information available.
            </div>
          )}
        </div>
        {/* (imdb.log removed) */}
      </div>
    </div>
  );
}

MediaInfoLane.propTypes = {
  media: PropTypes.object,
  mediaType: PropTypes.oneOf(["movie", "series", "tv"]),
  cast: PropTypes.array,
  castLoading: PropTypes.bool,
  castError: PropTypes.string,
};
