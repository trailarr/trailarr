import React from "react";
import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import Sidebar from "../../components/layout/Sidebar";
import SidebarDesktop from "../../components/layout/SidebarDesktop";
import { MemoryRouter } from "react-router-dom";

describe("Sidebar wanted submenu selection", () => {
  beforeEach(() => {
    // Mock fetch used by Sidebar to avoid console errors during tests
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ health: [] }),
      }),
    );
  });

  test("desktop shows system menu badge red when there are errors", async () => {
    // Return a health array containing an error
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ health: [{ level: "error", message: "err" }] }),
      }),
    );
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Sidebar mobile={false} open={true} onClose={() => {}} />
      </MemoryRouter>,
    );
    // Badge should show a count and be red
    const badge = await screen.findByLabelText(/1 health issues/i);
    const bg = window.getComputedStyle(badge).backgroundColor;
    expect(bg).toBe("rgb(239, 68, 68)");
  });

  test("desktop highlights Movies sub on /wanted/movies", () => {
    render(
      <MemoryRouter initialEntries={["/wanted/movies"]}>
        <Sidebar mobile={false} open={true} onClose={() => {}} />
      </MemoryRouter>,
    );
    const moviesLinks = screen.getAllByRole("link", { name: /Movies/i });
    const moviesLink = moviesLinks.find(
      (l) => l.getAttribute("href") === "/wanted/movies",
    );
    expect(moviesLink).toBeDefined();
    // The selected item should render as bold
    expect(
      moviesLink.style.fontWeight === "bold" ||
        moviesLink.style.fontWeight === "700",
    ).toBe(true);
  });

  test("mobile highlights Series sub on /wanted/series", () => {
    render(
      <MemoryRouter initialEntries={["/wanted/series"]}>
        <Sidebar mobile={true} open={true} onClose={() => {}} />
      </MemoryRouter>,
    );
    const seriesLinks = screen.getAllByRole("link", { name: /Series/i });
    const seriesLink = seriesLinks.find(
      (l) => l.getAttribute("href") === "/wanted/series",
    );
    expect(seriesLink).toBeDefined();
    expect(
      seriesLink.style.fontWeight === "bold" ||
        seriesLink.style.fontWeight === "700",
    ).toBe(true);
  });

  test("menu and submenu counters have same styling properties", () => {
    // Main (collapsed system menu) counter
    const mainProps = {
      selectedSection: "",
      selectedSettingsSub: "",
      selectedWantedSub: "",
      selectedSystemSub: "",
      isOpen: () => false,
      handleToggle: () => {},
      healthCount: 3,
      hasHealthError: false,
    };
    const { container: c1 } = render(
      <MemoryRouter initialEntries={["/"]}>
        <SidebarDesktop {...mainProps} />
      </MemoryRouter>,
    );
    const mainBadge = c1.querySelector('[aria-label="3 health issues"]');
    expect(mainBadge).not.toBeNull();
    const mainStyle = window.getComputedStyle(mainBadge);

    // Submenu (open system menu) counter
    const subProps = { ...mainProps, isOpen: (n) => n === "System" };
    const { container: c2 } = render(
      <MemoryRouter initialEntries={["/"]}>
        <SidebarDesktop {...subProps} />
      </MemoryRouter>,
    );
    const subBadge = c2.querySelector('[aria-label="3 health issues"]');
    expect(subBadge).not.toBeNull();
    const subStyle = window.getComputedStyle(subBadge);

    // Compare key style props (color/size)
    expect(mainStyle.backgroundColor).toBe(subStyle.backgroundColor);
    expect(mainStyle.width).toBe(subStyle.width);
    expect(mainStyle.height).toBe(subStyle.height);
    expect(mainStyle.fontSize).toBe(subStyle.fontSize);
  });
});
