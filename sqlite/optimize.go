package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RunOptimizer runs a periodic SQLite database optimizer task. It runs a
// thorough optimize on startup and a regular one every 24 hours. Each run
// also merges the b-trees of the given FTS5 tables.
func RunOptimizer(ctx context.Context, db *sql.DB, ftsTables ...string) error {
	var ftsStmts []string
	for _, table := range ftsTables {
		if !identRe.MatchString(table) {
			return fmt.Errorf("invalid FTS table name %q", table)
		}
		ftsStmts = append(ftsStmts, fmt.Sprintf(`INSERT INTO "%[1]s"("%[1]s") VALUES('optimize')`, table))
	}

	optimize := func(pragma string) error {
		start := time.Now()

		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return err
		}

		for _, stmt := range ftsStmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}

		slog.Info(fmt.Sprintf("database optimization finished in %v", time.Since(start)))

		return nil
	}

	if err := optimize(`PRAGMA optimize=0x10002`); err != nil {
		return err
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if err := optimize(`PRAGMA optimize`); err != nil {
				return err
			}
		}
	}
}
