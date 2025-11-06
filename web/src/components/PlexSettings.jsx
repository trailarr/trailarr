import React, { useEffect, useState } from "react";
import { isDark, addDarkModeListener } from "../utils/isDark.js";
import Toast from "./Toast.jsx";
import SectionHeader from "./SectionHeader.jsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faSave, faSpinner } from "@fortawesome/free-solid-svg-icons";
import ActionLane from "./ActionLane.jsx";
import Container from "./Container.jsx";
import PropTypes from "prop-types";

// Small standalone button used by PlexSettings. Kept as a top-level component so
// lint rules about component definitions are satisfied.
const OAuthButton = ({ onClick, disabled }) => {
  const [hover, setHover] = useState(false);
  const plexColor = "#E5A00D"; // Plex corporate orange
  const plexHover = "#C48A00";

  const btnStyle = {
    padding: "0.5rem 1rem",
    borderRadius: 6,
    border: "1px solid transparent",
    background: hover ? plexHover : plexColor,
    color: "#fff",
    cursor: "pointer",
    fontSize: "0.95em",
    fontWeight: 600,
    display: "inline-flex",
    alignItems: "center",
    gap: "0.5rem",
    transition: "background 0.12s ease, transform 0.06s",
    transform: hover ? "translateY(-1px)" : "none",
    outline: "none",
    boxShadow: hover ? "0 2px 6px rgba(0,0,0,0.12)" : "0 1px 3px rgba(0,0,0,0.08)",
  };


  return (
    <button
      onClick={() => {
        if (disabled) return;
        onClick?.();
      }}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        ...btnStyle,
        opacity: disabled ? 0.6 : 1,
        cursor: disabled ? "not-allowed" : btnStyle.cursor,
      }}
      aria-label="Get Plex Token"
      title="Get Plex Token"
      disabled={disabled}
    >
  <img src="/icons/plex.svg" alt="Plex" style={{ width: 22, height: 22, display: "inline-block", filter: "drop-shadow(0 1px 0 rgba(0,0,0,0.06))" }} />
      <span>Get Plex Token</span>
    </button>
  );
};

OAuthButton.propTypes = {
  onClick: PropTypes.func,
  disabled: PropTypes.bool,
};

export default function PlexSettings() {
  const [originalSettings, setOriginalSettings] = useState(null);
  const [settings, setSettings] = useState({
    protocol: "http",
    ip: "localhost",
    port: 32400,
    token: "",
    clientId: "",
    enabled: false,
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
    };
    setColors();
    const remove = addDarkModeListener(setColors);
    return remove;
  }, []);

  useEffect(() => {
    setLoading(true);
    fetch("/api/settings/plex")
      .then((res) => res.json())
      .then((data) => {
        const normalized = {
          protocol: data.protocol || "http",
          ip: data.ip || "localhost",
          port: data.port || 32400,
          token: data.token || "",
          clientId: data.clientId || "",
          enabled: data.enabled || false,
        };
        setSettings(normalized);
        setOriginalSettings(normalized);
        setLoading(false);
      })
      .catch(() => {
        setLoading(false);
      });
  }, []);

  function isSettingsChanged() {
    if (!originalSettings) return false;
    return (
      settings.protocol !== originalSettings.protocol ||
      settings.ip !== originalSettings.ip ||
      settings.port !== originalSettings.port ||
      settings.token !== originalSettings.token ||
      settings.clientId !== originalSettings.clientId ||
      settings.enabled !== originalSettings.enabled
    );
  }

  const handleChange = (e) => {
    const { name, value, type, checked } = e.target;
    setSettings({
      ...settings,
      [name]: type === "checkbox" ? checked : value,
    });
  };

  const handlePortChange = (e) => {
    const port = Number.parseInt(e.target.value, 10) || 32400;
    setSettings({ ...settings, port });
  };

  const saveSettings = async () => {
    setSaving(true);
    setToast("");
    try {
      const res = await fetch("/api/settings/plex", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      });
      if (res.ok) {
        setToast("Plex settings saved successfully!");
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

  // Re-add OAuth handler for the visible Plex button (token/clientId remain hidden)
  const startOAuthFlow = async () => {
    try {
      const response = await fetch("/api/plex/login");
      if (!response.ok) {
        setToast("Failed to initiate OAuth flow.");
        setToastSuccess(false);
        return;
      }
      const data = await response.json();

      // Store OAuth data for later exchange
      sessionStorage.setItem("plexOAuthPinID", data.pinId.toString());
      sessionStorage.setItem("plexOAuthCode", data.code);

      // Open Plex login in new window and keep a reference so we can close it
      // automatically once the token exchange completes.
      const width = 500;
      const height = 600;
      const left = window.screenX + (window.outerWidth - width) / 2;
      const top = window.screenY + (window.outerHeight - height) / 2;
      const popup = window.open(
        data.loginUrl,
        "PlexOAuth",
        `width=${width},height=${height},left=${left},top=${top}`,
      );

      // Poll for completion (check every 2 seconds for up to 5 minutes)
      let pollCount = 0;
      const pollInterval = setInterval(async () => {
        pollCount++;
        if (pollCount > 150) {
          clearInterval(pollInterval);
          setToast("OAuth authentication timeout. Please try again.");
          setToastSuccess(false);
          return;
        }

        // If the popup was closed by the user, stop polling and notify
        if (popup?.closed) {
          clearInterval(pollInterval);
          setToast("Plex login window was closed before authorization completed.");
          setToastSuccess(false);
          // Clean up stored pin/code
          sessionStorage.removeItem("plexOAuthPinID");
          sessionStorage.removeItem("plexOAuthCode");
          return;
        }

        try {
          const pinID = sessionStorage.getItem("plexOAuthPinID");
          const code = sessionStorage.getItem("plexOAuthCode");

          if (!pinID || !code) return;

          const exchangeRes = await fetch("/api/plex/exchange", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ code, pinId: Number.parseInt(pinID) }),
          });
          // 202 = still pending, 200 = success
          if (exchangeRes.status === 202) {
            // still pending, continue polling
            return;
          }
          if (exchangeRes.ok) {
            const exchangeData = await exchangeRes.json();
            clearInterval(pollInterval);

            // Update settings with new token
            setSettings(prev => ({ ...prev, token: exchangeData.token }));
            setToast("Plex authentication successful! Token acquired.");
            setToastSuccess(true);

            // Close the popup if it's still open
            try {
              if (popup && !popup.closed) popup.close();
            } catch (e) {
              console.debug("Failed to close popup window:", e);
            }

            // Clean up session storage
            sessionStorage.removeItem("plexOAuthPinID");
            sessionStorage.removeItem("plexOAuthCode");
          }
        } catch (error) {
          // Continue polling, errors are normal until user completes auth
          console.debug("OAuth polling error (expected during auth):", error);
        }
      }, 2000);
    } catch (error) {
      setToast("Error starting OAuth flow: " + error.message);
      setToastSuccess(false);
    }
  };

  // (OAuthButton top-level component is defined above)

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
            <SectionHeader>Plex Server Configuration</SectionHeader>
            <div
              style={{
                marginBottom: "1.5rem",
                display: "block",
                width: "100%",
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "0.5rem",
                  marginBottom: "1rem",
                  width: "100%",
                }}
              >
                <input
                  id="plexEnabled"
                  name="enabled"
                  type="checkbox"
                  checked={settings.enabled}
                  onChange={handleChange}
                  style={{ marginRight: 8 }}
                />
                <label
                  htmlFor="plexEnabled"
                  style={{
                    fontWeight: 600,
                    fontSize: "1.15em",
                    textAlign: "left",
                    display: "block",
                    margin: 0,
                  }}
                >
                  Enable Plex Integration
                </label>
              </div>

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
                  Protocol
                  <br />
                  <select
                    name="protocol"
                    value={settings.protocol}
                    onChange={handleChange}
                    disabled={!settings.enabled}
                    style={{
                      width: "60%",
                      minWidth: 120,
                      maxWidth: 300,
                      padding: "0.5rem",
                      borderRadius: 6,
                      border: "1px solid #bbb",
                      background: settings.enabled ? "var(--settings-input-bg, #f5f5f5)" : "#f0f0f0",
                      color: settings.enabled ? "var(--settings-input-text, #222)" : "#999",
                      cursor: settings.enabled ? "auto" : "not-allowed",
                    }}
                  >
                    <option value="http">HTTP</option>
                    <option value="https">HTTPS</option>
                  </select>
                </label>
              </div>

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
                  Plex Server IP/Hostname
                  <br />
                  <input
                    name="ip"
                    value={settings.ip}
                    onChange={handleChange}
                    placeholder="localhost or IP address"
                    disabled={!settings.enabled}
                    style={{
                      width: "60%",
                      minWidth: 220,
                      maxWidth: 600,
                      padding: "0.5rem",
                      borderRadius: 6,
                      border: "1px solid #bbb",
                      background: settings.enabled ? "var(--settings-input-bg, #f5f5f5)" : "#f0f0f0",
                      color: settings.enabled ? "var(--settings-input-text, #222)" : "#999",
                      cursor: settings.enabled ? "auto" : "not-allowed",
                    }}
                  />
                </label>
              </div>

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
                  Port
                  <br />
                  <input
                    name="port"
                    type="number"
                    value={settings.port}
                    onChange={handlePortChange}
                    placeholder="32400"
                    disabled={!settings.enabled}
                    style={{
                      width: "60%",
                      minWidth: 80,
                      maxWidth: 200,
                      padding: "0.5rem",
                      borderRadius: 6,
                      border: "1px solid #bbb",
                      background: settings.enabled ? "var(--settings-input-bg, #f5f5f5)" : "#f0f0f0",
                      color: settings.enabled ? "var(--settings-input-text, #222)" : "#999",
                      cursor: settings.enabled ? "auto" : "not-allowed",
                    }}
                  />
                </label>
              </div>

              <div style={{ width: "100%", marginBottom: "1.2rem" }}>
                {/* Keep OAuth button visible even though token/clientId fields are hidden */}
                <OAuthButton onClick={startOAuthFlow} disabled={false} />
              </div>

              {/* Token field intentionally hidden from the UI (preserved in settings state) */}

              {/* Client ID field intentionally hidden from the UI (preserved in settings state) */}
            </div>
          </>
        )}
      </div>
    </Container>
  );
}
