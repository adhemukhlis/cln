package scan

import "strings"

// Misc candidates are files and directories the OS or file archivers create
// on their own. They hold no user content and are regenerated when needed, so
// deleting them is safe. They are matched on every OS, since they travel with
// copied folders, archives and USB drives.
//
// Deliberately not matched, because deleting them changes behavior:
//   - desktop.ini (Windows): custom folder icons and localized names of
//     special folders such as Documents and Downloads.
//   - .localized (macOS): localized folder names in Finder.
//   - "Icon\r" (macOS): user-chosen custom folder icons.

// isMiscFile reports whether a regular file named name is OS metadata.
func isMiscFile(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", // macOS Finder view settings.
		"thumbs.db", "ehthumbs.db", "ehthumbs_vista.db", // Windows thumbnail caches.
		".directory": // KDE Dolphin view settings.
		return true
	}
	// macOS AppleDouble files ("._name"), left behind on non-Apple file
	// systems and in archives.
	return len(name) > 2 && strings.HasPrefix(name, "._")
}

// isMiscDir reports whether a directory named name is archiver metadata.
func isMiscDir(name string) bool {
	// Resource-fork folder added by macOS "Compress" to zip archives.
	return name == "__MACOSX"
}
