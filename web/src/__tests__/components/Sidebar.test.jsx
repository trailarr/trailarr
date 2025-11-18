import React from "react";
import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import Sidebar from "../../components/layout/Sidebar";
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
});
