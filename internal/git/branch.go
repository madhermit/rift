package git

import (
	"fmt"
	"sort"

	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
)

type BranchInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Remote  string `json:"remote"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

func (r *Repo) ListBranches() ([]BranchInfo, error) {
	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("get HEAD: %w", err)
	}

	cfg, err := r.repo.Config()
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	refs, err := r.repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer refs.Close()

	branches := []BranchInfo{}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()

		commit, err := r.repo.CommitObject(ref.Hash())
		if err != nil {
			return fmt.Errorf("resolve commit for %s: %w", name, err)
		}

		bi := BranchInfo{
			Name:    name,
			Current: ref.Hash() == head.Hash() && ref.Name() == head.Name(),
			Remote:  trackingRemote(cfg, name),
			Date:    commit.Author.When.Format("2006-01-02 15:04"),
			Message: firstLine(commit.Message),
		}
		branches = append(branches, bi)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}

	sort.Slice(branches, func(i, j int) bool {
		if branches[i].Current != branches[j].Current {
			return branches[i].Current
		}
		return branches[i].Name < branches[j].Name
	})

	return branches, nil
}

func trackingRemote(cfg *config.Config, branchName string) string {
	bc, ok := cfg.Branches[branchName]
	if !ok {
		return ""
	}
	if bc.Remote == "" || bc.Merge == "" {
		return ""
	}
	return bc.Remote + "/" + bc.Merge.Short()
}
