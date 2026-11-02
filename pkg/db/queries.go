package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

type TagUsage struct {
	Tag         core.Tag `json:"tag"`
	SampleCount int      `json:"sample_count"`
}

func (r *Repository) EnsureTag(ctx context.Context, tag core.Tag) (int64, error) {
	normalized := tag.NormalizedName
	if normalized == "" {
		normalized = core.NormalizeTagName(tag.Name)
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO tags (name, normalized_name)
		VALUES (?, ?)
		ON CONFLICT(normalized_name) DO UPDATE SET
			name = excluded.name
		RETURNING id;
	`, tag.Name, normalized)

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("ensure tag %q: %w", tag.Name, err)
	}

	return id, nil
}

func (r *Repository) AttachTagToSample(ctx context.Context, sampleID, tagID int64, source string, confidence float64) error {
	if source == "" {
		source = "unknown"
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO sample_tags (sample_id, tag_id, source, confidence)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(sample_id, tag_id) DO UPDATE SET
			source = excluded.source,
			confidence = excluded.confidence;
	`, sampleID, tagID, source, confidence); err != nil {
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

// ReplaceSampleTags atomically replaces the tag set attached to a sample.
// Provenance for tags already attached to the sample is preserved; new tags
// use the source and confidence supplied by the caller.
func (r *Repository) ReplaceSampleTags(ctx context.Context, sampleID int64, tags []core.Tag) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace sample tags transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingSampleID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM samples WHERE id = ?`, sampleID).Scan(&existingSampleID); err != nil {
		return fmt.Errorf("find sample %d: %w", sampleID, err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT
			st.tag_id,
			t.normalized_name,
			st.source,
			st.confidence
		FROM sample_tags st
		JOIN tags t ON t.id = st.tag_id
		WHERE st.sample_id = ?;
	`, sampleID)
	if err != nil {
		return fmt.Errorf("list existing tags for sample %d: %w", sampleID, err)
	}

	existing := make(map[string]core.Tag)
	for rows.Next() {
		var tag core.Tag
		if err := rows.Scan(&tag.ID, &tag.NormalizedName, &tag.Source, &tag.Confidence); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan existing sample tag: %w", err)
		}
		existing[tag.NormalizedName] = tag
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate existing sample tags: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close existing sample tags: %w", err)
	}

	desired := make(map[string]core.Tag, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		normalized := tag.NormalizedName
		if normalized == "" {
			normalized = core.NormalizeTagName(name)
		} else {
			normalized = core.NormalizeTagName(normalized)
		}
		if normalized == "" {
			return fmt.Errorf("tag name must not be empty")
		}
		if name == "" {
			name = normalized
		}
		if _, ok := desired[normalized]; ok {
			continue
		}
		if tag.Source == "" {
			tag.Source = "unknown"
		}
		tag.Name = name
		tag.NormalizedName = normalized
		desired[normalized] = tag
	}

	for normalized, tag := range desired {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tags (name, normalized_name)
			VALUES (?, ?)
			ON CONFLICT(normalized_name) DO NOTHING;
		`, tag.Name, normalized); err != nil {
			return fmt.Errorf("insert replacement tag %q: %w", normalized, err)
		}

		var tagID int64
		if err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM tags
			WHERE normalized_name = ?;
		`, normalized).Scan(&tagID); err != nil {
			return fmt.Errorf("find replacement tag %q: %w", normalized, err)
		}

		if _, ok := existing[normalized]; ok {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sample_tags (sample_id, tag_id, source, confidence)
			VALUES (?, ?, ?, ?);
		`, sampleID, tagID, tag.Source, tag.Confidence); err != nil {
			return fmt.Errorf("attach replacement tag %q to sample %d: %w", normalized, sampleID, err)
		}
	}

	for normalized, tag := range existing {
		if _, ok := desired[normalized]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM sample_tags
			WHERE sample_id = ? AND tag_id = ?;
		`, sampleID, tag.ID); err != nil {
			return fmt.Errorf("remove replaced tag %q from sample %d: %w", normalized, sampleID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace sample tags for sample %d: %w", sampleID, err)
	}
	return nil
}

func (r *Repository) FindTagByName(ctx context.Context, name string) (core.Tag, error) {
	normalized := core.NormalizeTagName(name)
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, normalized_name, created_at
		FROM tags
		WHERE normalized_name = ?;
	`, normalized)

	var (
		tag       core.Tag
		createdAt sql.NullString
	)
	if err := row.Scan(&tag.ID, &tag.Name, &tag.NormalizedName, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return core.Tag{}, err
		}
		return core.Tag{}, fmt.Errorf("find tag %q: %w", normalized, err)
	}
	tag.Source = "unknown"
	tag.Confidence = 1.0
	tag.CreatedAt = parseTime(createdAt)
	return tag, nil
}

func (r *Repository) ListTagUsage(ctx context.Context) ([]TagUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			t.id,
			t.name,
			t.normalized_name,
			COALESCE(GROUP_CONCAT(DISTINCT st.source), 'unknown') AS sources,
			COALESCE(MAX(st.confidence), 1.0) AS confidence,
			t.created_at,
			COUNT(st.sample_id) AS sample_count
		FROM tags t
		LEFT JOIN sample_tags st ON st.tag_id = t.id
		GROUP BY t.id, t.name, t.normalized_name, t.created_at
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
			st.source,
			st.confidence,
			st.created_at
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

// ListTagsForSampleIDs retrieves tags for a slice of sample IDs in batches.
func (r *Repository) ListTagsForSampleIDs(ctx context.Context, sampleIDs []int64) (map[int64][]core.Tag, error) {
	result := make(map[int64][]core.Tag, len(sampleIDs))
	if len(sampleIDs) == 0 {
		return result, nil
	}

	const batchSize = 500
	for i := 0; i < len(sampleIDs); i += batchSize {
		end := i + batchSize
		if end > len(sampleIDs) {
			end = len(sampleIDs)
		}
		chunk := sampleIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf(`
			SELECT
				st.sample_id,
				t.id,
				t.name,
				t.normalized_name,
				st.source,
				st.confidence,
				st.created_at
			FROM sample_tags st
			JOIN tags t ON t.id = st.tag_id
			WHERE st.sample_id IN (%s)
			ORDER BY st.sample_id, t.normalized_name;
		`, strings.Join(placeholders, ","))

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list tags for sample ids: %w", err)
		}

		for rows.Next() {
			var (
				sampleID  int64
				tag       core.Tag
				createdAt sql.NullString
			)
			if err := rows.Scan(&sampleID, &tag.ID, &tag.Name, &tag.NormalizedName, &tag.Source, &tag.Confidence, &createdAt); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan sample tag: %w", err)
			}
			tag.CreatedAt = parseTime(createdAt)
			result[sampleID] = append(result[sampleID], tag)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate sample tags: %w", err)
		}
		_ = rows.Close()
	}

	return result, nil
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
