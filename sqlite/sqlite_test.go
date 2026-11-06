package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "sqlite")
}

var db *bun.DB

var _ = BeforeEach(func() {
	db = sqlite.NewDB(":memory:").WithForeignKeys().Connect()
	DeferCleanup(db.Close)

	_, err := db.Exec(`
		CREATE TABLE parents (id INTEGER PRIMARY KEY);
		CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parents(id));
		CREATE VIRTUAL TABLE docs_fts USING fts5(body);
		INSERT INTO parents (id) VALUES (1);
	`)
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("NormalizeError", func() {
	It("maps no rows to NotFound", func() {
		var id int64
		err := sqlite.NormalizeError(db.QueryRow(`SELECT id FROM parents WHERE id = 2`).Scan(&id))
		Expect(errors.Is(err, webkit.NotFound)).To(BeTrue())
	})

	It("maps unique violations to AlreadyExists", func() {
		_, err := db.Exec(`INSERT INTO parents (id) VALUES (1)`)
		Expect(errors.Is(sqlite.NormalizeError(err), webkit.AlreadyExists)).To(BeTrue())
	})

	It("maps FTS syntax errors to InvalidValue", func() {
		var body string
		err := sqlite.NormalizeError(db.QueryRow(`SELECT body FROM docs_fts WHERE docs_fts MATCH 'AND'`).Scan(&body))
		Expect(errors.Is(err, webkit.InvalidValue)).To(BeTrue(), "%v", err)
	})

	It("enforces foreign keys", func() {
		_, err := db.Exec(`INSERT INTO children (id, parent_id) VALUES (1, 2)`)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("WithErrorChecking", func() {
	It("fails with PreconditionFailed when no rows were affected", func() {
		err := sqlite.WithErrorChecking(db.Exec(`DELETE FROM parents WHERE id = 2`))
		Expect(errors.Is(err, webkit.PreconditionFailed)).To(BeTrue())
	})

	It("succeeds when rows were affected", func() {
		Expect(sqlite.WithErrorChecking(db.Exec(`DELETE FROM parents WHERE id = 1`))).To(Succeed())
	})
})

var _ = Describe("Optimize", func() {
	segments := func(table string) int {
		GinkgoHelper()
		var n int
		Expect(db.QueryRow(`SELECT count(*) FROM "` + table + `_data"`).Scan(&n)).To(Succeed())
		return n
	}

	insertSeparately := func(table string) {
		GinkgoHelper()
		for _, body := range []string{"one", "two", "three"} {
			_, err := db.Exec(`INSERT INTO "`+table+`" (body) VALUES (?)`, body)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	It("merges the segments of all FTS5 tables", func() {
		_, err := db.Exec(`
			create virtual table "we""ird" using FTS5(body);
			CREATE VIRTUAL TABLE other_fts USING fts5(body, content='');
		`)
		Expect(err).NotTo(HaveOccurred())

		for _, table := range []string{"docs_fts", `we""ird`} {
			insertSeparately(table)
		}

		before := segments("docs_fts")
		beforeWeird := segments(`we""ird`)

		Expect(sqlite.Optimize(context.Background(), db.DB)).To(Succeed())

		Expect(segments("docs_fts")).To(BeNumerically("<", before))
		Expect(segments(`we""ird`)).To(BeNumerically("<", beforeWeird))
	})

	It("succeeds without FTS5 tables", func() {
		_, err := db.Exec(`DROP TABLE docs_fts`)
		Expect(err).NotTo(HaveOccurred())
		Expect(sqlite.Optimize(context.Background(), db.DB)).To(Succeed())
	})
})
