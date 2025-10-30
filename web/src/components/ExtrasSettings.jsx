import React, { useEffect, useState } from "react";
import Select from "react-select";
import axios from "axios";
import Container from "./Container.jsx";
import ActionLane from "./ActionLane.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSave, faSpinner } from "@fortawesome/free-solid-svg-icons";
import SectionHeader from "./SectionHeader.jsx";
import Toast from "./Toast.jsx";
import { isDark, addDarkModeListener } from "../utils/isDark";

import ExtrasTypeMappingConfig from "./ExtrasTypeMappingConfig.jsx";

const EXTRA_TYPES = [
  { key: "trailers", label: "Trailers" },
  { key: "scenes", label: "Scenes" },
  { key: "behindTheScenes", label: "Behind the Scenes" },
  { key: "interviews", label: "Interviews" },
  { key: "featurettes", label: "Featurettes" },
  { key: "deletedScenes", label: "Deleted Scenes" },
  { key: "other", label: "Other" },
];

const YTDLP_FLAGS = [
  { key: "quiet", label: "Quiet (no output)", type: "boolean" },
  { key: "noprogress", label: "No Progress Bar", type: "boolean" },
  { key: "writesubs", label: "Write Subs", type: "boolean" },
  { key: "writeautosubs", label: "Write Auto Subs", type: "boolean" },
  { key: "embedsubs", label: "Embed Subs", type: "boolean" },
  { key: "sublangs", label: "Subtitle Languages", type: "string" },
  { key: "requestedformats", label: "Requested Formats", type: "string" },
  { key: "timeout", label: "Timeout (s)", type: "number" },
  { key: "sleepInterval", label: "Sleep Interval (s)", type: "number" },
  { key: "maxDownloads", label: "Max Downloads", type: "number" },
  { key: "limitRate", label: "Limit Rate", type: "string" },
  { key: "sleepRequests", label: "Sleep Requests", type: "number" },
  { key: "maxSleepInterval", label: "Max Sleep Interval (s)", type: "number" },
];

export default function ExtrasSettings() {
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
    const remove = addDarkModeListener(() => setColors());
    return remove;
  }, [isDark]);

  const [settings, setSettings] = useState({});
  const [ytFlags, setYtFlags] = useState({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [toastSuccess, setToastSuccess] = useState(true);
  const [tmdbTypes, setTmdbTypes] = useState([]);
  const [plexTypes, setPlexTypes] = useState([]);
  const [mapping, setMapping] = useState({});

  // When writesubs is disabled we keep dependent values (writeautosubs, embedsubs, sublangs)
  // so users don't lose their input when toggling the option off. Inputs will be disabled
  // via the render logic but their values remain in state.

  useEffect(() => {
    Promise.all([
      axios.get("/api/tmdb/extratypes"),
      axios.get("/api/settings/extratypes"),
      axios.get("/api/settings/canonicalizeextratype"),
      axios.get("/api/settings/ytdlpflags"),
    ])
      .then(([tmdbRes, plexRes, mapRes, ytRes]) => {
        setTmdbTypes(tmdbRes.data.tmdbExtraTypes);
        if (!Array.isArray(plexRes.data)) {
          setError(
            "Server response missing mapp array in /api/settings/extratypes",
          );
          return;
        }
        const mapp = plexRes.data;
        setPlexTypes(mapp);
        const settingsFromMapp = {};
        for (const entry of mapp) {
          if (entry?.key) settingsFromMapp[entry.key] = !!entry.value;
        }
        const initialMapping = { ...mapRes.data.mapping };
        for (const type of tmdbRes.data.tmdbExtraTypes) {
          if (!initialMapping[type]) initialMapping[type] = "Other";
        }
        setMapping(initialMapping);
        setSettings(settingsFromMapp);
        setYtFlags(ytRes.data);
      })
      .catch(() => setError("Failed to load settings"));
  }, [isDark]);

  const handleMappingChange = (newMapping) => setMapping(newMapping);

  const handleSave = async () => {
    setSaving(true);
    setToast("");
    setError("");
    setYtError("");
    try {
      await axios.post("/api/settings/extratypes", settings);
      await axios.post("/api/settings/canonicalizeextratype", { mapping });
      await axios.post("/api/settings/ytdlpflags", ytFlags);
      setToast("All settings saved successfully!");
      setToastSuccess(true);
    } catch {
      setError("Failed to save one or more settings");
      setToast("Failed to save one or more settings");
      setToastSuccess(false);
    } finally {
      setSaving(false);
    }
  };

  const isChanged =
    EXTRA_TYPES.some(
      ({ key }) => settings[key] !== undefined && settings[key] !== false,
    ) || Object.keys(ytFlags).length > 0;

  return (
    <Container>
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
            showLabel:
              globalThis.window === undefined
                ? true
                : globalThis.window.innerWidth > 900,
          },
        ]}
        error={error}
      />
      <Toast
        message={toast}
        onClose={() => setToast("")}
        success={toastSuccess}
      />

      <div
        style={{
          marginTop: "4.5rem",
          color: "var(--settings-text, #222)",
          borderRadius: 12,
          boxShadow: "0 1px 4px #0001",
          padding: "2rem",
        }}
      >
        <SectionHeader>Extra Types</SectionHeader>
        <div style={{ marginBottom: "2em" }}>
          <Select
            isMulti
            options={EXTRA_TYPES.map(({ key, label }) => ({
              value: key,
              label,
            }))}
            value={EXTRA_TYPES.filter(({ key }) => settings[key]).map(
              ({ key, label }) => ({ value: key, label }),
            )}
            onChange={(selected) => {
              const newSettings = {};
              for (const { key } of EXTRA_TYPES) newSettings[key] = false;
              for (const { value } of selected) newSettings[value] = true;
              setSettings(newSettings);
            }}
            styles={{
              control: (base, state) => ({
                ...base,
                background: isDark ? "#23232a" : "#fff",
                borderColor: state.isFocused ? "#a855f7" : "#444",
                boxShadow: state.isFocused ? "0 0 0 2px #a855f7" : "none",
                color: isDark ? "#fff" : "#222",
                borderRadius: 8,
                minHeight: 32,
                fontSize: 13,
                padding: "0 4px",
                maxWidth: 480,
              }),
              valueContainer: (base) => ({ ...base, padding: "2px 4px" }),
              indicatorsContainer: (base) => ({ ...base, height: 32 }),
              multiValue: (base) => ({
                ...base,
                background: isDark ? "#333" : "#e5e7eb",
                color: isDark ? "#fff" : "#222",
                borderRadius: 6,
                fontSize: 13,
                height: 24,
                margin: "2px 2px",
                display: "flex",
                alignItems: "center",
              }),
              multiValueLabel: (base) => ({
                ...base,
                color: isDark ? "#fff" : "#222",
                fontWeight: 500,
                fontSize: 13,
                padding: "0 6px",
              }),
              multiValueRemove: (base) => ({
                ...base,
                color: isDark ? "#a855f7" : "#6d28d9",
                fontSize: 13,
                height: 24,
                ":hover": {
                  background: isDark ? "#a855f7" : "#6d28d9",
                  color: "#fff",
                },
              }),
              menu: (base) => ({
                ...base,
                background: isDark ? "#23232a" : "#fff",
                color: isDark ? "#fff" : "#222",
                borderRadius: 8,
                fontSize: 13,
              }),
              option: (base, state) => ({
                ...base,
                background: (() => {
                  if (state.isSelected) return isDark ? "#a855f7" : "#6d28d9";
                  if (state.isFocused) return isDark ? "#333" : "#eee";
                  return isDark ? "#23232a" : "#fff";
                })(),
                color: (() => {
                  if (state.isSelected) return "#fff";
                  return isDark ? "#fff" : "#222";
                })(),
                fontWeight: state.isSelected ? 600 : 400,
                fontSize: 13,
                height: 32,
                display: "flex",
                alignItems: "center",
                lineHeight: "normal",
              }),
            }}
            placeholder="Select extra types..."
            closeMenuOnSelect={false}
            hideSelectedOptions={false}
            menuPortalTarget={document.body}
          />
        </div>

        <ExtrasTypeMappingConfig
          mapping={mapping}
          onMappingChange={handleMappingChange}
          tmdbTypes={tmdbTypes}
          plexTypes={plexTypes}
        />

        <hr
          style={{ margin: "2em 0", borderColor: isDark ? "#444" : "#eee" }}
        />

        {/* yt-dlp flags moved to their own settings page */}
      </div>
    </Container>
  );
}
