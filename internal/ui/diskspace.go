package ui

import "syscall"

// volumeUsage reports free and total bytes for the filesystem holding path.
//
// SABnzbd's API reports free space for exactly two volumes: the temp dir and
// complete_dir. A category configured with an absolute dir can live on a third
// volume that SABnzbd never measures — and when that one fills, the only clue
// is a generic "Too little diskspace" warning naming no path at all. sabtop
// therefore stats those paths itself, which works whenever the machine running
// sabtop can see the same mounts as SABnzbd (the usual case).
func volumeUsage(path string) (free, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	bs := uint64(st.Bsize)
	total = st.Blocks * bs
	free = st.Bavail * bs
	if total == 0 {
		return 0, 0, false
	}
	return free, total, true
}
