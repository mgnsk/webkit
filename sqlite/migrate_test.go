package sqlite_test

import (
	"testing/fstest"

	"github.com/mgnsk/webkit/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var migrationsFS = fstest.MapFS{
	"migrations/1_init.up.sql":   {Data: []byte(`CREATE TABLE things (id INTEGER PRIMARY KEY);`)},
	"migrations/1_init.down.sql": {Data: []byte(`DROP TABLE things;`)},
}

var _ = Describe("Migrate", func() {
	It("migrates up idempotently and down", func() {
		Expect(sqlite.MigrateUp(db.DB, migrationsFS, "migrations")).To(Succeed())
		Expect(sqlite.MigrateUp(db.DB, migrationsFS, "migrations")).To(Succeed())

		_, err := db.Exec(`INSERT INTO things (id) VALUES (1)`)
		Expect(err).NotTo(HaveOccurred())

		Expect(sqlite.MigrateDown(db.DB, migrationsFS, "migrations")).To(Succeed())

		_, err = db.Exec(`INSERT INTO things (id) VALUES (1)`)
		Expect(err).To(HaveOccurred())
	})
})
