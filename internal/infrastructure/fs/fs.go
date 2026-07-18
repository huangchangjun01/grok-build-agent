package fs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultMaxSize = 100 * 1024 // 100KB

// FileReader reads file contents with pagination
type FileReader struct {
	logger *logrus.Logger
}

// ReadResult holds the result of reading a file
type ReadResult struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

// NewFileReader creates a new file reader
func NewFileReader(logger *logrus.Logger) *FileReader {
	return &FileReader{logger: logger}
}

// ReadFile reads a file with optional line range.
// If startLine and endLine are 0, reads the entire file.
// Limits output to maxSize bytes.
func (r *FileReader) ReadFile(path string, startLine, endLine int, maxSize int) (*ReadResult, error) {
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}

	file, err := os.Open(path)
	if err != nil {
		r.logger.WithError(err).WithField("path", path).Error("Failed to open file")
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // support long lines up to 1MB
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		r.logger.WithError(err).WithField("path", path).Error("Failed to read file")
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	totalLines := len(lines)

	// Apply line range filtering
	if startLine == 0 && endLine == 0 {
		startLine = 1
		endLine = totalLines
	}
	if startLine < 1 {
		startLine = 1
	}
	if endLine > totalLines {
		endLine = totalLines
	}
	if startLine > endLine {
		startLine = 1
		endLine = totalLines
	}

	// Build content with size limit
	var sb strings.Builder
	truncated := false
	actualStart := startLine
	actualEnd := endLine
	linesIncluded := 0

	for i := startLine - 1; i < endLine; i++ {
		line := lines[i]
		if sb.Len()+len(line)+1 > maxSize {
			truncated = true
			break
		}
		if i > startLine-1 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
		linesIncluded++
		actualEnd = i + 1
	}

	if linesIncluded == 0 && startLine <= totalLines {
		// Even the first line is too large, include it truncated
		line := lines[startLine-1]
		if len(line) > maxSize {
			sb.WriteString(line[:maxSize])
			truncated = true
			actualEnd = startLine
		}
	}

	return &ReadResult{
		Path:       path,
		Content:    sb.String(),
		StartLine:  actualStart,
		EndLine:    actualEnd,
		TotalLines: totalLines,
		Truncated:  truncated,
	}, nil
}

// FileWriter writes/creates files
type FileWriter struct {
	logger *logrus.Logger
}

// WriteResult holds the result of writing a file
type WriteResult struct {
	Path       string `json:"path"`
	Size       int    `json:"size"`
	Created    bool   `json:"created"`
	Overwritten bool  `json:"overwritten"`
}

// NewFileWriter creates a new file writer
func NewFileWriter(logger *logrus.Logger) *FileWriter {
	return &FileWriter{logger: logger}
}

// WriteFile writes content to a file, creating directories if needed
func (w *FileWriter) WriteFile(path string, content string) (*WriteResult, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		w.logger.WithError(err).WithField("dir", dir).Error("Failed to create directory")
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Check if file exists
	_, err := os.Stat(path)
	exists := err == nil
	created := !exists

	data := []byte(content)
	if err := os.WriteFile(path, data, 0644); err != nil {
		w.logger.WithError(err).WithField("path", path).Error("Failed to write file")
		return nil, fmt.Errorf("failed to write file %s: %w", path, err)
	}

	w.logger.WithField("path", path).WithField("size", len(data)).Debug("File written successfully")

	return &WriteResult{
		Path:       path,
		Size:       len(data),
		Created:    created,
		Overwritten: exists,
	}, nil
}

// DirLister lists directory contents
type DirLister struct {
	logger *logrus.Logger
}

// ListResult holds the result of listing a directory
type ListResult struct {
	Path  string     `json:"path"`
	Files []FileInfo `json:"files"`
}

// FileInfo represents file information
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

// NewDirLister creates a new directory lister
func NewDirLister(logger *logrus.Logger) *DirLister {
	return &DirLister{logger: logger}
}

// ListDir lists the contents of a directory
func (l *DirLister) ListDir(path string) (*ListResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		l.logger.WithError(err).WithField("path", path).Error("Failed to read directory")
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			l.logger.WithError(err).WithField("name", entry.Name()).Warn("Failed to get file info, skipping")
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(path, entry.Name()),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime(),
		})
	}

	// Sort by name
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return &ListResult{
		Path:  path,
		Files: files,
	}, nil
}

// GrepSearcher searches file contents using regex
type GrepSearcher struct {
	logger *logrus.Logger
}

// GrepResult holds a single grep match
type GrepResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// NewGrepSearcher creates a new grep searcher
func NewGrepSearcher(logger *logrus.Logger) *GrepSearcher {
	return &GrepSearcher{logger: logger}
}

// Search searches for a pattern in files under a directory.
// pattern: regex pattern, dir: directory to search, include: glob pattern for files to include
func (s *GrepSearcher) Search(pattern string, dir string, include string, maxResults int) ([]*GrepResult, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		s.logger.WithError(err).WithField("pattern", pattern).Error("Invalid regex pattern")
		return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
	}

	if maxResults <= 0 {
		maxResults = 1000
	}

	var results []*GrepResult

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			s.logger.WithError(err).WithField("path", path).Warn("Error walking path, skipping")
			return nil
		}

		if info.IsDir() {
			// Skip common directories to ignore
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == ".svn" {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply glob filter to file name
		if include != "" {
			matched, matchErr := filepath.Match(include, filepath.Base(path))
			if matchErr != nil || !matched {
				return nil
			}
		}

		file, openErr := os.Open(path)
		if openErr != nil {
			s.logger.WithError(openErr).WithField("path", path).Warn("Failed to open file, skipping")
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				results = append(results, &GrepResult{
					File:    path,
					Line:    lineNum,
					Content: line,
				})
				if len(results) >= maxResults {
					return fmt.Errorf("max results reached") // signal to stop walking
				}
			}
		}
		return nil
	}

	err = filepath.Walk(dir, walkFn)
	if err != nil && !strings.Contains(err.Error(), "max results reached") {
		s.logger.WithError(err).WithField("dir", dir).Error("Failed to walk directory")
		return nil, fmt.Errorf("failed to search directory %s: %w", dir, err)
	}

	s.logger.WithFields(logrus.Fields{
		"pattern":    pattern,
		"dir":        dir,
		"results":    len(results),
		"maxResults": maxResults,
	}).Debug("Grep search completed")

	return results, nil
}

// GlobSearcher finds files matching a glob pattern
type GlobSearcher struct {
	logger *logrus.Logger
}

// NewGlobSearcher creates a new glob searcher
func NewGlobSearcher(logger *logrus.Logger) *GlobSearcher {
	return &GlobSearcher{logger: logger}
}

// Search finds files matching a glob pattern in a directory
func (s *GlobSearcher) Search(pattern string, dir string) ([]string, error) {
	fullPattern := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		s.logger.WithError(err).WithField("pattern", fullPattern).Error("Failed to glob")
		return nil, fmt.Errorf("failed to glob pattern %q: %w", fullPattern, err)
	}

	s.logger.WithFields(logrus.Fields{
		"pattern": fullPattern,
		"matches": len(matches),
	}).Debug("Glob search completed")

	return matches, nil
}

// SearchReplacer performs search and replace in files
type SearchReplacer struct {
	logger *logrus.Logger
}

// ReplaceResult holds the result of a search/replace operation
type ReplaceResult struct {
	Path     string `json:"path"`
	Replaced int    `json:"replaced"`
	Diff     string `json:"diff"`
}

// NewSearchReplacer creates a new search/replace tool
func NewSearchReplacer(logger *logrus.Logger) *SearchReplacer {
	return &SearchReplacer{logger: logger}
}

// Replace replaces oldStr with newStr in a file.
// Returns the diff of changes.
func (r *SearchReplacer) Replace(path string, oldStr string, newStr string) (*ReplaceResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.logger.WithError(err).WithField("path", path).Error("Failed to read file for replace")
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	original := string(data)
	count := strings.Count(original, oldStr)
	if count == 0 {
		r.logger.WithField("path", path).Warn("Search string not found in file")
		return &ReplaceResult{
			Path:     path,
			Replaced: 0,
			Diff:     "",
		}, nil
	}

	replaced := strings.ReplaceAll(original, oldStr, newStr)

	if err := os.WriteFile(path, []byte(replaced), 0644); err != nil {
		r.logger.WithError(err).WithField("path", path).Error("Failed to write replaced file")
		return nil, fmt.Errorf("failed to write file %s: %w", path, err)
	}

	diff := generateDiff(original, replaced, oldStr, newStr)

	r.logger.WithFields(logrus.Fields{
		"path":       path,
		"replacements": count,
	}).Debug("Search/replace completed")

	return &ReplaceResult{
		Path:     path,
		Replaced: count,
		Diff:     diff,
	}, nil
}

// generateDiff creates a simple line-based diff showing changes
func generateDiff(original, replaced, oldStr, newStr string) string {
	origLines := strings.Split(original, "\n")
	replLines := strings.Split(replaced, "\n")

	var sb strings.Builder
	maxLen := len(origLines)
	if len(replLines) > maxLen {
		maxLen = len(replLines)
	}

	for i := 0; i < maxLen; i++ {
		origLine := ""
		replLine := ""
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(replLines) {
			replLine = replLines[i]
		}

		if origLine != replLine {
			if origLine != "" {
				sb.WriteString(fmt.Sprintf("- %s\n", origLine))
			}
			if replLine != "" {
				sb.WriteString(fmt.Sprintf("+ %s\n", replLine))
			}
		}
	}

	return sb.String()
}