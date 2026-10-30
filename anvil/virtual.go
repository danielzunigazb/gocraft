package anvil

import "os"

// Virtual is a Save with no files behind it: it reads as a world that never generated a column.
// A world that is generated rather than loaded uses one, so the code that works with a save's
// folder (edits, player state) has something to hold.
func Virtual(dir string, level Level) *Save {
	return &Save{Dir: dir, Level: level, files: map[[2]int32]*os.File{}}
}
