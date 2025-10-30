import React, { useEffect, useState } from "react";
import { isDark, addDarkModeListener } from "../utils/isDark.js";
import Toast from "./Toast.jsx";
import PropTypes from "prop-types";
import IconButton from "./IconButton.jsx";
import SectionHeader from "./SectionHeader.jsx";
import DirectoryPicker from "./DirectoryPicker.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faFolderOpen,
  faPlug,
  faSave,
  faSpinner,
} from "@fortawesome/free-solid-svg-icons";
import { faTrashAlt } from "@fortawesome/free-regular-svg-icons";
import ActionLane from "./ActionLane.jsx";
import Container from "./Container.jsx";

function mapFoldersToPathMappings(folderPaths, pathMappings) {
  return folderPaths.map((path) => {
    const existing = pathMappings.find((m) => m.from === path);
    return existing || { from: path, to: "" };
  });
}

export default function ProviderSettingsPage({ type }) {
  const [testing, setTesting] = useState(false);
  // Remove testResult state, use toast instead
  // type: 'radarr' or 'sonarr'
  const [originalSettings, setOriginalSettings] = useState(null);
  const [settings, setSettings] = useState({
    url: "",
    apiKey: "",
    pathMappings: [],
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState("");
  const [toastSuccess, setToastSuccess] = useState(true);

  useEffect(() => {
    const setColors = () => {
      document.documentElement.style.setProperty(
        "--settings-bg",
        isDark ? "#222" : "#fff",
      );
      document.documentElement.style.setProperty(
        "--settings-text",
        isDark ? "#eee" : "#222",
      );
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
      document.documentElement.style.setProperty(
        "--settings-table-bg",
        isDark ? "#444" : "#f7f7f7",
      );
      document.documentElement.style.setProperty(
        "--settings-table-text",
        isDark ? "#f3f3f3" : "#222",
      );
      document.documentElement.style.setProperty(
        "--settings-table-header-bg",
        isDark ? "#555" : "#ededed",
      );
      document.documentElement.style.setProperty(
        "--settings-table-header-text",
        isDark ? "#fff" : "#222",
      );
    };
    setColors();
    const remove = addDarkModeListener(setColors);
    return remove;
  }, []);

  useEffect(() => {
    setLoading(true);
    fetch(`/api/settings/${type}`)
      .then((res) => res.json())
      .then(async (data) => {
        // Immediately set settings so UI can render fast.
        let pathMappings = Array.isArray(data.pathMappings)
          ? data.pathMappings
          : [];
        const normalized = {
          ...data,
          pathMappings,
        };
        setSettings(normalized);
        setOriginalSettings(normalized);
        setLoading(false);

        // Fetch provider root folders in background and merge path mappings when available.
        if (data.providerURL && data.apiKey) {
          try {
            const res = await fetch(
              `/api/rootfolders?providerURL=${encodeURIComponent(data.providerURL)}&apiKey=${encodeURIComponent(data.apiKey)}&type=${type}`,
            );
            if (!res.ok) return;
            const folders = await res.json();
            if (folders && folders.length > 0) {
              const folderPaths = folders.map((f) => f.path || f);
              const merged = mapFoldersToPathMappings(folderPaths, pathMappings);
              // Update settings and originalSettings only if mappings changed.
              setSettings((s) => ({ ...s, pathMappings: merged }));
              setOriginalSettings((s) => ({ ...s, pathMappings: merged }));
            }
          } catch {
            // ignore background fetch errors
          }
        }
      });
  }, [type]);

  // Clear test status when switching between Radarr/Sonarr pages
  useEffect(() => {
    setTesting(false);
  }, [type]);

  function isSettingsChanged() {
    if (!originalSettings) return false;
    // Compare providerURL and apiKey (not url)
    if ((settings.providerURL || "") !== (originalSettings.providerURL || ""))
      return true;
    if ((settings.apiKey || "") !== (originalSettings.apiKey || ""))
      return true;
    // Compare pathMappings deeply
    const a = Array.isArray(settings.pathMappings) ? settings.pathMappings : [];
    const b = Array.isArray(originalSettings.pathMappings)
      ? originalSettings.pathMappings
      : [];
    if (a.length !== b.length) return true;
    for (let i = 0; i < a.length; i++) {
      if (
        (a[i].from || "") !== (b[i].from || "") ||
        (a[i].to || "") !== (b[i].to || "")
      )
        return true;
    }
    return false;
  }

  const handleChange = (e) => {
    setSettings({ ...settings, [e.target.name]: e.target.value });
  };

  const handleMappingChange = (idx, field, value) => {
    const updated = settings.pathMappings.map((m, i) =>
      i === idx ? { ...m, [field]: value } : m,
    );
    setSettings({ ...settings, pathMappings: updated });
  };

  const removeMapping = (idx) => {
    setSettings({
      ...settings,
      pathMappings: settings.pathMappings.filter((_, i) => i !== idx),
    });
  };

  const saveSettings = async () => {
    setSaving(true);
    setToast("");
    try {
      const res = await fetch(`/api/settings/${type}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      });
      if (res.ok) {
        setToast("Settings saved successfully!");
        setToastSuccess(true);
        setOriginalSettings(settings);
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

  const fetchAndUpdateRootFolders = async () => {
    try {
      const foldersRes = await fetch(
        `/api/rootfolders?providerURL=${encodeURIComponent(settings.providerURL)}&apiKey=${encodeURIComponent(settings.apiKey)}&type=${type}`,
      );
      if (!foldersRes.ok) return;
      const folders = await foldersRes.json();
      // Update pathMappings in settings and originalSettings
      const folderPaths = folders.map((f) => f.path || f);
      let pathMappings = Array.isArray(settings.pathMappings)
        ? settings.pathMappings
        : [];
      pathMappings = folderPaths.map((path) => {
        const existing = pathMappings.find((m) => m.from === path);
        return existing || { from: path, to: "" };
      });
      setSettings((s) => ({ ...s, pathMappings }));
      setOriginalSettings((s) => ({ ...s, pathMappings }));
    } catch {
      // ignore
    }
  };

  const testConnection = async () => {
    setTesting(true);
    setToast("");
    try {
      const res = await fetch(
        `/api/test/${type}?url=${encodeURIComponent(settings.providerURL)}&apiKey=${encodeURIComponent(settings.apiKey)}`,
      );
      if (!res.ok) {
        setToast("Connection failed.");
        setToastSuccess(false);
        return;
      }
      const data = await res.json();
      if (!data.success) {
        setToast(data.error || "Connection failed.");
        setToastSuccess(false);
        return;
      }
      setToast("Connection successful!");
      setToastSuccess(true);
      await fetchAndUpdateRootFolders();
    } catch {
      setToast("Connection failed.");
      setToastSuccess(false);
    } finally {
      setTesting(false);
    }
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
            onClick: saveSettings,
            disabled: saving || !isSettingsChanged(),
            loading: saving,
            showLabel:
              globalThis.window === undefined
                ? true
                : globalThis.window.innerWidth > 900,
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
          background: "var(--settings-bg, #fff)",
          color: "var(--settings-text, #222)",
          borderRadius: 12,
          boxShadow: "0 1px 4px #0001",
          padding: "2rem",
        }}
      >
        {!loading && (
          <>
            <div
              style={{
                marginBottom: "1.5rem",
                display: "block",
                width: "100%",
              }}
            >
              <div style={{ width: "100%", marginBottom: "1.2rem" }}>
                <label
                  style={{
                    fontWeight: 600,
                    fontSize: "1.15em",
                    marginBottom: 6,
                    display: "block",
                    textAlign: "left",
                  }}
                >
                  {type === "radarr" ? "Radarr URL" : "Sonarr URL"}
                  <br />
                  <input
                    name="providerURL"
                    value={settings.providerURL}
                    onChange={handleChange}
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
                </label>
              </div>
              <div
                style={{
                  width: "100%",
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "flex-start",
                  gap: "0.7rem",
                }}
              >
                <label
                  style={{
                    fontWeight: 600,
                    fontSize: "1.15em",
                    marginBottom: 6,
                    display: "block",
                    textAlign: "left",
                    width: "100%",
                  }}
                >
                  API Key
                  <br />
                  <input
                    name="apiKey"
                    value={settings.apiKey}
                    onChange={handleChange}
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
                </label>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: "1rem",
                    width: "100%",
                  }}
                >
                  <IconButton
                    onClick={testConnection}
                    disabled={
                      testing || !settings.providerURL || !settings.apiKey
                    }
                    title="Test Connection"
                    aria-label="Test Connection"
                    style={{
                      cursor:
                        testing || !settings.providerURL || !settings.apiKey
                          ? "not-allowed"
                          : "pointer",
                      opacity:
                        testing || !settings.providerURL || !settings.apiKey
                          ? 0.6
                          : 1,
                      display: "inline-flex",
                      alignItems: "center",
                      justifyContent: "center",
                      background: "none",
                      border: "none",
                      padding: 0,
                      margin: 0,
                      outline: "none",
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
                            color: "var(--settings-text, #222)",
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
            </div>
            <SectionHeader>Path Mappings</SectionHeader>
            <table
              style={{
                width: "100%",
                minWidth: 300,
                maxWidth: 620,
                marginLeft: 0,
                marginRight: "auto",
                borderCollapse: "collapse",
                background: "var(--settings-table-bg, #f5f5f5)",
                borderRadius: 8,
                overflow: "hidden",
                marginTop: "1rem",
                color: "var(--settings-table-text, #222)",
              }}
            >
              <thead>
                <tr
                  style={{
                    background: "var(--settings-table-header-bg, #eaeaea)",
                    color: "var(--settings-table-header-text, #222)",
                  }}
                >
                  <th style={{ padding: "0.5rem", textAlign: "left" }}>
                    {type === "radarr" ? "Radarr Path" : "Sonarr Path"}
                  </th>
                  <th style={{ padding: "0.5rem", textAlign: "left" }}>
                    Trailarr Path
                  </th>
                  <th style={{ padding: "0.5rem" }}></th>
                </tr>
              </thead>
              <tbody>
                {(Array.isArray(settings.pathMappings)
                  ? settings.pathMappings
                  : []
                ).map((m, i) => (
                  <tr key={m.from + "-" + i}>
                    <td style={{ textAlign: "left", width: "45%" }}>
                      <input
                        name={`pathMappingFrom-${i}`}
                        value={m.from}
                        onChange={(e) =>
                          handleMappingChange(i, "from", e.target.value)
                        }
                        placeholder={
                          type === "radarr" ? "Radarr path" : "Sonarr path"
                        }
                        style={{
                          width: "90%",
                          maxWidth: 500,
                          padding: "0.4rem",
                          borderRadius: 4,
                          border: "1px solid #bbb",
                          background: "var(--settings-input-bg, #f5f5f5)",
                          color: "var(--settings-input-text, #222)",
                        }}
                      />
                    </td>
                    <td style={{ textAlign: "left", width: "45%" }}>
                      <div
                        style={{
                          display: "flex",
                          alignItems: "center",
                          gap: "0.5rem",
                          height: "100%",
                        }}
                      >
                        <DirectoryPicker
                          name={`pathMappingTo-${i}`}
                          value={m.to}
                          onChange={(path) =>
                            handleMappingChange(i, "to", path)
                          }
                          label={null}
                          disabled={saving || loading}
                          icon={
                            <IconButton
                              icon={
                                <FontAwesomeIcon
                                  icon={faFolderOpen}
                                  style={{
                                    fontSize: 20,
                                    background: "none",
                                    padding: 0,
                                    margin: 0,
                                    border: "none",
                                  }}
                                />
                              }
                              disabled
                              style={{
                                background: "none",
                                padding: 0,
                                margin: 0,
                                border: "none",
                              }}
                            />
                          }
                        />
                      </div>
                    </td>
                    <td style={{ textAlign: "left" }}>
                      <IconButton
                        onClick={() => removeMapping(i)}
                        title="Remove"
                        aria-label="Remove path mapping"
                        style={{
                          cursor: "pointer",
                          display: "inline-flex",
                          alignItems: "center",
                          height: "100%",
                          justifyContent: "center",
                          verticalAlign: "middle",
                        }}
                        icon={
                          <FontAwesomeIcon
                            icon={faTrashAlt}
                            style={{
                              fontSize: 20,
                              color: "var(--settings-text, #222)",
                              filter: "drop-shadow(0 1px 2px #0002)",
                              alignSelf: "center",
                            }}
                          />
                        }
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </div>
    </Container>
  );
}

ProviderSettingsPage.propTypes = {
  type: PropTypes.oneOf(["radarr", "sonarr"]).isRequired,
};
