package scanner

// This file exposes the pieces of the scanner the app layer calls. The scan
// methods themselves stay unexported, as they were in the Swift original: the
// app talks to the scanner through the payload builders in payload.go and
// through the trash and whitelist entry points.

// TrashPaths moves paths into ~/.Trash. It is defined in trash.go together with
// the manifest that makes restoring possible; this file only keeps the exported
// surface of the scanner in one place.
