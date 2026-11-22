package gitmanager

// PlanOutputRequest names where a plan is exported and what it must avoid.
type PlanOutputRequest struct {
	Path              string
	WorkingDir        string
	Inspection        GitInspection
	ObserveFilesystem func(path string) (FilesystemObservation, error)
}

// prohibitedAreas are the physical areas a plan may never be written into
// (F008-FR-182): the project root, its Git and common directories, and the
// inspected linked worktrees with their Git directories.
func prohibitedAreas(inspection GitInspection) []string {
	areas := []string{string(inspection.ProjectIdentity.ReversiblePathBytes())}
	observation := inspection.Observation
	for _, path := range []ObservedPath{observation.GitDir, observation.CommonDir} {
		if !path.IsZero() {
			areas = append(areas, string(path.Bytes()))
		}
	}
	for _, worktree := range observation.LinkedWorktrees() {
		areas = append(areas, string(worktree.WorktreePath.Bytes()))
		if !worktree.GitDirPath.IsZero() {
			areas = append(areas, string(worktree.GitDirPath.Bytes()))
		}
	}
	return areas
}

func insideArea(path, area string) bool {
	return path == area || (len(path) > len(area) && path[:len(area)] == area && (area == "/" || path[len(area)] == '/'))
}
