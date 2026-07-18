package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// Status holds the git repository status
type Status struct {
	Branch    string   `json:"branch"`
	IsClean   bool     `json:"is_clean"`
	Modified  []string `json:"modified"`
	Added     []string `json:"added"`
	Deleted   []string `json:"deleted"`
	Untracked []string `json:"untracked"`
}

// DiffResult holds the result of a git diff
type DiffResult struct {
	Diff   string `json:"diff"`
	Staged bool   `json:"staged"`
}

// Client provides git operations
type Client struct {
	logger *logrus.Logger
}

// NewClient creates a new git client
func NewClient(logger *logrus.Logger) *Client {
	return &Client{logger: logger}
}

// IsRepo checks if a directory is a git repository
func (c *Client) IsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// GetStatus returns the git status of a repository at the given path
func (c *Client) GetStatus(repoPath string) (*Status, error) {
	if !c.IsRepo(repoPath) {
		err := fmt.Errorf("%s is not a git repository", repoPath)
		c.logger.WithField("path", repoPath).Warn(err.Error())
		return nil, err
	}

	branch, err := c.getBranch(repoPath)
	if err != nil {
		return nil, err
	}

	status := &Status{
		Branch: branch,
	}

	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		c.logger.WithError(err).WithField("path", repoPath).Error("Failed to run git status")
		return nil, fmt.Errorf("failed to run git status: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		c.parseStatusLine(line, status)
	}

	status.IsClean = len(status.Modified) == 0 &&
		len(status.Added) == 0 &&
		len(status.Deleted) == 0 &&
		len(status.Untracked) == 0

	return status, nil
}

// getBranch returns the current branch name
func (c *Client) getBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		c.logger.WithError(err).WithField("path", repoPath).Error("Failed to get current branch")
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// parseStatusLine parses a single line of git status --porcelain output
func (c *Client) parseStatusLine(line string, status *Status) {
	if len(line) < 3 {
		return
	}

	indexStatus := line[0]
	workTreeStatus := line[1]
	fileName := line[3:]

	// Merged or unmerged states — skip complex cases
	if indexStatus == 'R' || indexStatus == 'C' {
		return
	}

	// Modified in index or working tree
	if indexStatus == 'M' || workTreeStatus == 'M' {
		status.Modified = append(status.Modified, fileName)
		return
	}

	// Added in index
	if indexStatus == 'A' {
		status.Added = append(status.Added, fileName)
		return
	}

	// Deleted in index or working tree
	if indexStatus == 'D' || workTreeStatus == 'D' {
		status.Deleted = append(status.Deleted, fileName)
		return
	}

	// Untracked
	if indexStatus == '?' && workTreeStatus == '?' {
		status.Untracked = append(status.Untracked, fileName)
		return
	}
}

// GetDiff returns the git diff for a repository
func (c *Client) GetDiff(repoPath string, staged bool) (*DiffResult, error) {
	if !c.IsRepo(repoPath) {
		err := fmt.Errorf("%s is not a git repository", repoPath)
		c.logger.WithField("path", repoPath).Warn(err.Error())
		return nil, err
	}

	args := []string{"-C", repoPath, "diff"}
	if staged {
		args = append(args, "--staged")
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		c.logger.WithError(err).WithField("path", repoPath).WithField("staged", staged).Error("Failed to run git diff")
		return nil, fmt.Errorf("failed to run git diff: %w", err)
	}

	return &DiffResult{
		Diff:   string(output),
		Staged: staged,
	}, nil
}

// GetTrackedFiles returns a list of git-tracked files in the repository
func (c *Client) GetTrackedFiles(repoPath string) ([]string, error) {
	if !c.IsRepo(repoPath) {
		err := fmt.Errorf("%s is not a git repository", repoPath)
		c.logger.WithField("path", repoPath).Warn(err.Error())
		return nil, err
	}

	cmd := exec.Command("git", "-C", repoPath, "ls-files")
	output, err := cmd.Output()
	if err != nil {
		c.logger.WithError(err).WithField("path", repoPath).Error("Failed to run git ls-files")
		return nil, fmt.Errorf("failed to run git ls-files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}