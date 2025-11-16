import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { isDark } from "../../utils/isDark";

export default function LogsPage() {
  const formatLogDate = (dateStr) => {
    const d = new Date(dateStr);
    const now = new Date();
    const isToday = d.toDateString() === now.toDateString();
    const yesterday = new Date(now);
    yesterday.setDate(now.getDate() - 1);
    const isYesterday = d.toDateString() === yesterday.toDateString();
    if (isToday) {
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
    } else if (isYesterday) {
      return "Yesterday";
    }
    return d.toLocaleDateString([], { day: "2-digit", month: "short", year: "numeric" });
  };
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [logDir, setLogDir] = useState("");
  useEffect(() => {
    fetch("/api/logs/list").then((res) => res.json()).then((data) => { const sortedLogs = (data.logs || []).slice().sort((a, b) => { const aDate = new Date(a.lastWrite); const bDate = new Date(b.lastWrite); return bDate - aDate; }); setLogs(sortedLogs); setLoading(false); setLogDir(data.logDir || ""); });
  }, []);
  const bgColor = isDark ? "#222" : "#e3f2fd";
  const textColor = isDark ? "#eee" : "#333";
  const tableBg = isDark ? "#222" : "#f9fafb";
  const tableHeaderBg = isDark ? "#333" : "#f3f4f6";
  const borderColor = isDark ? "#444" : "#e5e7eb";
  const disclaimerBg = isDark ? "#2563eb" : bgColor;
  return (
    <div style={{ padding: "2em" }}>
      <div style={{ background: disclaimerBg, padding: "1em", borderRadius: 8, marginBottom: "1em", color: textColor, textAlign: "left" }}>
        Log files are located in: <span style={{ fontWeight: 600 }}>{logDir || "[log directory not available]"}</span>
        <br />
        The log level defaults to 'Debug' and can be changed in <Link to="/settings/general" style={{ color: isDark ? "#fff" : "#2563eb", textDecoration: "underline", fontWeight: 600 }}>General Settings</Link>
      </div>
      <table style={{ width: "100%", borderCollapse: "collapse", background: tableBg, borderRadius: 8 }}>
        <thead>
          <tr style={{ background: tableHeaderBg, color: textColor }}>
            <th style={{ textAlign: "left", padding: "0.75em" }}>#</th>
            <th style={{ textAlign: "left", padding: "0.75em" }}>Filename</th>
            <th style={{ textAlign: "left", padding: "0.75em" }}>Last Write Time</th>
            <th style={{ textAlign: "left", padding: "0.75em" }}>Download</th>
          </tr>
        </thead>
        <tbody>
          {loading ? (<tr><td colSpan={4}>Loading...</td></tr>) : (logs.map((log) => (<tr key={log.filename} style={{ borderBottom: `1px solid ${borderColor}` }}><td style={{ padding: "0.75em", textAlign: "left", color: textColor }}>{logs.indexOf(log) + 1}</td><td style={{ padding: "0.75em", textAlign: "left", color: textColor }}>{log.filename}</td><td style={{ padding: "0.75em", textAlign: "left", color: textColor }}>{formatLogDate(log.lastWrite)}</td><td style={{ padding: "0.75em", textAlign: "left" }}><a href={`/logs/${encodeURIComponent(log.filename)}`} target="_blank" rel="noopener noreferrer" style={{ color: isDark ? "#90cdf4" : "#2563eb", textDecoration: "none" }}>Download</a></td></tr>))) }
        </tbody>
      </table>
    </div>
  );
}
