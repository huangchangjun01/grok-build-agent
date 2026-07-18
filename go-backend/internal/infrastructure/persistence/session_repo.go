package persistence

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/spacexc/go-backend/internal/domain/session"
)

// SessionRepo implements session.SessionRepository using SQLite for metadata
// and JSONL files for message storage, with FTS5 for full-text search.
type SessionRepo struct {
	db          *sql.DB
	jsonlDir    string
	sessionsDir string
	logger      *logrus.Logger

	mu       sync.RWMutex
	fileMu   map[string]*sync.Mutex
}

// NewSessionRepo creates a new SessionRepo.
func NewSessionRepo(db *sql.DB, jsonlDir, sessionsDir string, logger *logrus.Logger) *SessionRepo {
	if err := os.MkdirAll(jsonlDir, 0755); err != nil {
		logger.WithError(err).WithField("dir", jsonlDir).Warn("failed to create jsonl directory")
	}
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		logger.WithError(err).WithField("dir", sessionsDir).Warn("failed to create sessions directory")
	}
	return &SessionRepo{
		db:          db,
		jsonlDir:    jsonlDir,
		sessionsDir: sessionsDir,
		logger:      logger,
		fileMu:      make(map[string]*sync.Mutex),
	}
}

// getFileMutex returns the mutex for a given session's JSONL file.
func (r *SessionRepo) getFileMutex(sessionID string) *sync.Mutex {
	r.mu.RLock()
	mu, ok := r.fileMu[sessionID]
	r.mu.RUnlock()
	if ok {
		return mu
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// double-check after acquiring write lock
	if mu, ok = r.fileMu[sessionID]; ok {
		return mu
	}
	mu = &sync.Mutex{}
	r.fileMu[sessionID] = mu
	return mu
}

// jsonlPath returns the full path to the JSONL file for a session.
func (r *SessionRepo) jsonlPath(sessionID string) string {
	return filepath.Join(r.jsonlDir, sessionID+".jsonl")
}

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

// Create inserts a new session into the database and creates an empty JSONL file.
func (r *SessionRepo) Create(s *session.Session) error {
	if s.ID == "" {
		return fmt.Errorf("session ID must not be empty")
	}

	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}

	isActive := 0
	if s.IsActive {
		isActive = 1
	}

	_, err := r.db.Exec(
		`INSERT INTO sessions (id, title, work_dir, created_at, updated_at, message_count, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.WorkDir, s.CreatedAt, s.UpdatedAt, s.MessageCount, isActive,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", s.ID).Error("failed to insert session into db")
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Create empty JSONL file
	mu := r.getFileMutex(s.ID)
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(r.jsonlPath(s.ID), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", s.ID).Error("failed to create jsonl file")
		return fmt.Errorf("failed to create jsonl file for session: %w", err)
	}
	f.Close()

	r.logger.WithField("session_id", s.ID).Info("session created")
	return nil
}

// GetByID retrieves a session by ID and loads its messages from the JSONL file.
func (r *SessionRepo) GetByID(id string) (*session.Session, error) {
	s := &session.Session{}
	var isActive int
	var createdAt, updatedAt string

	err := r.db.QueryRow(
		`SELECT id, title, work_dir, created_at, updated_at, message_count, is_active
		 FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.WorkDir, &createdAt, &updatedAt, &s.MessageCount, &isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		r.logger.WithError(err).WithField("session_id", id).Error("failed to query session")
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	s.IsActive = isActive == 1

	return s, nil
}

// List returns sessions matching the given filter.
func (r *SessionRepo) List(filter session.SessionFilter) ([]*session.Session, error) {
	query := `SELECT id, title, work_dir, created_at, updated_at, message_count, is_active FROM sessions WHERE 1=1`
	args := []interface{}{}

	if filter.IsActive != nil {
		if *filter.IsActive {
			query += " AND is_active = 1"
		} else {
			query += " AND is_active = 0"
		}
	}

	orderBy := "updated_at DESC"
	if filter.OrderBy != "" {
		switch filter.OrderBy {
		case "created_at":
			orderBy = "created_at DESC"
		case "title":
			orderBy = "title ASC"
		case "message_count":
			orderBy = "message_count DESC"
		}
	}
	query += " ORDER BY " + orderBy

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		r.logger.WithError(err).Error("failed to list sessions")
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*session.Session
	for rows.Next() {
		s := &session.Session{}
		var isActive int
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Title, &s.WorkDir, &createdAt, &updatedAt, &s.MessageCount, &isActive); err != nil {
			r.logger.WithError(err).Error("failed to scan session row")
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		s.IsActive = isActive == 1
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return sessions, nil
}

// Update modifies an existing session in the database.
func (r *SessionRepo) Update(s *session.Session) error {
	if s.ID == "" {
		return fmt.Errorf("session ID must not be empty")
	}

	s.UpdatedAt = time.Now()
	isActive := 0
	if s.IsActive {
		isActive = 1
	}

	res, err := r.db.Exec(
		`UPDATE sessions SET title=?, work_dir=?, updated_at=?, message_count=?, is_active=? WHERE id=?`,
		s.Title, s.WorkDir, s.UpdatedAt, s.MessageCount, isActive, s.ID,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", s.ID).Error("failed to update session")
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", s.ID)
	}

	r.logger.WithField("session_id", s.ID).Debug("session updated")
	return nil
}

// Delete removes a session from the database and deletes its JSONL file.
func (r *SessionRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", id).Error("failed to delete session from db")
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	// Remove JSONL file
	mu := r.getFileMutex(id)
	mu.Lock()
	os.Remove(r.jsonlPath(id))
	mu.Unlock()

	// Clean up file mutex
	r.mu.Lock()
	delete(r.fileMu, id)
	r.mu.Unlock()

	r.logger.WithField("session_id", id).Info("session deleted")
	return nil
}

// Search performs a search across all session messages using LIKE,
// and returns the sessions that contain matching messages.
func (r *SessionRepo) Search(query string) ([]*session.Session, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT s.id, s.title, s.work_dir, s.created_at, s.updated_at, s.message_count, s.is_active
		 FROM sessions s
		 INNER JOIN session_messages sm ON sm.session_id = s.id
		 WHERE sm.content LIKE ?
		 ORDER BY s.updated_at DESC`,
		"%"+query+"%",
	)
	if err != nil {
		r.logger.WithError(err).WithField("query", query).Error("failed to search sessions")
		return nil, fmt.Errorf("failed to search sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*session.Session
	for rows.Next() {
		s := &session.Session{}
		var isActive int
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Title, &s.WorkDir, &createdAt, &updatedAt, &s.MessageCount, &isActive); err != nil {
			r.logger.WithError(err).Error("failed to scan search result row")
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		s.IsActive = isActive == 1
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows iteration error: %w", err)
	}

	return sessions, nil
}

// ---------------------------------------------------------------------------
// Message operations
// ---------------------------------------------------------------------------

// AddMessage appends a message to the session's JSONL file and inserts it into
// the session_messages table (which triggers the FTS5 index update).
func (r *SessionRepo) AddMessage(sessionID string, msg *session.Message) error {
	if sessionID == "" {
		return fmt.Errorf("session ID must not be empty")
	}
	if msg == nil {
		return fmt.Errorf("message must not be nil")
	}

	// Ensure the session exists
	var exists int
	err := r.db.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		return fmt.Errorf("failed to check session existence: %w", err)
	}

	msg.SessionID = sessionID
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Serialize tool calls to JSON for DB storage
	toolCallsJSON := ""
	if len(msg.ToolCalls) > 0 {
		data, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			r.logger.WithError(err).Warn("failed to marshal tool calls, storing empty")
		} else {
			toolCallsJSON = string(data)
		}
	}

	// Insert into session_messages table (triggers FTS5 index update)
	_, err = r.db.Exec(
		`INSERT INTO session_messages (session_id, role, content, tool_calls_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionID, string(msg.Role), msg.Content, toolCallsJSON, msg.CreatedAt,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", sessionID).Error("failed to insert message into db")
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Update session message count
	_, err = r.db.Exec(
		`UPDATE sessions SET message_count = message_count + 1, updated_at = ? WHERE id = ?`,
		time.Now(), sessionID,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", sessionID).Warn("failed to update message count")
	}

	// Append to JSONL file (thread-safe)
	mu := r.getFileMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(r.jsonlPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open jsonl file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("failed to write message to jsonl: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"role":       msg.Role,
	}).Debug("message appended")
	return nil
}

// GetMessages reads messages from the JSONL file with pagination.
// Returns messages in chronological order (oldest first).
func (r *SessionRepo) GetMessages(sessionID string, limit, offset int) ([]*session.Message, error) {
	mu := r.getFileMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	f, err := os.Open(r.jsonlPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open jsonl file: %w", err)
	}
	defer f.Close()

	var messages []*session.Message
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large messages
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	lineIdx := 0
	for scanner.Scan() {
		if lineIdx < offset {
			lineIdx++
			continue
		}
		if limit > 0 && len(messages) >= limit {
			break
		}

		msg := &session.Message{}
		if err := json.Unmarshal(scanner.Bytes(), msg); err != nil {
			r.logger.WithError(err).WithField("session_id", sessionID).WithField("line", lineIdx).Warn("failed to unmarshal message line, skipping")
			lineIdx++
			continue
		}
		messages = append(messages, msg)
		lineIdx++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan jsonl file: %w", err)
	}

	return messages, nil
}

// GetMessageCount returns the total number of messages in a session
// by querying the session_messages table.
func (r *SessionRepo) GetMessageCount(sessionID string) (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM session_messages WHERE session_id = ?`,
		sessionID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}
	return count, nil
}

// DeleteMessagesFrom removes messages starting from the given index (inclusive)
// from both the JSONL file and the database.
func (r *SessionRepo) DeleteMessagesFrom(sessionID string, fromIndex int) error {
	if fromIndex < 0 {
		return fmt.Errorf("fromIndex must be non-negative, got %d", fromIndex)
	}

	mu := r.getFileMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	jsonlPath := r.jsonlPath(sessionID)

	// Read all messages from JSONL, keep only those before fromIndex
	f, err := os.Open(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open jsonl file: %w", err)
	}

	var keptMessages []*session.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	lineIdx := 0
	for scanner.Scan() {
		if lineIdx >= fromIndex {
			lineIdx++
			continue
		}
		msg := &session.Message{}
		if err := json.Unmarshal(scanner.Bytes(), msg); err != nil {
			lineIdx++
			continue
		}
		keptMessages = append(keptMessages, msg)
		lineIdx++
	}
	f.Close()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan jsonl file: %w", err)
	}

	// Rewrite JSONL file with only kept messages
	tmpPath := jsonlPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp jsonl file: %w", err)
	}

	encodeErr := false
	for _, msg := range keptMessages {
		line, err := json.Marshal(msg)
		if err != nil {
			encodeErr = true
			continue
		}
		line = append(line, '\n')
		if _, err := tmpFile.Write(line); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write temp jsonl: %w", err)
		}
	}
	tmpFile.Close()

	if err := os.Rename(tmpPath, jsonlPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to replace jsonl file: %w", err)
	}

	if encodeErr {
		r.logger.WithField("session_id", sessionID).Warn("some messages failed to marshal during truncation")
	}

	// Delete messages from DB: delete messages whose rowid corresponds to lines >= fromIndex.
	// Since session_messages.id is auto-increment and messages are inserted in order,
	// we delete messages with id >= the id of the fromIndex-th message.
	// We use a subquery to find the id of the (fromIndex+1)-th message (0-based to 1-based).
	delResult, err := r.db.Exec(
		`DELETE FROM session_messages WHERE session_id = ? AND id IN (
			SELECT id FROM session_messages WHERE session_id = ?
			ORDER BY id ASC LIMIT -1 OFFSET ?
		)`,
		sessionID, sessionID, fromIndex,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", sessionID).Error("failed to delete messages from db")
		return fmt.Errorf("failed to delete messages from db: %w", err)
	}

	rowsDeleted, _ := delResult.RowsAffected()

	// Update message_count in sessions table
	_, err = r.db.Exec(
		`UPDATE sessions SET message_count = MAX(0, message_count - ?), updated_at = ? WHERE id = ?`,
		rowsDeleted, time.Now(), sessionID,
	)
	if err != nil {
		r.logger.WithError(err).WithField("session_id", sessionID).Warn("failed to update message count after deletion")
	}

	r.logger.WithFields(logrus.Fields{
		"session_id":   sessionID,
		"from_index":   fromIndex,
		"rows_deleted": rowsDeleted,
	}).Debug("messages truncated")
	return nil
}

// Ensure SessionRepo implements session.SessionRepository.
var _ session.SessionRepository = (*SessionRepo)(nil)