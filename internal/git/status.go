package git

import (
	"fmt"
	"sort"
)

type StatusFile struct {
	Path           string `json:"path"`
	StagingStatus  string `json:"staging_status"`
	WorktreeStatus string `json:"worktree_status"`
}

func (r *Repo) StatusFiles() ([]StatusFile, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}

	files := []StatusFile{}
	for path, s := range status {
		staging := statusCodeToString(s.Staging)
		worktree := statusCodeToString(s.Worktree)
		if staging == "" && worktree == "" {
			continue
		}
		files = append(files, StatusFile{
			Path:           path,
			StagingStatus:  staging,
			WorktreeStatus: worktree,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

func StatusChar(status string) string {
	switch status {
	case "Modified":
		return "M"
	case "Added":
		return "A"
	case "Deleted":
		return "D"
	case "Renamed":
		return "R"
	case "Copied":
		return "C"
	case "Untracked":
		return "?"
	default:
		return " "
	}
}
