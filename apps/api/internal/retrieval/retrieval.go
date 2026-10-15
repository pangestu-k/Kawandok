package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"office-ai-agent/apps/api/internal/authz"
	"office-ai-agent/apps/api/internal/embedding"
)

type Evidence struct {
	CitationRef string
	ChunkID     string
	DocumentID  string
	VersionID   string
	Title       string
	Content     string
	PageStart   *int
	PageEnd     *int
	SectionRef  *string
}

type Searcher struct {
	db       *sql.DB
	embedder embedding.Provider
	topK     int
	// maxDistance is the largest cosine distance a chunk may have and still be
	// treated as evidence. A permissive gate makes every question look
	// document-related, which forced the chat into grounded-only mode.
	maxDistance float64
}

// DefaultMaxDistance mirrors the historical 0.95 gate, kept permissive for
// compatibility when callers do not configure one.
const DefaultMaxDistance = 0.95

func NewSearcher(db *sql.DB, embedder embedding.Provider, topK int, maxDistance float64) *Searcher {
	if maxDistance <= 0 || maxDistance > 2 {
		maxDistance = DefaultMaxDistance
	}
	return &Searcher{db: db, embedder: embedder, topK: topK, maxDistance: maxDistance}
}

type row struct {
	chunkID    string
	documentID string
	versionID  string
	title      string
	content    string
	pageStart  *int
	pageEnd    *int
	sectionRef *string
}

const (
	selection = `
		SELECT c.id::text, d.id::text, v.id::text, d.title, c.content,
		       c.page_start, c.page_end, c.section_ref
		FROM document_chunks c
		JOIN document_versions v ON v.id = c.document_version_id
		JOIN documents d ON d.id = v.document_id
	`
	readiness = `d.deleted_at IS NULL AND v.status = 'ready'`
)

func scopeFilter(scope authz.Scope, args *[]any) string {
	where := []string{readiness}
	where = authz.AppendScopeClause(where, args, scope, "read", "d.id")
	return strings.Join(where, " AND ")
}

func (s *Searcher) ftsCandidates(ctx context.Context, scope authz.Scope, query string) ([]row, error) {
	args := []any{query, BuildPrefixQuery(query), s.topK}
	where := scopeFilter(scope, &args)
	querySQL := fmt.Sprintf(`
		%s
		WHERE %s
		  AND (
		      c.tsv @@ websearch_to_tsquery('simple', unaccent($1))
		      OR c.tsv @@ to_tsquery('simple', $2)
		  )
		ORDER BY ts_rank(c.tsv, websearch_to_tsquery('simple', unaccent($1))) DESC,
		         ts_rank(c.tsv, to_tsquery('simple', $2)) DESC,
		         c.id
		LIMIT $3
	`, selection, where)
	return s.rows(ctx, querySQL, args...)
}

func (s *Searcher) vectorCandidates(ctx context.Context, scope authz.Scope, vector []float32) ([]row, error) {
	args := []any{vectorLiteral(vector), s.topK}
	where := scopeFilter(scope, &args)
	querySQL := fmt.Sprintf(`
		%s
		WHERE %s AND c.embedding <=> $1::vector < %v
		ORDER BY c.embedding <=> $1::vector, c.id
		LIMIT $2
	`, selection, where, s.maxDistance)
	return s.rows(ctx, querySQL, args...)
}

func (s *Searcher) rows(ctx context.Context, query string, args ...any) ([]row, error) {
	dbRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("retrieval: %w", err)
	}
	defer dbRows.Close()

	out := []row{}
	for dbRows.Next() {
		var r row
		if err := dbRows.Scan(&r.chunkID, &r.documentID, &r.versionID, &r.title,
			&r.content, &r.pageStart, &r.pageEnd, &r.sectionRef); err != nil {
			return nil, fmt.Errorf("retrieval scan: %w", err)
		}
		out = append(out, r)
	}
	return out, dbRows.Err()
}

func (s *Searcher) byIDs(ctx context.Context, scope authz.Scope, ids []string) ([]row, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(ids)+1)
	args := []any{}
	for _, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", len(args)))
	}
	where := scopeFilter(scope, &args)
	idsClause := strings.Join(placeholders, ",")
	querySQL := fmt.Sprintf(`
		%s
		JOIN unnest(ARRAY[%s]) WITH ORDINALITY AS wanted(chunk_id, ord) ON c.id = wanted.chunk_id::uuid
		WHERE %s
		ORDER BY wanted.ord
	`, selection, idsClause, where)
	return s.rows(ctx, querySQL, args...)
}

func (s *Searcher) Retrieve(ctx context.Context, scope authz.Scope, query string) ([]Evidence, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Evidence{}, nil
	}

	fts, err := s.ftsCandidates(ctx, scope, query)
	if err != nil {
		return nil, err
	}
	var vec []row
	if s.embedder != nil {
		vectors, err := s.embedder.Embed(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("retrieval: embed query: %w", err)
		}
		vec, err = s.vectorCandidates(ctx, scope, vectors[0])
		if err != nil {
			return nil, err
		}
	}

	fused := reciprocalRankFusion(fts, vec, s.topK)
	if len(fused) == 0 {
		return []Evidence{}, nil
	}

	full, err := s.byIDs(ctx, scope, fused)
	if err != nil {
		return nil, err
	}

	evidence := make([]Evidence, 0, len(full))
	for i, r := range full {
		evidence = append(evidence, Evidence{
			CitationRef: fmt.Sprintf("%d", i+1),
			ChunkID:     r.chunkID,
			DocumentID:  r.documentID,
			VersionID:   r.versionID,
			Title:       r.title,
			Content:     r.content,
			PageStart:   r.pageStart,
			PageEnd:     r.pageEnd,
			SectionRef:  r.sectionRef,
		})
	}
	return evidence, nil
}

func reciprocalRankFusion(a, b []row, limit int) []string {
	scores := map[string]float64{}
	order := map[string]int{}
	add := func(rows []row, weight float64) {
		for i, r := range rows {
			scores[r.chunkID] += weight / float64(i+1)
			if _, ok := order[r.chunkID]; !ok {
				order[r.chunkID] = len(order)
			}
		}
	}
	add(a, 1.0)
	add(b, 1.0)

	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			si, sj := scores[ids[i]], scores[ids[j]]
			if sj > si || (sj == si && order[ids[j]] < order[ids[i]]) {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, 0, len(vector))
	for _, v := range vector {
		parts = append(parts, fmt.Sprintf("%g", v))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func BuildPrefixQuery(query string) string {
	fields := strings.Fields(strings.ToLower(query))
	clean := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ",.;:!?()\"'")
		if len(f) > 2 && !strings.ContainsAny(f, `\()*:&|'`) {
			clean = append(clean, f+":*")
		}
	}
	return strings.Join(clean, " | ")
}
