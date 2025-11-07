package db

import (
	"github.com/jmoiron/sqlx"
	"time"
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
`
	_, err := db.Exec(ddl)
	return err
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
