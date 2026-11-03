//go:build !windows

package anvil

import "os"

// renameReplace moves tmp over path, which may exist.
func renameReplace(tmp, path string) error { return os.Rename(tmp, path) }
