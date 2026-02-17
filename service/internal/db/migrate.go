package db

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store { return &Store{db: db} }

func Setup(db *sqlx.DB) error {
	ddl := `
CREATE TABLE IF NOT EXISTS ai_metrics_daily (
  day DATE NOT NULL,
  metric TEXT NOT NULL,
  model TEXT NULL,
  count BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (day, metric, model)
);

CREATE TABLE IF NOT EXISTS rag_documents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  chunk_index INT NOT NULL DEFAULT 0,
  content TEXT NOT NULL,
  embedding JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rag_documents_owner ON rag_documents(owner_id, created_at DESC);
`
	_, err := db.Exec(ddl)
	return err
}

// RAG document operations

type RagDocument struct {
	ID         string    `db:"id" json:"id"`
	OwnerID    string    `db:"owner_id" json:"ownerId"`
	Title      string    `db:"title" json:"title"`
	Source     string    `db:"source" json:"source"`
	ChunkIndex int      `db:"chunk_index" json:"chunkIndex"`
	Content    string    `db:"content" json:"content"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
}

func (s *Store) InsertRagChunk(ownerID, title, source string, chunkIndex int, content string, embedding []float32) (string, error) {
	embJSON, _ := json.Marshal(embedding)
	var id string
	err := s.db.QueryRowx(
		`INSERT INTO rag_documents (owner_id, title, source, chunk_index, content, embedding) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		ownerID, title, source, chunkIndex, content, string(embJSON)).Scan(&id)
	return id, err
}

func (s *Store) ListRagDocuments(ownerID string) ([]RagDocument, error) {
	rows := []RagDocument{}
	err := s.db.Select(&rows,
		`SELECT id, owner_id, title, source, chunk_index, content, created_at FROM rag_documents WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 200`, ownerID)
	return rows, err
}

func (s *Store) DeleteRagDocument(id, ownerID string) error {
	_, err := s.db.Exec(`DELETE FROM rag_documents WHERE id=$1 AND owner_id=$2`, id, ownerID)
	return err
}

func (s *Store) DeleteRagDocumentsByTitle(title, ownerID string) error {
	_, err := s.db.Exec(`DELETE FROM rag_documents WHERE title=$1 AND owner_id=$2`, title, ownerID)
	return err
}

type RagChunkWithEmbedding struct {
	ID        string `db:"id"`
	Content   string `db:"content"`
	Title     string `db:"title"`
	Source    string `db:"source"`
	Embedding string `db:"embedding"`
}

func (s *Store) AllRagChunksWithEmbeddings(ownerID string) ([]RagChunkWithEmbedding, error) {
	rows := []RagChunkWithEmbedding{}
	err := s.db.Select(&rows,
		`SELECT id, content, title, source, embedding::text AS embedding FROM rag_documents WHERE owner_id=$1 AND embedding IS NOT NULL`, ownerID)
	return rows, err
}

func (s *Store) IncDaily(metric, model string, delta int64) error {
	if delta == 0 {
		return nil
	}
	q := `INSERT INTO ai_metrics_daily (day, metric, model, count)
VALUES (CURRENT_DATE, $1, $2, $3)
ON CONFLICT (day, metric, model)
DO UPDATE SET count = ai_metrics_daily.count + EXCLUDED.count`
	_, err := s.db.Exec(q, metric, nullIfEmpty(model), delta)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type DailyRow struct {
	Day    time.Time `db:"day" json:"day"`
	Metric string    `db:"metric" json:"metric"`
	Model  *string   `db:"model" json:"model,omitempty"`
	Count  int64     `db:"count" json:"count"`
}

func (s *Store) GetDaily(days int) ([]DailyRow, error) {
	if days <= 0 {
		days = 7
	}
	q := `SELECT day, metric, model, count FROM ai_metrics_daily
          WHERE day >= CURRENT_DATE - ($1::int - 1)
          ORDER BY day ASC, metric ASC, model NULLS FIRST`
	out := []DailyRow{}
	if err := s.db.Select(&out, q, days); err != nil {
		return nil, err
	}
	return out, nil
}
