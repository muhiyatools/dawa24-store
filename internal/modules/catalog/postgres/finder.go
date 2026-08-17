package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetFirstFinderQuestion returns the active entry question.
func (r *Repository) GetFirstFinderQuestion(ctx context.Context) (*catalog.FinderQuestion, error) {
	var q catalog.FinderQuestion
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, question, type, is_first, is_active, created_at, updated_at FROM catalog.finder_questions WHERE is_first = true AND is_active = true ORDER BY id ASC LIMIT 1;`
		err := tx.QueryRow(txCtx, query).Scan(&q.ID, &q.Question, &q.Type, &q.IsFirst, &q.IsActive, &q.CreatedAt, &q.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("finder_question")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// GetFinderQuestionByID fetches one question.
func (r *Repository) GetFinderQuestionByID(ctx context.Context, id int64) (*catalog.FinderQuestion, error) {
	var q catalog.FinderQuestion
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, question, type, is_first, is_active, created_at, updated_at FROM catalog.finder_questions WHERE id = $1;`
		err := tx.QueryRow(txCtx, query, id).Scan(&q.ID, &q.Question, &q.Type, &q.IsFirst, &q.IsActive, &q.CreatedAt, &q.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("finder_question")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// ListFinderOptions returns a question's answer choices.
func (r *Repository) ListFinderOptions(ctx context.Context, questionID int64) ([]*catalog.FinderOption, error) {
	var list []*catalog.FinderOption
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, question_id, label, next_question_id, result_id, sort_order FROM catalog.finder_options WHERE question_id = $1 ORDER BY sort_order ASC, id ASC;`
		rows, err := tx.Query(txCtx, query, questionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o catalog.FinderOption
			if err := rows.Scan(&o.ID, &o.QuestionID, &o.Label, &o.NextQuestionID, &o.ResultID, &o.SortOrder); err != nil {
				return err
			}
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, err
}

// GetFinderResultByID fetches the terminal recommendation.
func (r *Repository) GetFinderResultByID(ctx context.Context, id int64) (*catalog.FinderResult, error) {
	var res catalog.FinderResult
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, title, description FROM catalog.finder_results WHERE id = $1;`
		err := tx.QueryRow(txCtx, query, id).Scan(&res.ID, &res.Title, &res.Description)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("finder_result")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// ListFinderQuestions returns all questions for the admin builder.
func (r *Repository) ListFinderQuestions(ctx context.Context) ([]*catalog.FinderQuestion, error) {
	var list []*catalog.FinderQuestion
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, question, type, is_first, is_active, created_at, updated_at FROM catalog.finder_questions ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q catalog.FinderQuestion
			if err := rows.Scan(&q.ID, &q.Question, &q.Type, &q.IsFirst, &q.IsActive, &q.CreatedAt, &q.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &q)
		}
		return rows.Err()
	})
	return list, err
}

// CreateFinderQuestion inserts a finder question.
func (r *Repository) CreateFinderQuestion(ctx context.Context, q *catalog.FinderQuestion) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO catalog.finder_questions (question, type, is_first, is_active) VALUES ($1, $2, $3, true) RETURNING id, created_at, updated_at;`
		return tx.QueryRow(txCtx, query, q.Question, q.Type, q.IsFirst).Scan(&q.ID, &q.CreatedAt, &q.UpdatedAt)
	})
}

// CreateFinderOption inserts an answer choice.
func (r *Repository) CreateFinderOption(ctx context.Context, o *catalog.FinderOption) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO catalog.finder_options (question_id, label, next_question_id, result_id, sort_order) VALUES ($1, $2, $3, $4, $5) RETURNING id;`
		return tx.QueryRow(txCtx, query, o.QuestionID, o.Label, o.NextQuestionID, o.ResultID, o.SortOrder).Scan(&o.ID)
	})
}

// CreateFinderResult inserts a terminal recommendation.
func (r *Repository) CreateFinderResult(ctx context.Context, res *catalog.FinderResult) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO catalog.finder_results (title, description) VALUES ($1, $2) RETURNING id;`
		return tx.QueryRow(txCtx, query, res.Title, res.Description).Scan(&res.ID)
	})
}

// ListFinderResults returns all terminal recommendations.
func (r *Repository) ListFinderResults(ctx context.Context) ([]*catalog.FinderResult, error) {
	var list []*catalog.FinderResult
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, title, description FROM catalog.finder_results ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var res catalog.FinderResult
			if err := rows.Scan(&res.ID, &res.Title, &res.Description); err != nil {
				return err
			}
			list = append(list, &res)
		}
		return rows.Err()
	})
	return list, err
}
