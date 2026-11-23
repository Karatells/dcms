package store

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

type item struct {
	Base
	Owner  int64  `json:"owner"`
	Name   string `json:"name"`
	Secret string `json:"-"`
	Active bool   `json:"active"`
}

func TestHelpers(t *testing.T) {
	if got := toSnakeCase("PassHash"); got != "pass_hash" {
		t.Fatalf("toSnakeCase(PassHash) = %q", got)
	}
	if got := toSnakeCase("IDUser"); got != "id_user" {
		t.Fatalf("toSnakeCase(IDUser) = %q", got)
	}
	if got := toSnakeCase("Email"); got != "email" {
		t.Fatalf("toSnakeCase(Email) = %q", got)
	}
	cols := inspectStruct[item]()
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}
	want := "owner,name,secret,active"
	if strings.Join(names, ",") != want {
		t.Fatalf("cols = %v, want %s", names, want)
	}
	longKey := strings.Repeat("a", 300)
	nk := normIdxKey(longKey)
	if len(nk) > 191 || !strings.HasPrefix(nk, "#h:") {
		t.Fatalf("normIdxKey = %q", nk)
	}
	norm, err := NormalizeDSN("dcms:pass@tcp(127.0.0.1:3306)/dcms")
	if err != nil {
		t.Fatalf("NormalizeDSN: %v", err)
	}
	if !strings.Contains(norm, "charset=utf8mb4") {
		t.Fatalf("expected utf8mb4 in %q", norm)
	}
}

func openTestDB(t *testing.T) (*DB, *Table[item, *item]) {
	t.Helper()
	dsn := os.Getenv("DCMS_TEST_DSN")
	if dsn == "" {
		t.Skip("DCMS_TEST_DSN не задан — пропуск интеграционного теста MySQL/MariaDB")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	tb := NewTable[item](db, "test_store_item").
		Index("owner", func(v *item) string { return I(v.Owner) }).
		Index("secret", func(v *item) string { return v.Secret })
	if err := db.Load(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, tb
}

func resetTestTable(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.SQL().Exec("DROP TABLE IF EXISTS `test_store_item`"); err != nil {
		t.Fatal(err)
	}
}

func TestPersistAndQuery(t *testing.T) {
	dsn := os.Getenv("DCMS_TEST_DSN")
	if dsn == "" {
		t.Skip("DCMS_TEST_DSN не задан — пропуск интеграционного теста MySQL/MariaDB")
	}
	// чистый старт
	tmp, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	resetTestTable(t, tmp)
	tmp.Close()

	db, tb := openTestDB(t)
	for i := 0; i < 100; i++ {
		tb.Insert(&item{Owner: int64(i % 3), Name: "n", Secret: "Sec" + I(int64(i)), Active: i%2 == 0})
	}
	tb.Update(5, func(v *item) { v.Owner = 9; v.Secret = "UpdSecret" })
	tb.Delete(7)
	if tb.CountBy("owner", "9") != 1 || tb.Len() != 99 {
		t.Fatalf("counts: owner9=%d len=%d", tb.CountBy("owner", "9"), tb.Len())
	}
	// Проверка регистрозависимости индекса (utf8mb4_bin)
	if _, ok := tb.OneBy("secret", "updsecret"); ok {
		t.Fatal("index must be case-sensitive")
	}
	if v, ok := tb.OneBy("secret", "UpdSecret"); !ok || v.ID != 5 {
		t.Fatalf("OneBy secret failed: %+v %v", v, ok)
	}
	rows, total := tb.Select(Opts[item]{Desc: true, Limit: 3})
	if total != 99 || len(rows) != 3 || rows[0].ID != 100 {
		t.Fatalf("desc %v %d", rows, total)
	}
	tb.Insert(&item{Owner: 1, Name: "after"})

	var dump bytes.Buffer
	if err := db.DumpSQL(&dump); err != nil {
		t.Fatalf("DumpSQL: %v", err)
	}
	if !strings.Contains(dump.String(), "INSERT INTO `test_store_item`") {
		t.Fatalf("unexpected dump: %s", dump.String())
	}
	stats, err := db.Stats()
	if err != nil || stats.Version == "" || stats.Tables < 1 {
		t.Fatalf("Stats: %+v err=%v", stats, err)
	}
	db.Close()

	db2, tb2 := openTestDB(t)
	defer func() {
		resetTestTable(t, db2)
		db2.Close()
	}()
	if tb2.Len() != 100 {
		t.Fatalf("len %d", tb2.Len())
	}
	if v, ok := tb2.Get(5); !ok || v.Owner != 9 || v.Secret != "UpdSecret" {
		t.Fatalf("update lost: %+v %v", v, ok)
	}
	if tb2.Has(7) {
		t.Fatal("delete lost")
	}
	if id := tb2.Insert(&item{}); id != 102 {
		t.Fatalf("next id %d", id)
	}
}

func TestConcurrent(t *testing.T) {
	dsn := os.Getenv("DCMS_TEST_DSN")
	if dsn == "" {
		t.Skip("DCMS_TEST_DSN не задан — пропуск интеграционного теста MySQL/MariaDB")
	}
	tmp, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	resetTestTable(t, tmp)
	tmp.Close()

	db, tb := openTestDB(t)
	defer func() {
		resetTestTable(t, db)
		db.Close()
	}()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id := tb.Insert(&item{Owner: int64(g)})
				tb.Update(id, func(v *item) { v.Name = "x" })
				tb.Select(Opts[item]{Index: "owner", Key: I(int64(g)), Limit: 5, Desc: true})
			}
		}(g)
	}
	wg.Wait()
	if tb.Len() != 400 {
		t.Fatal(tb.Len())
	}
}
