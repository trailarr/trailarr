import React, { useEffect, useState } from "react";
import { isDark, addDarkModeListener } from "../utils/isDark";
import IconButton from "./IconButton.jsx";
import SectionHeader from "./SectionHeader.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faPlug, faSave, faSpinner } from "@fortawesome/free-solid-svg-icons";
import ActionLane from "./ActionLane.jsx";
import Container from "./Container.jsx";
import Toast from "./Toast.jsx";

export default function GeneralSettings() {
  const [testing, setTesting] = useState(false);
  // Remove testResult state, use toast instead
  const [tmdbKey, setTmdbKey] = useState("");
  const [autoDownloadExtras, setAutoDownloadExtras] = useState(true);
  const [logLevel, setLogLevel] = useState("Debug");
  const [frontendUrl, setFrontendUrl] = useState("");
  const [originalKey, setOriginalKey] = useState("");
  const [originalAutoDownload, setOriginalAutoDownload] = useState(true);
  const [originalLogLevel, setOriginalLogLevel] = useState("Debug");
  const [originalFrontendUrl, setOriginalFrontendUrl] = useState("");
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState("");
  const [toastSuccess, setToastSuccess] = useState(true);
  useEffect(() => {
    const setColors = () => {
      document.documentElement.style.setProperty(
        "--save-lane-bg",
        isDark ? "#333" : "#e5e7eb",
      );
      document.documentElement.style.setProperty(
        "--save-lane-text",
        isDark ? "#eee" : "#222",
      );
      document.documentElement.style.setProperty(
        "--settings-input-bg",
        isDark ? "#333" : "#f5f5f5",
      );
      document.documentElement.style.setProperty(
        "--settings-input-text",
        isDark ? "#eee" : "#222",
      );
    };
    setColors();
    const remove = addDarkModeListener(setColors);
    return remove;
  }, []);
  useEffect(() => {
    fetch("/api/settings/general")
      .then((r) => r.json())
      .then((data) => {
        setTmdbKey(data.tmdbKey || "");
        setOriginalKey(data.tmdbKey || "");
        setAutoDownloadExtras(data.autoDownloadExtras !== false); // default true
        setOriginalAutoDownload(data.autoDownloadExtras !== false);
        setLogLevel(data.logLevel || "Debug");
        setOriginalLogLevel(data.logLevel || "Debug");
        setFrontendUrl(data.frontendUrl || "");
        setOriginalFrontendUrl(data.frontendUrl || "");
      });
  }, []);
  const isChanged =
    tmdbKey !== originalKey ||
    autoDownloadExtras !== originalAutoDownload ||
    logLevel !== originalLogLevel ||
    frontendUrl !== originalFrontendUrl;

  const testTmdbKey = async () => {
    setTesting(true);
    setToast("");
    try {
      const res = await fetch(
        `/api/test/tmdb?apiKey=${encodeURIComponent(tmdbKey)}`,
      );
      if (res.ok) {
        const data = await res.json();
        if (data.success) {
          setToast("Connection successful!");
          setToastSuccess(true);
        } else {
          setToast(data.error || "Connection failed.");
          setToastSuccess(false);
        }
      } else {
        setToast("Connection failed.");
        setToastSuccess(false);
      }
    } catch {
      setToast("Connection failed.");
      setToastSuccess(false);
    }
    setTesting(false);
  };

  const handleSave = async () => {
    setSaving(true);
    setToast("");
    try {
      const res = await fetch("/api/settings/general", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tmdbKey, autoDownloadExtras, logLevel, frontendUrl }),
      });
      if (res.ok) {
        setToast("Settings saved successfully!");
        setToastSuccess(true);
        setOriginalKey(tmdbKey);
        setOriginalAutoDownload(autoDownloadExtras);
        setOriginalLogLevel(logLevel);
        setOriginalFrontendUrl(frontendUrl);
      } else {
        setToast("Error saving settings.");
        setToastSuccess(false);
      }
    } catch {
      setToast("Error saving settings.");
      setToastSuccess(false);
    }
    setSaving(false);
  };
  return (
    <Container>
      {/* Save lane */}
      <ActionLane
        buttons={[
          {
            icon: saving ? (
              <FontAwesomeIcon icon={faSpinner} spin />
            ) : (
              <FontAwesomeIcon icon={faSave} />
            ),
            label: "Save",
            onClick: handleSave,
            disabled: saving || !isChanged,
            loading: saving,
            showLabel: globalThis.window && globalThis.window.innerWidth > 900,
          },
        ]}
        error={""}
      />
      <Toast
        message={toast}
        onClose={() => setToast("")}
        success={toastSuccess}
      />
      <div
        style={{
          marginTop: "4.5rem",
          color: isDark ? "#f3f4f6" : "#23232a",
          borderRadius: 12,
          boxShadow: "0 1px 4px #0001",
          padding: "2rem",
        }}
      >
        <SectionHeader>TMDB API Key</SectionHeader>
        <div style={{ width: "100%" }}>
          <fieldset
            style={{
              border: 0,
              padding: 0,
              margin: 0,
              width: "100%",
              alignItems: "flex-start",
              display: "flex",
              flexDirection: "column",
            }}
          >
            <legend
              style={{
                fontWeight: 600,
                fontSize: "1.15em",
                marginBottom: 6,
                textAlign: "left",
              }}
            >
              TMDB API Key
            </legend>
            <input
              id="tmdbKey"
              type="text"
              value={tmdbKey}
              onChange={(e) => setTmdbKey(e.target.value)}
              style={{
                width: "100%",
                minWidth: 220,
                maxWidth: 600,
                padding: "0.5rem",
                borderRadius: 6,
                border: "1px solid #bbb",
                background: "var(--settings-input-bg, #f5f5f5)",
                color: "var(--settings-input-text, #222)",
                marginBottom: "0.7rem",
              }}
            />
            <div
              style={{
                marginTop: "0.7rem",
                display: "flex",
                flexDirection: "column",
                alignItems: "flex-start",
                gap: "0.5rem",
                width: "60%",
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "1rem",
                  width: "100%",
                }}
              >
                <IconButton
                  title="Test TMDB Key"
                  aria-label="Test TMDB Key"
                  onClick={testTmdbKey}
                  disabled={testing || !tmdbKey}
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    background: "none",
                    border: "none",
                    padding: 0,
                    margin: 0,
                    outline: "none",
                    opacity: testing || !tmdbKey ? 0.6 : 1,
                  }}
                  icon={
                    <span
                      style={{
                        position: "relative",
                        display: "inline-block",
                        width: 22,
                        height: 22,
                      }}
                    >
                      <FontAwesomeIcon
                        icon={faPlug}
                        style={{
                          fontSize: 22,
                          color: isDark ? "#f3f4f6" : "#23232a",
                          transition: "color 0.2s",
                          position: "absolute",
                          left: 0,
                          top: 0,
                        }}
                      />
                      {/* No icon overlay, feedback is now via toast */}
                    </span>
                  }
                />
                {/* No inline feedback, feedback is now via toast */}
              </div>
            </div>
          </fieldset>
        </div>
        <SectionHeader>Frontend URL</SectionHeader>
        <div
          style={{
            width: "100%",
            alignItems: "flex-start",
            display: "flex",
            flexDirection: "column",
            gap: "0.5rem",
          }}
        >
          <label
            htmlFor="frontendUrl"
            style={{
              fontWeight: 600,
              fontSize: "1.15em",
              marginBottom: 6,
              display: "block",
              textAlign: "left",
            }}
          >
            Frontend URL
          </label>
          <input
            id="frontendUrl"
            type="text"
            value={frontendUrl}
            onChange={(e) => setFrontendUrl(e.target.value)}
            placeholder="http://localhost:8080"
            style={{
              width: "60%",
              minWidth: 220,
              maxWidth: 600,
              padding: "0.5rem",
              borderRadius: 6,
              border: "1px solid #bbb",
              background: "var(--settings-input-bg, #f5f5f5)",
              color: "var(--settings-input-text, #222)",
            }}
          />
        </div>

        <SectionHeader>Log Level</SectionHeader>
        <div
          style={{
            width: "100%",
            alignItems: "flex-start",
            display: "flex",
            flexDirection: "column",
            gap: "0.5rem",
          }}
        >
          <label
            htmlFor="logLevel"
            style={{
              fontWeight: 600,
              fontSize: "1.15em",
              marginBottom: 6,
              display: "block",
              textAlign: "left",
            }}
          >
            Log Level
          </label>
          <select
            id="logLevel"
            value={logLevel}
            onChange={(e) => setLogLevel(e.target.value)}
            style={{
              width: "60%",
              minWidth: 120,
              maxWidth: 300,
              padding: "0.5rem",
              borderRadius: 6,
              border: "1px solid #bbb",
              background: "var(--settings-input-bg, #f5f5f5)",
              color: "var(--settings-input-text, #222)",
            }}
          >
            <option value="Debug">Debug</option>
            <option value="Info">Info</option>
            <option value="Warn">Warn</option>
            <option value="Error">Error</option>
          </select>
        </div>
        <SectionHeader>Extras Download</SectionHeader>
        <div style={{ width: "100%" }}>
          <input
            id="autoDownloadExtras"
            type="checkbox"
            checked={autoDownloadExtras}
            onChange={(e) => setAutoDownloadExtras(e.target.checked)}
            style={{ marginRight: 8, display: "block" }}
          />
          <label
            htmlFor="autoDownloadExtras"
            style={{
              fontWeight: 600,
              fontSize: "1.15em",
              textAlign: "left",
              display: "block",
              margin: 0,
            }}
          >
            Enable automatic download of extras
          </label>
        </div>
      </div>
    </Container>
  );
}
