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

// Optimize runs a thorough PRAGMA optimize, checking all tables, and merges
// the b-trees of the given FTS5 tables. Run it on startup and periodically,
// e.g. with app.Every.
func Optimize(ctx context.Context, db *sql.DB, ftsTables ...string) error {
	start := time.Now()

	for _, table := range ftsTables {
		if !identRe.MatchString(table) {
			return fmt.Errorf("invalid FTS table name %q", table)
		}
	}

	if _, err := db.ExecContext(ctx, `PRAGMA optimize=0x10002`); err != nil {
		return err
	}

	for _, table := range ftsTables {
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO "%[1]s"("%[1]s") VALUES('optimize')`, table)); err != nil {
			return err
		}
	}

	slog.Info(fmt.Sprintf("database optimization finished in %v", time.Since(start)))

	return nil
}
