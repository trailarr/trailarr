import React, { useEffect, useState } from "react";
import PropTypes from "prop-types";
import MediaList from "../media/MediaList";
import { isDark } from "../../utils/isDark";

export default function Wanted({
  type,
  items: itemsProp,
  loading: loadingProp,
  error: errorProp,
}) {
  const [items, setItems] = useState(() =>
    itemsProp === undefined ? [] : itemsProp,
  );
  const [loading, setLoading] = useState(() =>
    loadingProp === undefined ? true : loadingProp,
  );
  const [error, setError] = useState(() =>
    errorProp === undefined ? "" : errorProp,
  );
  const [showAll, setShowAll] = useState(false);
  useEffect(() => {
    if (itemsProp !== undefined) setItems(itemsProp);
  }, [itemsProp]);

  useEffect(() => {
    if (loadingProp !== undefined) setLoading(loadingProp);
  }, [loadingProp]);

  useEffect(() => {
    if (errorProp !== undefined) setError(errorProp);
  }, [errorProp]);

  useEffect(() => {
    if (itemsProp !== undefined) return;
    let cancelled = false;
    async function fetchWanted() {
      setLoading(true);
      setError("");
      try {
        const endpoint =
          type === "movie" ? "/api/movies/wanted" : "/api/series/wanted";
        const res = await fetch(endpoint);
        const data = await res.json();
        if (cancelled) return;
        setItems(data.items || []);
      } catch {
        if (cancelled) return;
        setError(
          "Failed to fetch wanted " + (type === "movie" ? "movies" : "series"),
        );
      }
      if (!cancelled) setLoading(false);
    }
    fetchWanted();
    return () => {
      cancelled = true;
    };
  }, [type, itemsProp]);

  const renderSkeleton = () => {
    const placeholders = new Array(8).fill(0);
    return (
      <div
        className="media-list-grid"
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, 220px)",
          gridAutoRows: "1fr",
          justifyContent: "start",
          gap: "2rem 1.5rem",
          padding: "1.5rem 1rem",
          width: "100%",
          boxSizing: "border-box",
          alignItems: "start",
        }}
      >
        {placeholders.map((_, i) => (
          <div
            key={"skeleton-" + i}
            style={{
              background: isDark ? "#23232a" : "#fff",
              borderRadius: 12,
              padding: "0.85rem",
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              height: 410,
              boxSizing: "border-box",
              border: isDark ? "1px solid #333" : "1px solid #eee",
            }}
          >
            <div
              style={{
                width: "100%",
                height: 260,
                background: isDark ? "#111" : "#f0f0f0",
                borderRadius: 8,
              }}
            />
            <div style={{ height: 12 }} />
            <div
              style={{
                width: "70%",
                height: 14,
                background: isDark ? "#202124" : "#e8e8e8",
                borderRadius: 6,
              }}
            />
            <div style={{ height: 8 }} />
            <div
              style={{
                width: "40%",
                height: 12,
                background: isDark ? "#202124" : "#e8e8e8",
                borderRadius: 6,
              }}
            />
          </div>
        ))}
      </div>
    );
  };

  return (
    <div style={{ padding: "0em 0em", width: "100%" }}>
      {loading ? renderSkeleton() : null}
      {error && <div style={{ color: "red" }}>{error}</div>}
      <MediaList
        items={showAll ? items : items.slice(0, 200)}
        type={type}
        basePath={type === "series" ? "/wanted/series" : "/wanted/movies"}
        loading={loading}
      />
      {!loading && items && items.length > 200 && (
        <div style={{ padding: "0.5rem 1rem" }}>
          <button
            onClick={() => setShowAll((s) => !s)}
            style={{ padding: "0.5rem 1rem", cursor: "pointer" }}
          >
            {showAll
              ? `Show less (${items.length})`
              : `Show all (${items.length})`}
          </button>
        </div>
      )}
    </div>
  );
}

Wanted.propTypes = {
  type: PropTypes.oneOf(["movie", "series"]).isRequired,
  items: PropTypes.arrayOf(PropTypes.object),
  loading: PropTypes.bool,
  error: PropTypes.string,
};
