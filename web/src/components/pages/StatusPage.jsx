import React, { useEffect, useState, useCallback } from "react";
import Container from "../layout/Container.jsx";
import "./StatusPage.css";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCog,
  faVial,
  faBookReader,
  faCircleExclamation,
  faArrowsRotate,
} from "@fortawesome/free-solid-svg-icons";
import Toast from "../layout/Toast.jsx";
import { isDark } from "../../utils/isDark";
import PropTypes from "prop-types";

const HealthRow = React.memo(function HealthRow({
  h,
  idx,
  moreInfo,
  executeHealthcheck,
  executeUpdate,
}) {
  const src = (h.source || "").toLowerCase();
  let settingsHref = "/settings";
  if (src.includes("radarr")) settingsHref = "/settings/radarr";
  else if (src.includes("sonarr")) settingsHref = "/settings/sonarr";
  const handleTrigger = useCallback(
    (e) => {
      e.preventDefault();
      executeHealthcheck(h, idx);
    },
    [executeHealthcheck, h, idx],
  );
  const handleUpdate = useCallback(
    (e) => {
      e.preventDefault();
      executeUpdate(h, idx);
    },
    [executeUpdate, h, idx],
  );
  const isUpdateSource = src.includes("yt-dlp") || src.includes("ffmpeg");
  const level = (h.level || h.Level || "error").toString().toLowerCase();
  return (
    <tr
      className="status-row"
      key={h.id || `${h.source || ""}-${h.message || ""}`}
    >
      <td className="status-message">
        <span className="status-icon">
          <FontAwesomeIcon
            icon={faCircleExclamation}
            className={`status-fa-icon ${level}`}
          />
        </span>
        <div className="status-message-text">{h.message}</div>
      </td>
      <td className="status-actions">
        <a
          href={moreInfo.home || "#"}
          title="Wiki"
          className="action-icon"
          aria-label="wiki"
          target="_blank"
          rel="noopener noreferrer"
        >
          <FontAwesomeIcon icon={faBookReader} />
        </a>
        {isUpdateSource ? (
          <a
            href="#"
            onClick={(e) => {
              e.preventDefault();
              handleUpdate(e);
            }}
            title="Update"
            className="action-icon"
            aria-label="update-ytdlp"
          >
            <FontAwesomeIcon icon={faArrowsRotate} />
          </a>
        ) : (
          <a
            href={settingsHref}
            title="Settings"
            className="action-icon"
            aria-label="settings"
          >
            <FontAwesomeIcon icon={faCog} />
          </a>
        )}
        {!isUpdateSource && (
          <a
            href="#"
            onClick={handleTrigger}
            role="button"
            title="Trigger Healthcheck"
            className="action-icon test-black"
            aria-label="trigger-health"
          >
            <FontAwesomeIcon icon={faVial} />
          </a>
        )}
      </td>
    </tr>
  );
});

export default function StatusPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const retryingRef = React.useRef({});
  const [toastMessage, setToastMessage] = useState("");
  const [toastSuccess, setToastSuccess] = useState(false);
  const fetchStatus = async (suppressLoading = false) => {
    if (suppressLoading === false) {
      setLoading(true);
      setError("");
    }
    try {
      const res = await fetch("/api/system/status");
      if (!res.ok) throw new Error("Failed to fetch status");
      const json = await res.json();
      if (suppressLoading) {
        setData((prev) => (prev ? { ...prev, ...json } : { ...json }));
      } else {
        setData(json);
      }
    } catch (e) {
      if (suppressLoading) {
        setToastMessage(
          "Failed to refresh status: " + (e.message || String(e)),
        );
        setToastSuccess(false);
      } else {
        setError(e.message);
      }
    } finally {
      if (!suppressLoading) setLoading(false);
    }
  };
  useEffect(() => {
    fetchStatus();
  }, []);
  const executeHealthcheck = async (h, idx) => {
    try {
      retryingRef.current = {
        ...retryingRef.current,
        [idx]: { status: "pending", msg: "" },
      };
      const src = (h.source || "").toLowerCase();
      let provider = "";
      if (src.includes("radarr")) provider = "radarr";
      else if (src.includes("sonarr")) provider = "sonarr";
      const endpoint = provider
        ? `/api/health/${provider}/execute`
        : "/api/health/execute";
      const res = await fetch(endpoint, { method: "POST" });
      let json = null;
      try {
        json = await res.json();
      } catch {
        const txt = await res.text();
        throw new Error(txt || "Failed to trigger healthcheck");
      }
      if (!json?.success) {
        const errMsg = json?.error || "Healthcheck reported failure";
        throw new Error(errMsg);
      }
      retryingRef.current = {
        ...retryingRef.current,
        [idx]: { status: "success", msg: "OK" },
      };
      setToastMessage("Healthcheck successful");
      setToastSuccess(true);
      setData((prev) => {
        if (!prev) return prev;
        const cur = { ...prev };
        const arr = Array.isArray(cur.health) ? cur.health : [];
        const filtered = arr.filter((item) => {
          if (h.id && item.id) return item.id !== h.id;
          return !(item.source === h.source && item.message === h.message);
        });
        cur.health = filtered;
        return cur;
      });
      await fetchStatus(true);
      setTimeout(() => {
        retryingRef.current = { ...retryingRef.current, [idx]: undefined };
      }, 1500);
    } catch (err) {
      const msg = err?.message ? err.message : String(err);
      retryingRef.current = {
        ...retryingRef.current,
        [idx]: { status: "error", msg },
      };
      setToastMessage("Healthcheck failed: " + msg);
      setToastSuccess(false);
      setTimeout(() => {
        retryingRef.current = { ...retryingRef.current, [idx]: undefined };
      }, 3000);
    }
  };

  const executeUpdate = async (h) => {
    try {
      // Choose endpoint and tool name based on source
      const src = (h.source || "").toLowerCase();
      const toolName = src.includes("ffmpeg") ? "ffmpeg" : "yt-dlp";
      setToastMessage(`Updating ${toolName}...`);
      setToastSuccess(true);
      let endpoint = "/api/system/update/ytdlp";
      if (src.includes("ffmpeg")) endpoint = "/api/system/update/ffmpeg";
      const res = await fetch(endpoint, { method: "POST" });
      if (!res.ok) {
        let errTxt = `Failed to update ${toolName}`;
        try {
          const j = await res.json();
          if (j && j.error) errTxt = j.error;
        } catch (e) {
          void e; /* ignore parse error, use default error text */
        }
        throw new Error(errTxt);
      }
      setToastMessage(`${toolName} update triggered`);
      setToastSuccess(true);
      await fetchStatus(true);
    } catch (err) {
      const msg = err?.message ? err.message : String(err);
      const src = (h.source || "").toLowerCase();
      const toolName = src.includes("ffmpeg") ? "ffmpeg" : "yt-dlp";
      setToastMessage(`${toolName} update failed: ${msg}`);
      setToastSuccess(false);
    }
  };

  if (loading) {
    return (
      <Container style={{ padding: "1.2rem" }}>
        <div>Loading status...</div>
      </Container>
    );
  }
  if (error) {
    return (
      <Container style={{ padding: "1.2rem" }}>
        <div>Error loading status: {error}</div>
      </Container>
    );
  }
  const { health = [], disks = [], about = {}, moreInfo = {} } = data || {};
  const sortedDisks = Array.isArray(disks)
    ? [...disks].sort((a, b) =>
        String(a.location || "")
          .toLowerCase()
          .localeCompare(String(b.location || "").toLowerCase()),
      )
    : [];
  return (
    <Container
      className={`status-root ${isDark ? "status-dark" : ""}`}
      style={{ padding: "1.2rem" }}
    >
      <div className="status-page container">
        <h2 className="status-title">Health</h2>
        <div className="status-card health">
          {!health || health.length === 0 ? (
            <div className="status-row" style={{ padding: "1rem" }}>
              No health issues detected.
            </div>
          ) : (
            <>
              <table className="health-table">
                <thead>
                  <tr>
                    <th>Message</th>
                    <th style={{ width: "160px" }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {health.map((h, idx) => (
                    <HealthRow
                      key={h.id || `${h.source || ""}-${h.message || ""}`}
                      h={h}
                      idx={idx}
                      moreInfo={moreInfo}
                      executeHealthcheck={executeHealthcheck}
                      executeUpdate={executeUpdate}
                    />
                  ))}
                </tbody>
              </table>
              <div className="status-hint">
                You can find more information about the cause of these health
                check messages by clicking the wiki link (book icon) at the end
                of the row, or by checking your <a href="/system/logs">logs</a>.
                If you have difficulty interpreting these messages then you can
                reach out to our support, at the links below.
              </div>
            </>
          )}
        </div>
        <h2 className="status-title">Disk Space</h2>
        <div className="status-card disk">
          <table className="disk-table">
            <thead>
              <tr>
                <th>Location</th>
                <th>Free Space</th>
                <th>Total Space</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {sortedDisks.map((d, i) => (
                <tr key={d.location || `disk-${i}`}>
                  <td>{d.location}</td>
                  <td>{d.freeHuman || d.freeStr || d.free || "N/A"}</td>
                  <td>{d.totalHuman || d.totalStr || d.total || "N/A"}</td>
                  <td>
                    <div
                      className={`bar ${d.usedPercent >= 95 ? "critical" : ""}`}
                    >
                      <div
                        className="bar-inner"
                        style={{ width: `${d.usedPercent || d.usedPct || 0}%` }}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <h2 className="status-title">About</h2>
        <div className="status-card about">
          <dl className="about-list">
            {Object.entries({
              Version: about.version,
              "yt-dlp": about.ytdlpVersion,
              ffmpeg: about.ffmpegVersion,
              "AppData Directory": about.appDataDirectory,
              "Startup Directory": about.startupDirectory,
              Mode: about.mode,
              Uptime: about.uptime,
            }).map(([k, v]) => (
              <div className="about-row" key={k}>
                <dt>{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        </div>
        <h2 className="status-title">More Info</h2>
        <div className="status-card moreinfo">
          <dl className="about-list">
            {Object.keys(moreInfo || {}).length === 0 ? (
              <div className="about-row">
                <dt />
                <dd>No additional information provided.</dd>
              </div>
            ) : (
              Object.entries(moreInfo).map(([k, v]) => {
                const keyLabel = String(k)
                  .replaceAll(/[_-]/g, " ")
                  .replaceAll(/([a-z])([A-Z])/g, "$1 $2")
                  .replaceAll(/\b\w/g, (c) => c.toUpperCase());
                let valueNode = null;
                if (v === null || v === undefined) valueNode = "-";
                else if (typeof v === "string" && /^https?:\/\//i.test(v))
                  valueNode = (
                    <a href={v} target="_blank" rel="noopener noreferrer">
                      {v}
                    </a>
                  );
                else if (typeof v === "object") {
                  try {
                    valueNode = JSON.stringify(v);
                  } catch {
                    valueNode = String(v);
                  }
                } else valueNode = String(v);
                return (
                  <div className="about-row" key={k}>
                    <dt>{keyLabel}</dt>
                    <dd>{valueNode}</dd>
                  </div>
                );
              })
            )}
          </dl>
        </div>
      </div>
      <Toast
        message={toastMessage}
        onClose={() => setToastMessage("")}
        success={toastSuccess}
      />
    </Container>
  );
}

HealthRow.propTypes = {
  h: PropTypes.object.isRequired,
  idx: PropTypes.number.isRequired,
  moreInfo: PropTypes.object.isRequired,
  executeHealthcheck: PropTypes.func.isRequired,
  executeUpdate: PropTypes.func.isRequired,
};
