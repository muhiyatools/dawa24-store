package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Pipeline orchestrates the MariaDB to PostgreSQL migration.
type Pipeline struct {
	sourceDSN   string
	targetDSN   string
	batchSize   int
	validator   *Validator
	transformer *Transformer
	log         *slog.Logger
}

// NewPipeline creates a new ETL migration pipeline.
func NewPipeline(sourceDSN, targetDSN string, batchSize int, log *slog.Logger) *Pipeline {
	return &Pipeline{
		sourceDSN:   sourceDSN,
		targetDSN:   targetDSN,
		batchSize:   batchSize,
		validator:   NewValidator(),
		transformer: NewTransformer(),
		log:         log,
	}
}

// Run executes the migration.
func (p *Pipeline) Run(ctx context.Context, verifyOnly bool) error {
	p.log.Info("connecting to databases")
	sourceDB, err := sql.Open("mysql", p.sourceDSN)
	if err != nil {
		return fmt.Errorf("source db: %w", err)
	}
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", p.targetDSN)
	if err != nil {
		return fmt.Errorf("target db: %w", err)
	}
	defer targetDB.Close()

	if verifyOnly {
		p.log.Info("running verification gates only")
		return p.Verify(ctx, sourceDB, targetDB)
	}

	p.log.Info("starting users migration stage")
	if err := p.migrateUsers(ctx, sourceDB, targetDB); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	p.log.Info("running post-migration verification gates")
	return p.Verify(ctx, sourceDB, targetDB)
}

func (p *Pipeline) migrateUsers(ctx context.Context, source *sql.DB, target *sql.DB) error {
	rows, err := source.QueryContext(ctx, `SELECT id, name, email, password, COALESCE(phone, ''), created_at FROM users;`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO identity.users (id, public_id, email, password_hash, name, role, status, language, phone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var src SourceUser
		var createdAt *time.Time
		if err := rows.Scan(&src.ID, &src.Name, &src.Email, &src.Password, &src.Phone, &createdAt); err != nil {
			return err
		}
		if createdAt != nil {
			src.CreatedAt = *createdAt
		} else {
			src.CreatedAt = time.Now()
		}

		if err := p.validator.ValidateUser(&src); err != nil {
			p.log.Warn("skipping invalid user", "id", src.ID, "error", err)
			continue
		}

		tgt := p.transformer.TransformUser(&src)

		nameJSON := fmt.Sprintf(`{"ar":"%s","en":"%s"}`, tgt.Name["ar"], tgt.Name["en"])
		_, err := stmt.ExecContext(ctx,
			tgt.ID, tgt.PublicID, tgt.Email, tgt.PasswordHash, nameJSON,
			"customer", tgt.Status, tgt.Language, tgt.Phone, tgt.CreatedAt, tgt.UpdatedAt,
		)
		if err != nil {
			return err
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	p.log.Info("users migration completed", "count", count)
	return nil
}

// Verify runs 2-way verification gates between source and target databases.
func (p *Pipeline) Verify(ctx context.Context, source *sql.DB, target *sql.DB) error {
	var srcCount, tgtCount int
	if err := source.QueryRowContext(ctx, `SELECT COUNT(*) FROM users;`).Scan(&srcCount); err != nil {
		return fmt.Errorf("verify source count: %w", err)
	}
	if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM identity.users;`).Scan(&tgtCount); err != nil {
		return fmt.Errorf("verify target count: %w", err)
	}

	p.log.Info("verification gate results", "source_users", srcCount, "target_users", tgtCount)
	if srcCount > tgtCount {
		return fmt.Errorf("verification gate failed: target has fewer records (%d) than source (%d)", tgtCount, srcCount)
	}
	return nil
}
