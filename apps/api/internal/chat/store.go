package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"office-ai-agent/apps/api/internal/authz"
)

type Citation struct {
	Ref        string  `json:"ref"`
	DocumentID string  `json:"document_id"`
	VersionID  string  `json:"version_id"`
	Title      string  `json:"title"`
	PageStart  *int    `json:"page_start,omitempty"`
	PageEnd    *int    `json:"page_end,omitempty"`
	SectionRef *string `json:"section_ref,omitempty"`
	Snippet    string  `json:"snippet"`
}

type Message struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Body      string     `json:"body"`
	Citations []Citation `json:"citations,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type Store interface {
	CreateSession(ctx context.Context, ownerID, title string) (string, error)
	SessionOwner(ctx context.Context, sessionID string) (string, bool, error)
	AppendMessage(ctx context.Context, sessionID, role, body string, citations []Citation, clientRequestID string) (string, error)
	Messages(ctx context.Context, sessionID string) ([]Message, error)
	FindByClientRequestID(ctx context.Context, sessionID, clientRequestID string) (*Message, bool, error)
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) CreateSession(ctx context.Context, ownerID, title string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO chat_sessions (owner_user_id, title)
		VALUES ($1::uuid, $2)
		RETURNING id::text
	`, ownerID, title).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("chat: create session: %w", err)
	}
	return id, nil
}

func (r *Repo) SessionOwner(ctx context.Context, sessionID string) (string, bool, error) {
	var owner string
	err := r.db.QueryRowContext(ctx,
		`SELECT owner_user_id::text FROM chat_sessions WHERE id = $1::uuid`,
		sessionID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("chat: session lookup: %w", err)
	}
	return owner, true, nil
}

func (r *Repo) AppendMessage(ctx context.Context, sessionID, role, body string, citations []Citation, clientRequestID string) (string, error) {
	var citationsJSON any
	if len(citations) > 0 {
		encoded, err := json.Marshal(citations)
		if err != nil {
			return "", fmt.Errorf("chat: marshal citations: %w", err)
		}
		citationsJSON = encoded
	}
	var requestID any
	if clientRequestID != "" {
		requestID = clientRequestID
	}
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (session_id, role, body, citations_json, client_request_id)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text
	`, sessionID, role, body, citationsJSON, requestID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("chat: append message: %w", err)
	}
	return id, nil
}

func (r *Repo) Messages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, role, body, COALESCE(citations_json::text, ''), created_at
		FROM chat_messages
		WHERE session_id = $1::uuid
		ORDER BY created_at, id
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("chat: messages: %w", err)
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var m Message
		var citationsRaw string
		if err := rows.Scan(&m.ID, &m.Role, &m.Body, &citationsRaw, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("chat: message scan: %w", err)
		}
		if citationsRaw != "" {
			var citations []Citation
			if err := json.Unmarshal([]byte(citationsRaw), &citations); err != nil {
				return nil, fmt.Errorf("chat: citations decode: %w", err)
			}
			m.Citations = citations
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repo) FindByClientRequestID(ctx context.Context, sessionID, clientRequestID string) (*Message, bool, error) {
	var (
		m            Message
		citationsRaw string
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, role, body, COALESCE(citations_json::text, ''), created_at
		FROM chat_messages
		WHERE session_id = $1::uuid AND client_request_id = $2
	`, sessionID, clientRequestID).Scan(&m.ID, &m.Role, &m.Body, &citationsRaw, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("chat: find by request id: %w", err)
	}
	if citationsRaw != "" {
		var citations []Citation
		if err := json.Unmarshal([]byte(citationsRaw), &citations); err != nil {
			return nil, false, fmt.Errorf("chat: citations decode: %w", err)
		}
		m.Citations = citations
	}
	return &m, true, nil
}

var _ Store = (*Repo)(nil)

type CheckerAuthorizer struct {
	checker *authz.Checker
}

func NewAuthorizer(checker *authz.Checker) *CheckerAuthorizer {
	return &CheckerAuthorizer{checker: checker}
}

func (a *CheckerAuthorizer) CanRead(ctx context.Context, principalID, documentID string) (bool, error) {
	return a.checker.CanByUserID(ctx, principalID, "read", documentID)
}
