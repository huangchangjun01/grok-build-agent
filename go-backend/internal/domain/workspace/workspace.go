package workspace

import "time"

// Workspace represents a working directory with its state.
type Workspace struct {
	RootDir   string     `json:"root_dir"`
	GitBranch string     `json:"git_branch"`
	GitStatus string     `json:"git_status"`
	Files     []FileInfo `json:"files"`
}

// FileInfo holds metadata about a file in the workspace.
type FileInfo struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

// FileChange represents a change made to a file.
type FileChange struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}