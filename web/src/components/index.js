// Barrel file to aggregate common exports so code can import from `components` root
export { default as Header } from "./layout/Header.jsx";
export { default as Sidebar } from "./layout/Sidebar.jsx";
export { default as Toast } from "./layout/Toast.jsx";
export { default as Container } from "./layout/Container.jsx";
export { default as SectionHeader } from "./layout/SectionHeader.jsx";
export { default as ErrorBoundary } from "./layout/ErrorBoundary.jsx";
export { default as RouteMap, appRoutes } from "./layout/RouteMap.jsx";

export { default as IconButton } from "./ui/IconButton.jsx";
export { default as HealthBadge } from "./ui/HealthBadge.jsx";
export { default as ExtraCard } from "./ui/ExtraCard.jsx";
export { default as DirectoryPicker } from "./ui/DirectoryPicker.jsx";
export { default as ActorRow } from "./ui/ActorRow.jsx";
export { default as ActionLane } from "./ui/ActionLane.jsx";
export { default as ExtrasList } from "./ui/ExtrasList.jsx";

export { default as MediaList } from "./media/MediaList.jsx";
export { default as MediaCard } from "./media/MediaCard.jsx";
export { default as MediaDetails } from "./media/MediaDetails.jsx";
export { default as MediaInfoLane } from "./media/MediaInfoLane.jsx";
export { default as LoadingMediaSkeleton } from "./media/LoadingMediaSkeleton.jsx";

export { default as BlacklistPage } from "./pages/BlacklistPage.jsx";
export { default as HistoryPage } from "./pages/HistoryPage.jsx";
export { default as LogsPage } from "./pages/LogsPage.jsx";
export { default as Tasks } from "./pages/Tasks.jsx";
export { default as Wanted } from "./pages/Wanted.jsx";
export { default as ProviderSettingsPage } from "./pages/ProviderSettingsPage.jsx";
export { default as StatusPage } from "./pages/StatusPage.jsx";

export { default as GeneralSettings } from "./settings/GeneralSettings.jsx";
export { default as PlexSettings } from "./settings/PlexSettings.jsx";
export { default as ExtrasSettings } from "./settings/ExtrasSettings.jsx";
export { default as YtdlpFlagsSettings } from "./settings/YtdlpFlagsSettings.jsx";

export { default as YoutubePlayer } from "./youtube/YoutubePlayer.jsx";
export { default as YoutubeModal } from "./youtube/YoutubeModal.jsx";

// Legacy compatibility re-exports
// Legacy compatibility re-export -> point to canonical UI path now that the root wrapper will be removed
export { default as ExtraCardOld } from "./ui/ExtraCard.jsx";
