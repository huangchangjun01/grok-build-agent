package session

// SessionFilter defines filtering criteria for listing sessions.
type SessionFilter struct {
	IsActive *bool  `json:"is_active,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
	OrderBy  string `json:"order_by,omitempty"`
}

// SessionRepository defines the persistence interface for sessions.
type SessionRepository interface {
	// Create inserts a new session.
	Create(session *Session) error

	// GetByID retrieves a session by its ID.
	GetByID(id string) (*Session, error)

	// List returns sessions matching the given filter.
	List(filter SessionFilter) ([]*Session, error)

	// Update modifies an existing session.
	Update(session *Session) error

	// Delete removes a session by its ID.
	Delete(id string) error

	// Search performs a text search across sessions.
	Search(query string) ([]*Session, error)

	// AddMessage appends a message to a session.
	AddMessage(sessionID string, msg *Message) error

	// GetMessages retrieves messages for a session with pagination.
	GetMessages(sessionID string, limit, offset int) ([]*Message, error)

	// GetMessageCount returns the total number of messages in a session.
	GetMessageCount(sessionID string) (int, error)

	// DeleteMessagesFrom removes messages starting from the given index (inclusive).
	DeleteMessagesFrom(sessionID string, fromIndex int) error
}