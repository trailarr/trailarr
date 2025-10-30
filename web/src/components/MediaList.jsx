import React, { useMemo } from "react";
import "./MediaList.css";
import { Link } from "react-router-dom";
import MediaCard from "./MediaCard.jsx";
import PropTypes from "prop-types";
import { isDark } from "../utils/isDark";

// basePath: e.g. '/wanted/movies' or '/movies'
export default function MediaList({ items, type, basePath, loading }) {
  // Prepare a sorted copy of items by sortTitle (case-insensitive). Fall back to title when sortTitle is missing.
  const sortedItems = useMemo(() => {
    return (items || []).slice().sort((a, b) => {
      const aKey = (a.sortTitle || a.title || "").toString().toLowerCase();
      const bKey = (b.sortTitle || b.title || "").toString().toLowerCase();
      if (aKey < bKey) return -1;
      if (aKey > bKey) return 1;
      return 0;
    });
  }, [items]);

  // Only show the empty banner if items is an array and is empty, and not loading
  const showEmptyBanner =
    Array.isArray(items) && items.length === 0 && !loading;

  // Extract the nested ternary into an independent statement
  const gridContent =
    !loading && items && items.length > 0 ? (
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
        {sortedItems.map((item) => {
          let linkTo;
          if (basePath) {
            linkTo = `${basePath}/${item.id}`;
          } else if (type === "series") {
            linkTo = `/series/${item.id}`;
          } else {
            linkTo = `/movies/${item.id}`;
          }
          return (
            <div
              key={item.id + "-" + type}
              style={{
                background: isDark ? "#23232a" : "#fff",
                borderRadius: 12,
                boxShadow: isDark ? "0 2px 8px #18181b" : "0 2px 8px #e5e7eb",
                padding: "0.85rem",
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                height: 410,
                transition: "box-shadow 0.2s",
                border: isDark ? "1px solid #333" : "1px solid #eee",
                overflow: "hidden",
                boxSizing: "border-box",
              }}
            >
              <Link
                to={linkTo}
                style={{
                  width: "100%",
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "center",
                  textDecoration: "none",
                  height: "100%",
                }}
              >
                <div
                  style={{
                    width: "100%",
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    height: "100%",
                  }}
                >
                  <MediaCard media={item} mediaType={type} />
                  <div style={{ flex: 1 }} />
                </div>
              </Link>
            </div>
          );
        })}
      </div>
    ) : null;

  return (
    <div style={{ width: "100%" }}>
      {showEmptyBanner ? (
        <div
          style={{
            minHeight: "calc(100vh - 120px)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            padding: "1.5rem",
            boxSizing: "border-box",
          }}
        >
          <div
            style={{
              textAlign: "center",
              color: isDark ? "#ddd" : "#333",
              background: isDark ? "#121214" : "#fbfbfb",
              border: isDark ? "1px solid #222" : "1px solid #eee",
              padding: "1.25rem 1.5rem",
              borderRadius: 10,
              maxWidth: 800,
              width: "auto",
              margin: "0 auto",
            }}
          >
            <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 6 }}>
              No media found
            </div>
            <div style={{ fontSize: 14, opacity: 0.85 }}>
              There are no items to show here. Try scanning your libraries,
              check your path mappings, or adjust filters.
            </div>
          </div>
        </div>
      ) : (
        gridContent
      )}
    </div>
  );
}

MediaList.propTypes = {
  items: PropTypes.arrayOf(PropTypes.object),
  type: PropTypes.string,
  basePath: PropTypes.string,
  loading: PropTypes.bool,
};

MediaList.defaultProps = {
  items: [],
  type: "movies",
  basePath: "",
  loading: false,
};
