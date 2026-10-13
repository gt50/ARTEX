//go:build windows

package evidence

import "golang.org/x/sys/windows"

// Windows cannot fsync a directory handle. Publish the already-flushed body
// using a write-through move instead, and propagate publication failures.
func publishBody(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
