//go:build !windows && !linux && !darwin && !freebsd

package ui

// volumeUsage has no implementation on this platform, so category volumes are
// reported as not visible rather than guessed at. See the statfs version for
// why sabtop measures these paths at all.
func volumeUsage(path string) (free, total uint64, ok bool) {
	return 0, 0, false
}
