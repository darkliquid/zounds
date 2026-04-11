package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/darkliquid/zounds/pkg/core"
)

type TagUsage struct {
	Tag         core.Tag
	SampleCount int
}

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

func (r *Repository) RemoveTagFromSample(ctx context.Context, sampleID, tagID int64) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM sample_tags
		WHERE sample_id = ? AND tag_id = ?;
	`, sampleID, tagID); err != nil {
		return fmt.Errorf("remove tag %d from sample %d: %w", tagID, sampleID, err)
	}

	return nil
}

func (r *Repository) FindTagByName(ctx context.Context, name string) (core.Tag, error) {
	normalized := core.NormalizeTagName(name)
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, normalized_name, source, confidence, created_at
		FROM tags
		WHERE normalized_name = ?;
	`, normalized)

	var (
		tag       core.Tag
		createdAt sql.NullString
	)
	if err := row.Scan(&tag.ID, &tag.Name, &tag.NormalizedName, &tag.Source, &tag.Confidence, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return core.Tag{}, err
		}
		return core.Tag{}, fmt.Errorf("find tag %q: %w", normalized, err)
	}
	tag.CreatedAt = parseTime(createdAt)
	return tag, nil
}

func (r *Repository) ListTagUsage(ctx context.Context) ([]TagUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			t.id,
			t.name,
			t.normalized_name,
			t.source,
			t.confidence,
			t.created_at,
			COUNT(st.sample_id) AS sample_count
		FROM tags t
		LEFT JOIN sample_tags st ON st.tag_id = t.id
		GROUP BY t.id, t.name, t.normalized_name, t.source, t.confidence, t.created_at
		ORDER BY sample_count DESC, t.normalized_name ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("list tag usage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var usages []TagUsage
	for rows.Next() {
		var (
			usage     TagUsage
			createdAt sql.NullString
		)
		if err := rows.Scan(
			&usage.Tag.ID,
			&usage.Tag.Name,
			&usage.Tag.NormalizedName,
			&usage.Tag.Source,
			&usage.Tag.Confidence,
			&createdAt,
			&usage.SampleCount,
		); err != nil {
			return nil, fmt.Errorf("scan tag usage: %w", err)
		}
		usage.Tag.CreatedAt = parseTime(createdAt)
		usages = append(usages, usage)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag usage: %w", err)
	}

	return usages, nil
}

func (r *Repository) ListTagsForSample(ctx context.Context, sampleID int64) ([]core.Tag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			t.id,
			t.name,
			t.normalized_name,
			t.source,
			t.confidence,
			t.created_at
		FROM tags t
		JOIN sample_tags st ON st.tag_id = t.id
		WHERE st.sample_id = ?
		ORDER BY t.normalized_name;
	`, sampleID)
	if err != nil {
		return nil, fmt.Errorf("list tags for sample %d: %w", sampleID, err)
	}
	defer func() { _ = rows.Close() }()

	var tags []core.Tag
	for rows.Next() {
		var (
			tag       core.Tag
			createdAt sql.NullString
		)
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.NormalizedName, &tag.Source, &tag.Confidence, &createdAt); err != nil {
			return nil, fmt.Errorf("scan sample tag: %w", err)
		}
		tag.CreatedAt = parseTime(createdAt)
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sample tags: %w", err)
	}

	return tags, nil
}

func (r *Repository) FindSamplesByTag(ctx context.Context, name string) ([]core.Sample, error) {
	normalized := core.NormalizeTagName(name)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			s.id,
			s.source_root,
			s.path,
			s.relative_path,
			s.file_name,
			s.extension,
			s.format,
			s.size_bytes,
			s.modified_at,
			s.scanned_at
		FROM samples s
		JOIN sample_tags st ON st.sample_id = s.id
		JOIN tags t ON t.id = st.tag_id
		WHERE t.normalized_name = ?
		ORDER BY s.path;
	`, normalized)
	if err != nil {
		return nil, fmt.Errorf("find samples by tag %q: %w", normalized, err)
	}
	defer func() { _ = rows.Close() }()

	var samples []core.Sample
	for rows.Next() {
		sample, err := scanSample(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tagged sample: %w", err)
		}
		samples = append(samples, sample)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tagged samples: %w", err)
	}

	return samples, nil
}

func (r *Repository) DB() *sql.DB {
	return r.db
}
