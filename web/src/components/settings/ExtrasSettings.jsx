import React, { useEffect, useState } from "react";
import Select from "react-select";
import axios from "axios";
import Container from "../layout/Container.jsx";
import ActionLane from "../ui/ActionLane.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSave, faSpinner } from "@fortawesome/free-solid-svg-icons";
import SectionHeader from "../layout/SectionHeader.jsx";
import Toast from "../layout/Toast.jsx";
import { isDark, addDarkModeListener } from "../../utils/isDark";
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

export default function ExtrasSettings() {
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
              container: (base) => ({ ...base, maxWidth: 480 }),
              control: (base, state) => ({
                ...base,
                background: "var(--settings-input-bg)",
                color: "var(--settings-input-text)",
                // Invert the non-focused dark/light colors so dark mode gets a lighter border
                borderColor: state.isFocused
                  ? isDark
                    ? "#fff"
                    : "#6d28d9"
                  : isDark
                    ? "#ccc"
                    : "#444",
                minHeight: 42,
                outline: "none",
                borderRadius: 12,
                boxShadow: state.isFocused
                  ? isDark
                    ? "0 0 0 4px rgba(255,255,255,0.12)"
                    : "0 0 0 4px rgba(109,40,217,0.12)"
                  : "none",
              }),
              menuPortal: (base) => ({ ...base, zIndex: 9999 }),
              menu: (base) => ({
                ...base,
                background: "var(--settings-input-bg)",
                borderRadius: 12,
                boxShadow: "0 8px 20px rgba(0,0,0,0.12)",
              }),
              menuList: (base) => ({
                ...base,
                background: "var(--settings-input-bg)",
                borderRadius: 10,
                padding: 4,
              }),
              option: (base, state) => ({
                ...base,
                background: state.isSelected
                  ? isDark
                    ? "#2b2b2b"
                    : "#e6e6ff"
                  : state.isFocused
                    ? isDark
                      ? "rgba(255,255,255,0.04)"
                      : "rgba(109,40,217,0.06)"
                    : "transparent",
                color: "var(--settings-text)",
                borderRadius: 6,
              }),
              multiValue: (base) => ({
                ...base,
                background: isDark ? "#333" : "#e9e9ff",
                borderRadius: 12,
              }),
              multiValueLabel: (base) => ({
                ...base,
                color: "var(--settings-text)",
              }),
              singleValue: (base) => ({
                ...base,
                color: "var(--settings-text)",
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
      </div>
    </Container>
  );
}
