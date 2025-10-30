import React, { useEffect, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faFolderOpen } from "@fortawesome/free-solid-svg-icons";

// DirectoryPicker: server-side file browser for folder selection
// Props:
// - value: current directory path
// - onChange: function(newPath)
// - label: optional label
// - disabled: optional
export default function DirectoryPicker({
  value,
  onChange,
  label,
  disabled,
  name,
}) {
  const [folders, setFolders] = useState([]);
  const [currentPath, setCurrentPath] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [showBrowser, setShowBrowser] = useState(false);

  useEffect(() => {
    if (showBrowser) {
      fetchFolders(currentPath);
    }
  }, [showBrowser, currentPath]);

  function fetchFolders(path) {
    setLoading(true);
    setError("");
    fetch(`/api/files/list?path=${encodeURIComponent(path || "")}`)
      .then((res) => res.json())
      .then((data) => {
        setFolders(data.folders || []);
        setLoading(false);
      })
      .catch(() => {
        setError("Failed to load folders");
        setLoading(false);
      });
  }

  function handleSelect(folder) {
    setCurrentPath(folder);
  }

  function handlePick() {
    onChange(currentPath);
    setShowBrowser(false);
  }

  function handleOpen() {
    setShowBrowser(true);
    setCurrentPath("");
  }

  function handleBack() {
    if (!currentPath) return;
    const parts = currentPath.split("/").filter(Boolean);
    if (parts.length <= 1) {
      setCurrentPath("");
    } else {
      setCurrentPath("/" + parts.slice(0, -1).join("/"));
    }
  }

  return (
    <div style={{ marginBottom: "0", width: "100%" }}>
      {label && (
        <label style={{ fontWeight: 500, marginRight: 8 }}>{label}</label>
      )}
      <div
        style={{
          position: "relative",
          display: "flex",
          alignItems: "center",
          width: "100%",
        }}
      >
        <input
          name={name}
          type="text"
          value={value}
          onChange={(e) => !disabled && onChange(e.target.value)}
          disabled={disabled}
          style={{
            width: "100%",
            paddingRight: 32,
            paddingLeft: 8,
            paddingTop: 6,
            paddingBottom: 6,
            borderRadius: 4,
            border: "1px solid #bbb",
            background: "var(--settings-input-bg, #f5f5f5)",
            color: "var(--settings-input-text, #222)",
            fontSize: 15,
            boxSizing: "border-box",
          }}
        />
        <span
          onClick={disabled ? undefined : handleOpen}
          style={{
            position: "absolute",
            right: 8,
            top: "50%",
            transform: "translateY(-50%)",
            cursor: disabled ? "not-allowed" : "pointer",
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
          }}
          title="Browse Server"
        >
          <FontAwesomeIcon icon={faFolderOpen} style={{ fontSize: 20 }} />
        </span>
      </div>
      {showBrowser && (
        <div
          style={{
            position: "absolute",
            zIndex: 1000,
            background: "#222",
            color: "#eee",
            border: "1px solid #444",
            borderRadius: 8,
            padding: "1rem",
            minWidth: 320,
            boxShadow: "0 2px 12px #0008",
            marginTop: 8,
          }}
        >
          <div style={{ marginBottom: 8, fontWeight: 600 }}>Select Folder</div>
          {loading ? (
            <div>Loading...</div>
          ) : error ? (
            <div style={{ color: "#f44" }}>{error}</div>
          ) : (
            <>
              <div style={{ marginBottom: 8 }}>
                <button
                  onClick={handleBack}
                  style={{
                    background: "#333",
                    color: "#fff",
                    border: "none",
                    borderRadius: 4,
                    padding: "0.3rem 0.8rem",
                    marginRight: 8,
                  }}
                >
                  Up
                </button>
                <span style={{ color: "#aaa" }}>{currentPath || "/"}</span>
              </div>
              <div
                style={{ maxHeight: 220, overflowY: "auto", marginBottom: 8 }}
              >
                {folders.map((folder) => (
                  <div
                    key={folder}
                    style={{
                      padding: "0.3rem 0.5rem",
                      cursor: "pointer",
                      borderRadius: 4,
                      background: folder === currentPath ? "#444" : "none",
                      marginBottom: 2,
                    }}
                    onClick={() => handleSelect(folder)}
                  >
                    {folder}
                  </div>
                ))}
                {folders.length === 0 && (
                  <div style={{ color: "#888" }}>No folders</div>
                )}
              </div>
              <button
                onClick={handlePick}
                style={{
                  background: "#0078d7",
                  color: "#fff",
                  border: "none",
                  borderRadius: 6,
                  padding: "0.5rem 1.2rem",
                  marginRight: 8,
                }}
              >
                Select
              </button>
              <button
                onClick={() => setShowBrowser(false)}
                style={{
                  background: "#c00",
                  color: "#fff",
                  border: "none",
                  borderRadius: 6,
                  padding: "0.5rem 1.2rem",
                }}
              >
                Cancel
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
