package scanner

// This file exposes the pieces of the scanner the app layer calls. The scan
// methods themselves stay unexported, as they were in the Swift original: the
// app talks to the scanner through the two payload builders in payload.go and
// through MovePathsToTrash.

// MovePathsToTrash moves paths into ~/.Trash and reports how many it moved.
// Labels the frontend sends are truncated at the first space before the move,
// exactly as the Swift app did.
func (s *Scanner) MovePathsToTrash(rawPaths []string) int {
	return s.movePathsToTrash(rawPaths)
}
