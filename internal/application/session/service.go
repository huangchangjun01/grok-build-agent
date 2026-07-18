package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	domain "github.com/spacexc/grok-build/internal/domain/session"
	"github.com/spacexc/grok-build/internal/infrastructure/llm"
	"github.com/spacexc/grok-build/pkg/config"
)

// Service provides session management operations.
type Service struct {
	repo      domain.SessionRepository
	llmClient *llm.Client
	config    *config.Config
	logger    *logrus.Logger
}

// NewService creates a new session Service.
func NewService(repo domain.SessionRepository, llmClient *llm.Client, cfg *config.Config, logger *logrus.Logger) *Service {
	return &Service{
		repo:      repo,
		llmClient: llmClient,
		config:    cfg,
		logger:    logger,
	}
}

// CreateSession creates a new session with the given working directory.
func (s *Service) CreateSession(workDir string) (*domain.Session, error) {
	session := &domain.Session{
		ID:        uuid.New().String(),
		WorkDir:   workDir,
		Title:     "",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	if err := s.repo.Create(session); err != nil {
		s.logger.WithError(err).WithField("work_dir", workDir).Error("failed to create session")
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.WithField("session_id", session.ID).Info("session created")
	return session, nil
}

// ListSessions returns sessions matching the given filter, sorted by updated_at descending.
func (s *Service) ListSessions(filter *domain.SessionFilter) ([]*domain.Session, error) {
	var f domain.SessionFilter
	if filter != nil {
		f = *filter
	}

	sessions, err := s.repo.List(f)
	if err != nil {
		s.logger.WithError(err).Error("failed to list sessions")
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// GetSession retrieves a session by ID.
func (s *Service) GetSession(id string) (*domain.Session, error) {
	session, err := s.repo.GetByID(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get session")
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// DeleteSession removes a session and its associated files.
func (s *Service) DeleteSession(id string) error {
	if err := s.repo.Delete(id); err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to delete session")
		return fmt.Errorf("failed to delete session: %w", err)
	}

	s.logger.WithField("session_id", id).Info("session deleted")
	return nil
}

// RenameSession updates the title of a session.
func (s *Service) RenameSession(id string, title string) error {
	session, err := s.repo.GetByID(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get session for rename")
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.Title = title

	if err := s.repo.Update(session); err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to rename session")
		return fmt.Errorf("failed to rename session: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"session_id": id,
		"title":      title,
	}).Info("session renamed")
	return nil
}

// ForkSession creates a new session by copying messages up to fromMessageIndex from the original session.
func (s *Service) ForkSession(id string, fromMessageIndex int) (*domain.Session, error) {
	original, err := s.repo.GetByID(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get original session for fork")
		return nil, fmt.Errorf("failed to get original session: %w", err)
	}

	if fromMessageIndex <= 0 {
		fromMessageIndex = 0
	}

	messages, err := s.repo.GetMessages(id, fromMessageIndex, 0)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get messages for fork")
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	newSession := &domain.Session{
		ID:           uuid.New().String(),
		Title:        original.Title + " (forked)",
		WorkDir:      original.WorkDir,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MessageCount: len(messages),
		IsActive:     true,
		Metadata:     original.Metadata,
	}

	if err := s.repo.Create(newSession); err != nil {
		s.logger.WithError(err).WithField("session_id", newSession.ID).Error("failed to create forked session")
		return nil, fmt.Errorf("failed to create forked session: %w", err)
	}

	for _, msg := range messages {
		copiedMsg := &domain.Message{
			ID:        uuid.New().String(),
			SessionID: newSession.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
			CreatedAt: msg.CreatedAt,
		}
		if err := s.repo.AddMessage(newSession.ID, copiedMsg); err != nil {
			s.logger.WithError(err).WithField("session_id", newSession.ID).Error("failed to copy message during fork")
			return nil, fmt.Errorf("failed to copy message: %w", err)
		}
	}

	// Update message count after copying
	newSession.MessageCount = len(messages)
	if err := s.repo.Update(newSession); err != nil {
		s.logger.WithError(err).WithField("session_id", newSession.ID).Warn("failed to update message count after fork")
	}

	s.logger.WithFields(logrus.Fields{
		"original_id": id,
		"forked_id":   newSession.ID,
		"messages":    len(messages),
	}).Info("session forked")
	return newSession, nil
}

// RewindSession truncates the session to the given message index.
func (s *Service) RewindSession(id string, toMessageIndex int) error {
	if toMessageIndex < 0 {
		return fmt.Errorf("toMessageIndex must be non-negative, got %d", toMessageIndex)
	}

	if err := s.repo.DeleteMessagesFrom(id, toMessageIndex); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"session_id": id,
			"to_index":   toMessageIndex,
		}).Error("failed to rewind session")
		return fmt.Errorf("failed to rewind session: %w", err)
	}

	count, err := s.repo.GetMessageCount(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Warn("failed to get message count after rewind")
	} else {
		session, getErr := s.repo.GetByID(id)
		if getErr != nil {
			s.logger.WithError(getErr).WithField("session_id", id).Warn("failed to get session for count update")
		} else {
			session.MessageCount = count
			session.UpdatedAt = time.Now()
			if updateErr := s.repo.Update(session); updateErr != nil {
				s.logger.WithError(updateErr).WithField("session_id", id).Warn("failed to update message count after rewind")
			}
		}
	}

	s.logger.WithFields(logrus.Fields{
		"session_id": id,
		"to_index":   toMessageIndex,
	}).Info("session rewound")
	return nil
}

// ExportSession exports session messages in the specified format ("json" or "markdown").
// Returns the exported data, content type, and any error.
func (s *Service) ExportSession(id string, format string) ([]byte, string, error) {
	session, err := s.repo.GetByID(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get session for export")
		return nil, "", fmt.Errorf("failed to get session: %w", err)
	}

	messages, err := s.repo.GetMessages(id, 0, 0)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get messages for export")
		return nil, "", fmt.Errorf("failed to get messages: %w", err)
	}

	switch strings.ToLower(format) {
	case "json":
		data, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			s.logger.WithError(err).WithField("session_id", id).Error("failed to marshal messages to JSON")
			return nil, "", fmt.Errorf("failed to marshal messages: %w", err)
		}
		return data, "application/json", nil

	case "markdown":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Session: %s\n", session.Title))
		sb.WriteString("\n")

		for _, msg := range messages {
			switch msg.Role {
			case domain.RoleUser:
				sb.WriteString("## User\n")
			case domain.RoleAssistant:
				sb.WriteString("## Assistant\n")
			case domain.RoleSystem:
				sb.WriteString("## System\n")
			case domain.RoleTool:
				sb.WriteString("## Tool\n")
			default:
				sb.WriteString(fmt.Sprintf("## %s\n", msg.Role))
			}
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		}

		data := []byte(sb.String())
		return data, "text/markdown", nil

	default:
		return nil, "", fmt.Errorf("unsupported export format: %s (supported: json, markdown)", format)
	}
}

// GenerateSummary generates a concise title for the session using the LLM.
func (s *Service) GenerateSummary(id string) (string, error) {
	messages, err := s.repo.GetMessages(id, 10, 0)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get messages for summary")
		return "", fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) == 0 {
		return "", fmt.Errorf("no messages available for summary generation")
	}

	// Build a conversation string from the first few messages
	var convBuilder strings.Builder
	for _, msg := range messages {
		convBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	model := s.config.LLM.DefaultModel
	if s.config.LLM.SessionSummaryModel != "" {
		model = s.config.LLM.SessionSummaryModel
	}

	req := &llm.ChatRequest{
		Model: model,
		Messages: []llm.ChatMessage{
			{
				Role:    "system",
				Content: "You are a helpful assistant that generates short, concise titles for conversations. Generate a title of at most 6 words that captures the main topic of the conversation. Respond with ONLY the title, no other text.",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Generate a short title (max 6 words) for this conversation:\n\n%s", convBuilder.String()),
			},
		},
		Temperature: 0.3,
		MaxTokens:   50,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to generate summary")
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	title := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Update the session title
	session, err := s.repo.GetByID(id)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to get session for title update")
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	session.Title = title
	if err := s.repo.Update(session); err != nil {
		s.logger.WithError(err).WithField("session_id", id).Error("failed to update session title")
		return "", fmt.Errorf("failed to update session title: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"session_id": id,
		"title":      title,
	}).Info("session summary generated")
	return title, nil
}

// SearchSessions performs a full-text search across sessions.
func (s *Service) SearchSessions(query string) ([]*domain.Session, error) {
	sessions, err := s.repo.Search(query)
	if err != nil {
		s.logger.WithError(err).WithField("query", query).Error("failed to search sessions")
		return nil, fmt.Errorf("failed to search sessions: %w", err)
	}

	return sessions, nil
}

// GetMessages retrieves messages for a session with pagination.
func (s *Service) GetMessages(sessionID string, limit, offset int) ([]*domain.Message, error) {
	messages, err := s.repo.GetMessages(sessionID, limit, offset)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("failed to get messages")
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	return messages, nil
}

// AddMessage adds a message to a session, auto-generating a summary if this is the first user message.
func (s *Service) AddMessage(sessionID string, msg *domain.Message) error {
	if msg == nil {
		return fmt.Errorf("message must not be nil")
	}

	if err := s.repo.AddMessage(sessionID, msg); err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("failed to add message")
		return fmt.Errorf("failed to add message: %w", err)
	}

	// Auto-generate summary if this is the first user message and session has no title
	if msg.Role == domain.RoleUser {
		count, err := s.repo.GetMessageCount(sessionID)
		if err != nil {
			s.logger.WithError(err).WithField("session_id", sessionID).Warn("failed to get message count for auto-summary")
		} else if count == 1 {
			session, err := s.repo.GetByID(sessionID)
			if err != nil {
				s.logger.WithError(err).WithField("session_id", sessionID).Warn("failed to get session for auto-summary")
			} else if session.Title == "" {
				go func() {
					if _, err := s.GenerateSummary(sessionID); err != nil {
						s.logger.WithError(err).WithField("session_id", sessionID).Warn("auto-summary generation failed")
					}
				}()
			}
		}
	}

	return nil
}