declare global {
  interface Window {
    clashcubeAndroid?: {
      setSystemBars(color: string, dark: boolean): void;
      uptimeSeconds(): number;
      readClipboard(): string;
      isSystemDark(): boolean;
      openReleases(): void;
    };
  }
}

// Wails' desktop BrowserManager uses xdg-open on Android; the Activity owns this action.
export function openReleases() { window.clashcubeAndroid?.openReleases(); }
