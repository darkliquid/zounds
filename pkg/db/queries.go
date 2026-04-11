package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/darkliquid/zounds/pkg/core"
)

func (r *Repository) EnsureTag(ctx context.Context, tag core.Tag) (int64, error) {
	normalized := tag.NormalizedName
	if normalized == "" {
		normalized = core.NormalizeTagName(tag.Name)
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO tags (name, normalized_name, source, confidence)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(normalized_name) DO UPDATE SET
			name = excluded.name,
			source = excluded.source,
			confidence = excluded.confidence
		RETURNING id;
	`, tag.Name, normalized, tag.Source, tag.Confidence)

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("ensure tag %q: %w", tag.Name, err)
	}

	return id, nil
}

func (r *Repository) AttachTagToSample(ctx context.Context, sampleID, tagID int64) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO sample_tags (sample_id, tag_id)
		VALUES (?, ?)
		ON CONFLICT(sample_id, tag_id) DO NOTHING;
	`, sampleID, tagID); err != nil {
		return fmt.Errorf("attach tag %d to sample %d: %w", tagID, sampleID, err)
	}

	return nil
}

func (r *Repository) DB() *sql.DB {
	return r.db
}
