import { describe, it, expect } from "vitest";
import { getSearchSections } from "../utils/search";

const items = [
  { title: "Alpha", overview: "An action movie" },
  { title: "Beta", overview: "Romantic drama" },
  { title: "Gamma", overview: "A thrilling action-adventure" },
];

describe("getSearchSections", () => {
  it("returns all items as titleMatches when search is empty", () => {
    const res = getSearchSections(items, "");
    expect(res.titleMatches).toHaveLength(3);
    expect(res.overviewMatches).toHaveLength(0);
  });

  it("finds title matches case-insensitively", () => {
    const res = getSearchSections(items, "alpha");
    expect(res.titleMatches).toHaveLength(1);
    expect(res.titleMatches[0].title).toBe("Alpha");
    expect(res.overviewMatches).toHaveLength(0);
  });

  it("finds overview matches when title does not match", () => {
    const res = getSearchSections(items, "thrilling");
    expect(res.titleMatches).toHaveLength(0);
    expect(res.overviewMatches).toHaveLength(1);
    expect(res.overviewMatches[0].title).toBe("Gamma");
  });
});
