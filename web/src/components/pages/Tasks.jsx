import React, { useEffect, useState, useRef } from "react";
import { useLocation } from "react-router-dom";
import { FaArrowsRotate, FaClock } from "react-icons/fa6";
import "./Tasks.css";
import { isDarkNow, addDarkModeListener } from "../../utils/isDark";

function formatTimeDiff({ from, to, suffix = "", roundType = "ceil" }) {
  if (!from || !to) return "-";
  let diff = Math.max(0, to - from);
  return durationToText(diff, suffix, roundType);
}
function formatInterval(interval) {
  if (interval == null || interval === "") return "-";
  if (typeof interval === "number") {
    return durationToText(interval * 60 * 1000);
  }
  if (typeof interval !== "string") interval = String(interval);
  const regex = /(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?/;
  const match = interval.match(regex);
  if (!match) return interval;
  const days = Number.parseInt(match[1] || "0", 10);
  const hours = Number.parseInt(match[2] || "0", 10);
  const minutes = Number.parseInt(match[3] || "0", 10);
  if (days > 0 || hours > 0 || minutes > 0) {
    return durationToText((days * 86400 + hours * 3600 + minutes * 60) * 1000);
  }
  const min = Number.parseInt(interval, 10);
  if (!Number.isNaN(min)) {
    return durationToText(min * 60 * 1000);
  }
  return interval;
}
function formatDuration(duration) {
  if (!duration || duration === "-") return "-";
  if (typeof duration === "number") {
    if (duration < 1) return `${(duration * 1000).toFixed(2)} ms`;
    return durationToText(duration * 1000);
  }
  if (typeof duration === "string" && duration.endsWith("ms")) {
    const ms = Number.parseFloat(duration.replace("ms", ""));
    if (ms < 1000) return `${ms.toFixed(2)} ms`;
    return durationToText(ms);
  }
  const match = duration.match(/(?:(\d+)h)?(?:(\d+)m)?([\d.]+)s/);
  if (!match) return duration;
  const hours = Number.parseInt(match[1] || "0", 10);
  const minutes = Number.parseInt(match[2] || "0", 10);
  const secondsFloat = Number.parseFloat(match[3] || "0");
  if (secondsFloat < 1 && hours === 0 && minutes === 0)
    return `${(secondsFloat * 1000).toFixed(2)} ms`;
  return durationToText(
    (hours * 3600 + minutes * 60 + Math.floor(secondsFloat)) * 1000,
  );
}
const iconNoOutline = { outline: "none", boxShadow: "none" };
const getStyles = (isDark) => ({
  table: {
    width: "100%",
    marginBottom: "2em",
    borderCollapse: "collapse",
    background: isDark ? "#23272f" : "#f6f7f9",
    color: isDark ? "#eee" : "#222",
    fontSize: "15px",
  },
  th: {
    textAlign: "left",
    padding: "0.75em 0.5em",
    fontWeight: 500,
    background: isDark ? "#23272f" : "#f6f7f9",
    borderBottom: isDark ? "1px solid #444" : "1px solid #e5e7eb",
    color: isDark ? "#eee" : "#222",
  },
  td: {
    padding: "0.75em 0.5em",
    borderBottom: isDark ? "1px solid #444" : "1px solid #e5e7eb",
    background: isDark ? "#181a20" : "#fff",
    textAlign: "left",
    color: isDark ? "#eee" : "#222",
  },
  header: {
    fontSize: "1.4em",
    fontWeight: 600,
    margin: "0 0 1em 0",
    color: isDark ? "#eee" : "#222",
  },
  container: {
    padding: "2em",
    background: isDark ? "#181a20" : "#f6f7f9",
    minHeight: "100vh",
    color: isDark ? "#eee" : "#222",
  },
});
function durationToText(ms, suffix = "", roundType = "round") {
  if (typeof ms !== "number" || Number.isNaN(ms) || ms < 0)
    return `0 seconds${suffix}`;
  const units = [
    { name: "day", value: 86400 },
    { name: "hour", value: 3600 },
    { name: "minute", value: 60 },
    { name: "second", value: 1 },
  ];
  let totalSeconds;
  switch (roundType) {
    case "cut":
      totalSeconds = Math.floor(ms / 1000);
      break;
    case "round":
      totalSeconds = Math.round(ms / 1000);
      break;
    default:
      totalSeconds = Math.ceil(ms / 1000);
  }
  for (const unit of units) {
    if (totalSeconds >= unit.value) {
      let amount;
      switch (roundType) {
        case "cut":
          amount = Math.floor(totalSeconds / unit.value);
          break;
        case "round":
          amount = Math.round(totalSeconds / unit.value);
          break;
        default:
          amount = Math.ceil(totalSeconds / unit.value);
      }
      return `${amount} ${unit.name}${amount > 1 ? "s" : ""}${suffix}`;
    }
  }
  return `0 seconds${suffix}`;
}
function getQueueKey(item) {
  return `${item.taskId || ""}-${item.queued || ""}-${item.started || ""}-${item.ended || ""}`;
}
export default function Tasks() {
  const location = useLocation();
  const [loading, setLoading] = useState(true);
  const [status, setStatus] = useState(null);
  const [queues, setQueues] = useState([]);
  const [isDark, setIsDark] = useState(false);
  const activeRef = useRef(false);
  async function fetchStatus() {
    if (!activeRef.current) return;
    if (
      typeof document !== "undefined" &&
      document.visibilityState !== "visible"
    )
      return;
    setLoading(true);
    try {
      const res = await fetch("/api/tasks/status");
      const data = await res.json();
      setStatus(data);
    } catch {
      setStatus(null);
    }
    setLoading(false);
  }
  async function fetchQueue() {
    if (!activeRef.current) return;
    if (
      typeof document !== "undefined" &&
      document.visibilityState !== "visible"
    )
      return;
    try {
      const res = await fetch("/api/tasks/queue");
      const data = await res.json();
      if (data && Array.isArray(data.queues)) setQueues(data.queues);
      else setQueues([]);
    } catch {
      setQueues([]);
    }
  }
  useEffect(() => {
    if (!location.pathname?.startsWith("/system/tasks")) {
      activeRef.current = false;
      return;
    }
    activeRef.current = true;
    setIsDark(isDarkNow());
    const remove = addDarkModeListener((v) => setIsDark(v));
    return remove;
  }, [location.pathname]);
  useEffect(() => {
    if (!location.pathname?.startsWith("/system/tasks")) return;
    let ws;
    let pollingInterval;
    let queueInterval;
    let wsConnected = false;
    let pollingActive = false;
    function startPolling() {
      if (pollingActive) return;
      pollingActive = true;
      fetchStatus();
      pollingInterval = setInterval(fetchStatus, 500);
      fetchQueue();
      queueInterval = setInterval(fetchQueue, 1000);
      try {
        if (globalThis.window !== undefined) {
          try {
            globalThis.__trailarr_tasks_polling?.stop?.();
          } catch (e) {
            void e;
          }
          globalThis.__trailarr_tasks_polling = {
            stop: () => {
              try {
                if (pollingInterval) clearInterval(pollingInterval);
                if (queueInterval) clearInterval(queueInterval);
                if (ws) ws.close();
              } catch (e) {
                void e;
              }
            },
          };
        }
      } catch (e) {
        void e;
      }
    }
    function stopPolling() {
      pollingActive = false;
      if (pollingInterval) clearInterval(pollingInterval);
      if (queueInterval) clearInterval(queueInterval);
    }
    function handleVisibility() {
      if (document.visibilityState === "visible") {
        if (!wsConnected) startPolling();
      } else {
        stopPolling();
      }
    }
    try {
      ws = new globalThis.WebSocket(
        (globalThis.location.protocol === "https:" ? "wss://" : "ws://") +
          globalThis.location.host +
          "/ws/tasks",
      );
      ws.onopen = () => {
        wsConnected = true;
        stopPolling();
        fetchQueue();
        queueInterval = setInterval(fetchQueue, 1000);
      };
      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          setStatus(data);
          setLoading(false);
        } catch (e) {
          void e; /* ignore malformed ws payloads */
        }
      };
      ws.onerror = () => {
        wsConnected = false;
        if (document.visibilityState === "visible") startPolling();
      };
      ws.onclose = () => {
        wsConnected = false;
        if (document.visibilityState === "visible") startPolling();
      };
    } catch {
      if (document.visibilityState === "visible") startPolling();
    }
    if (!wsConnected && document.visibilityState === "visible") startPolling();
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      activeRef.current = false;
      stopPolling();
      if (ws) ws.close();
      try {
        if (
          globalThis.window !== undefined &&
          globalThis.__trailarr_tasks_polling?.stop
        ) {
          try {
            globalThis.__trailarr_tasks_polling.stop();
          } catch (e) {
            void e;
          }
          try {
            delete globalThis.__trailarr_tasks_polling;
          } catch (e) {
            void e;
          }
        }
      } catch (e) {
        void e;
      }
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [location.pathname]);
  async function forceExecute(taskId) {
    try {
      await fetch(`/api/tasks/force`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ taskId }),
      });
      fetchStatus();
    } catch (e) {
      void e; /* ignore network errors */
    }
  }
  const styles = getStyles(isDark);
  function hasValidTime(val) {
    if (val === null || val === undefined) return false;
    if (typeof val === "number") return val > 0;
    if (typeof val !== "string") return false;
    if (val.trim() === "") return false;
    if (val.startsWith("0001-01-01")) return false;
    const d = new Date(val);
    if (Number.isNaN(d.getTime())) return false;
    if (d.getUTCFullYear && d.getUTCFullYear() === 1) return false;
    return true;
  }
  const [showLoading, setShowLoading] = useState(false);
  useEffect(() => {
    let timer;
    if (loading) {
      timer = setTimeout(() => setShowLoading(true), 500);
    } else {
      setShowLoading(false);
    }
    return () => timer && clearTimeout(timer);
  }, [loading]);
  if (showLoading) return <div style={styles.container}>Loading...</div>;
  if (!status)
    return <div style={styles.container}>Error loading task status.</div>;
  const schedules = status.schedules || [];
  function renderScheduleStatus(scheduled) {
    if (scheduled.interval === 0)
      return (
        <span style={{ color: isDark ? "#888" : "#bbb", fontStyle: "italic" }}>
          Disabled
        </span>
      );
    const status = scheduled.status;
    if (!status) return <span>-</span>;
    if (status === "running")
      return (
        <span style={{ color: isDark ? "#66aaff" : "#007bff" }}>Running</span>
      );
    if (status === "success")
      return (
        <span style={{ color: isDark ? "#4fdc7b" : "#28a745" }}>Success</span>
      );
    if (status === "failed")
      return (
        <span style={{ color: isDark ? "#ff6b6b" : "#dc3545" }}>Failed</span>
      );
    return <span>{status}</span>;
  }
  function renderScheduleInterval(scheduled) {
    if (scheduled.interval === 0)
      return (
        <span style={{ color: isDark ? "#888" : "#bbb", fontStyle: "italic" }}>
          Disabled
        </span>
      );
    return formatInterval(scheduled.interval);
  }
  function renderScheduleNextExecution(scheduled) {
    if (scheduled.interval === 0)
      return (
        <span style={{ color: isDark ? "#888" : "#bbb", fontStyle: "italic" }}>
          Disabled
        </span>
      );
    return scheduled.nextExecution
      ? formatTimeDiff({
          from: new Date(),
          to: new Date(scheduled.nextExecution),
        })
      : "-";
  }
  function getForceIconStyle(scheduled) {
    let color;
    if (scheduled.status === "running") color = isDark ? "#66aaff" : "#007bff";
    else color = isDark ? "#aaa" : "#888";
    return {
      cursor: scheduled.status === "running" ? "not-allowed" : "pointer",
      opacity: scheduled.status === "running" ? 0.5 : 1,
      color,
      ...iconNoOutline,
    };
  }
  return (
    <div style={styles.container}>
      <div style={styles.header}>Scheduled</div>
      <table style={styles.table}>
        <thead>
          <tr>
            <th style={styles.th}>Name</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Status</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Interval</th>
            <th style={{ ...styles.th, textAlign: "center" }}>
              Last Execution
            </th>
            <th style={{ ...styles.th, textAlign: "center" }}>Last Duration</th>
            <th style={{ ...styles.th, textAlign: "center" }}>
              Next Execution
            </th>
            <th style={{ ...styles.th, textAlign: "center" }}></th>
          </tr>
        </thead>
        <tbody>
          {schedules.length === 0 ? (
            <tr>
              <td colSpan={7} style={styles.td}>
                No scheduled tasks
              </td>
            </tr>
          ) : (
            schedules.map((scheduled) => (
              <tr key={scheduled.taskId || scheduled.name}>
                <td style={styles.td}>{scheduled.name}</td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  {renderScheduleStatus(scheduled)}
                </td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  {renderScheduleInterval(scheduled)}
                </td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  {scheduled.lastExecution
                    ? formatTimeDiff({
                        from: new Date(scheduled.lastExecution),
                        to: new Date(),
                        suffix: " ago",
                        roundType: "cut",
                      })
                    : "-"}
                </td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  {scheduled.lastDuration
                    ? formatDuration(scheduled.lastDuration)
                    : "-"}
                </td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  {renderScheduleNextExecution(scheduled)}
                </td>
                <td style={{ ...styles.td, textAlign: "center" }}>
                  <span
                    style={{
                      display: "inline-block",
                      marginLeft: "0.5em",
                      verticalAlign: "middle",
                    }}
                  >
                    <FaArrowsRotate
                      onClick={
                        scheduled.status === "running"
                          ? undefined
                          : () => forceExecute(scheduled.taskId)
                      }
                      className={
                        scheduled.status === "running" ? "spin-icon" : ""
                      }
                      style={getForceIconStyle(scheduled)}
                      size={20}
                      title={
                        scheduled.status === "running"
                          ? "Task is running"
                          : "Force Execute"
                      }
                      tabIndex={scheduled.status === "running" ? -1 : 0}
                      aria-disabled={scheduled.status === "running"}
                    />
                  </span>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
      <div style={styles.header}>Queue</div>
      <table style={styles.table}>
        <thead>
          <tr>
            <th style={{ ...styles.th, textAlign: "center" }}></th>
            <th style={styles.th}>Task Name</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Queued</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Started</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Ended</th>
            <th style={{ ...styles.th, textAlign: "center" }}>Duration</th>
          </tr>
        </thead>
        <tbody>
          {Array.isArray(queues) && queues.length > 0 ? (
            (() => {
              const arr = queues;
              return arr.map((item) => {
                let taskName = item.taskId;
                if (schedules && item.taskId) {
                  const sch = schedules.find((s) => s.taskId === item.taskId);
                  if (sch?.name) taskName = sch.name;
                }
                function renderQueueStatus() {
                  if (!item.status) return <span title="Unknown">-</span>;
                  if (item.status === "success")
                    return (
                      <span
                        title="Success"
                        style={{ color: isDark ? "#4fdc7b" : "#28a745" }}
                      >
                        &#x2714;
                      </span>
                    );
                  if (item.status === "running")
                    return (
                      <span
                        title="Running"
                        style={{ color: isDark ? "#66aaff" : "#007bff" }}
                      >
                        &#x25D4;
                      </span>
                    );
                  if (item.status === "failed")
                    return (
                      <span
                        title="Failed"
                        style={{ color: isDark ? "#ff6b6b" : "#dc3545" }}
                      >
                        &#x2716;
                      </span>
                    );
                  if (item.status === "queued")
                    return (
                      <FaClock
                        title="Queued"
                        style={{
                          color: isDark ? "#ffb300" : "#e6b800",
                          verticalAlign: "middle",
                        }}
                      />
                    );
                  return <span title={item.status}>{item.status}</span>;
                }
                function renderDuration() {
                  if (
                    item.duration === null ||
                    item.duration === undefined ||
                    item.duration === ""
                  )
                    return "—";
                  let dur = item.duration;
                  if (typeof dur === "string") {
                    const n = Number(dur);
                    if (!Number.isNaN(n)) dur = n;
                  }
                  if (typeof dur === "number" && !Number.isNaN(dur)) {
                    let seconds = dur;
                    if (Math.abs(dur) >= 1e6) {
                      seconds = dur / 1e9;
                    }
                    return formatDuration(seconds);
                  }
                  return formatDuration(item.duration);
                }
                return (
                  <tr key={getQueueKey(item)}>
                    <td style={{ ...styles.td, textAlign: "center" }}>
                      {renderQueueStatus()}
                    </td>
                    <td style={styles.td}>{taskName || "-"}</td>
                    <td style={{ ...styles.td, textAlign: "center" }}>
                      {hasValidTime(item.queued)
                        ? formatTimeDiff({
                            from: new Date(item.queued),
                            to: new Date(),
                            suffix: " ago",
                            roundType: "cut",
                          })
                        : "—"}
                    </td>
                    <td style={{ ...styles.td, textAlign: "center" }}>
                      {hasValidTime(item.started)
                        ? formatTimeDiff({
                            from: new Date(item.started),
                            to: new Date(),
                            suffix: " ago",
                            roundType: "cut",
                          })
                        : "—"}
                    </td>
                    <td style={{ ...styles.td, textAlign: "center" }}>
                      {hasValidTime(item.ended)
                        ? formatTimeDiff({
                            from: new Date(item.ended),
                            to: new Date(),
                            suffix: " ago",
                            roundType: "cut",
                          })
                        : "—"}
                    </td>
                    <td
                      style={{
                        ...styles.td,
                        textAlign: "right",
                        paddingRight: "3em",
                      }}
                    >
                      {renderDuration()}
                    </td>
                  </tr>
                );
              });
            })()
          ) : (
            <tr>
              <td colSpan={6} style={{ ...styles.td, textAlign: "center" }}>
                No queue items
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
