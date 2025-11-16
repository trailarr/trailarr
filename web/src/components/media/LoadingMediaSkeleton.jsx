import React from "react";
import { isDarkNow } from "../../utils/isDark";

export default function LoadingMediaSkeleton() {
  const dark = isDarkNow();
  return (
    <div style={{ padding: 88, minHeight: "60vh", boxSizing: "border-box" }}>
      <div style={{ display: "flex", gap: 24, alignItems: "flex-start" }}>
        <div style={{ flex: 1 }}>
          aaa
          <div
            style={{
              width: "60%",
              height: 28,
              borderRadius: 6,
              background: dark ? "#202124" : "#e8e8e8",
              marginBottom: 12,
            }}
          />
          <div
            style={{
              width: "40%",
              height: 18,
              borderRadius: 6,
              background: dark ? "#202124" : "#e8e8e8",
              marginBottom: 18,
            }}
          />
          <div style={{ display: "flex", gap: 12, marginBottom: 12 }}>
            <div style={{ width: 120, height: 36, borderRadius: 8, background: dark ? "#202124" : "#e8e8e8" }} />
            <div style={{ width: 120, height: 36, borderRadius: 8, background: dark ? "#202124" : "#e8e8e8" }} />
          </div>
        </div>
      </div>
    </div>
  );
}
