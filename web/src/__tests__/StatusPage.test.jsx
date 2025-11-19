import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { vi } from "vitest";

// Mock fetch to avoid network calls in the component
global.fetch = vi.fn(() =>
  Promise.resolve({ ok: true, json: () => Promise.resolve({ health: [] }) }),
);

describe("StatusPage dark mode class", () => {
  afterEach(() => {
    vi.resetAllMocks();
  });

  it("applies status-dark class when isDark is true", async () => {
    // Mock matchMedia to simulate dark mode and import component afterwards
    global.matchMedia = () => ({
      matches: true,
      addEventListener: () => {},
      removeEventListener: () => {},
    });
    vi.resetModules();
    const { default: StatusPage } = await import("../components/pages/StatusPage");
    const { container } = render(<StatusPage />);
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    const root = container.querySelector(".status-root");
    expect(root.classList.contains("status-dark")).toBe(true);
  });

  it("does not apply status-dark class when isDark is false", async () => {
    global.matchMedia = () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    });
    vi.resetModules();
    const { default: StatusPage } = await import("../components/pages/StatusPage");
    const { container } = render(<StatusPage />);
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    const root = container.querySelector(".status-root");
    expect(root.classList.contains("status-dark")).toBe(false);
  });
});
