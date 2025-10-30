import React, { useEffect, useState } from "react";
import axios from "axios";
import Container from "./Container.jsx";
import ActionLane from "./ActionLane.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSave, faSpinner } from "@fortawesome/free-solid-svg-icons";
import SectionHeader from "./SectionHeader.jsx";
import Toast from "./Toast.jsx";
import { isDark, addDarkModeListener } from "../utils/isDark";

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

export default function YtdlpFlagsSettings() {
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
  }, []);

  const [ytFlags, setYtFlags] = useState({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [toastSuccess, setToastSuccess] = useState(true);

  useEffect(() => {
    axios
      .get("/api/settings/ytdlpflags")
      .then((res) => setYtFlags(res.data))
      .catch(() => setError("Failed to load yt-dlp flags"));
  }, []);

  const handleYtFlagChange = (key, value) =>
    setYtFlags((p) => ({ ...p, [key]: value }));

  const handleSave = async () => {
    if (
      typeof ytFlags.maxSleepInterval === "number" &&
      typeof ytFlags.sleepInterval === "number" &&
      ytFlags.maxSleepInterval < ytFlags.sleepInterval
    ) {
      setError("Max Sleep Interval must not be lower than Sleep Interval.");
      return;
    }
    setSaving(true);
    setError("");
    setToast("");
    try {
      await axios.post("/api/settings/ytdlpflags", ytFlags);
      setToast("yt-dlp flags saved");
      setToastSuccess(true);
    } catch {
      setError("Failed to save yt-dlp flags");
      setToast("Failed to save yt-dlp flags");
      setToastSuccess(false);
    } finally {
      setSaving(false);
    }
  };

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
            disabled: saving,
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
        <SectionHeader>yt-dlp Download Flags</SectionHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            handleSave();
          }}
        >
          {YTDLP_FLAGS.map(({ key, label, type }) => {
            const dependentOnWriteSubs =
              key === "writeautosubs" || key === "embedsubs" || key === "sublangs";
            const disabledDueToWriteSubs = dependentOnWriteSubs && ytFlags.writesubs === false;
            return (
              <div
                key={key}
                style={{ display: "flex", alignItems: "center", marginBottom: 16 }}
              >
                {type === "boolean" ? (
                  <>
                    <input
                      type="checkbox"
                      id={key}
                      checked={!!ytFlags[key]}
                      onChange={() => handleYtFlagChange(key, !ytFlags[key])}
                      disabled={disabledDueToWriteSubs}
                      style={{ marginRight: 12, accentColor: isDark ? "#2563eb" : "#6d28d9" }}
                    />
                    <label htmlFor={key} style={{ fontSize: 16 }}>
                      {label}
                    </label>
                  </>
                ) : (
                  <>
                    <label htmlFor={key} style={{ fontSize: 16, minWidth: 180, textAlign: "left", width: 180 }}>
                      {label}
                    </label>
                    <input
                      type={type === "number" ? "number" : "text"}
                      id={key}
                      value={ytFlags[key] ?? ""}
                      onChange={(e) => handleYtFlagChange(key, type === "number" ? Number(e.target.value) : e.target.value)}
                      disabled={disabledDueToWriteSubs}
                      style={{ marginLeft: 12, width: 120, minWidth: 80, maxWidth: 160, padding: "0.15em 0.5em", fontSize: 13, border: "1px solid", borderColor: isDark ? "#444" : "#ccc", borderRadius: 4, background: isDark ? "#23232a" : "#fff", color: isDark ? "#e5e7eb" : "#222" }}
                    />
                  </>
                )}
              </div>
            );
          })}
        </form>
      </div>
    </Container>
  );
}
