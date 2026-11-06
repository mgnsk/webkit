package migrate_test

import (
	"testing"
	"testing/fstest"

	"github.com/mgnsk/webkit/migrate"
	"github.com/mgnsk/webkit/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "migrate")
}

var fsys = fstest.MapFS{
	"migrations/1_init.up.sql":   {Data: []byte(`CREATE TABLE things (id INTEGER PRIMARY KEY);`)},
	"migrations/1_init.down.sql": {Data: []byte(`DROP TABLE things;`)},
}

var _ = Describe("migrate", func() {
	It("migrates up idempotently and down", func() {
		db := sqlite.NewDB(":memory:").Connect()
		DeferCleanup(db.Close)

		Expect(migrate.Up(db.DB, fsys, "migrations")).To(Succeed())
		Expect(migrate.Up(db.DB, fsys, "migrations")).To(Succeed())

		_, err := db.Exec(`INSERT INTO things (id) VALUES (1)`)
		Expect(err).NotTo(HaveOccurred())

		Expect(migrate.Down(db.DB, fsys, "migrations")).To(Succeed())

		_, err = db.Exec(`INSERT INTO things (id) VALUES (1)`)
		Expect(err).To(HaveOccurred())
	})
})
