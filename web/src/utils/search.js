export function getSearchSections(items, search) {
  const trimmed = (search || "").trim();
  if (!trimmed) return { titleMatches: items, overviewMatches: [] };
  const q = trimmed.toLowerCase();
  const titleMatches = (items || []).filter((item) =>
    item.title?.toLowerCase().includes(q),
  );
  const overviewMatches = (items || []).filter(
    (item) =>
      !titleMatches.includes(item) && item.overview?.toLowerCase().includes(q),
  );
  return { titleMatches, overviewMatches };
}

export function filterAndSortMedia(items) {
  return (items || [])
    .filter((item) => item?.title)
    .sort((a, b) => a.title.localeCompare(b.title));
}
