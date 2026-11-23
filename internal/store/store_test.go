package store

import (
	"os"
	"sync"
	"testing"
)

type item struct {
	Base
	Owner int64  `json:"owner"`
	Name  string `json:"name"`
}

func open(t *testing.T, dir string) (*DB, *Table[item, *item]) {
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	tb := NewTable[item](db, "item").Index("owner", func(v *item) string { return I(v.Owner) })
	if err := db.Load(); err != nil {
		t.Fatal(err)
	}
	return db, tb
}

func TestPersistAndQuery(t *testing.T) {
	dir := t.TempDir()
	db, tb := open(t, dir)
	for i := 0; i < 100; i++ {
		tb.Insert(&item{Owner: int64(i % 3), Name: "n"})
	}
	tb.Update(5, func(v *item) { v.Owner = 9 })
	tb.Delete(7)
	if tb.CountBy("owner", "9") != 1 || tb.Len() != 99 {
		t.Fatal("counts")
	}
	rows, total := tb.Select(Opts[item]{Desc: true, Limit: 3})
	if total != 99 || rows[0].ID != 100 {
		t.Fatalf("desc %v %d", rows, total)
	}
	db.Compact()
	tb.Insert(&item{Owner: 1, Name: "after"})
	db.Close()
	db2, tb2 := open(t, dir)
	defer db2.Close()
	if tb2.Len() != 100 {
		t.Fatalf("len %d", tb2.Len())
	}
	if v, _ := tb2.Get(5); v.Owner != 9 {
		t.Fatal("update lost")
	}
	if tb2.Has(7) {
		t.Fatal("delete lost")
	}
	if id := tb2.Insert(&item{}); id != 102 {
		t.Fatalf("next id %d", id)
	}
	_ = os.RemoveAll(dir)
}

func TestConcurrent(t *testing.T) {
	db, tb := open(t, t.TempDir())
	defer db.Close()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				id := tb.Insert(&item{Owner: int64(g)})
				tb.Update(id, func(v *item) { v.Name = "x" })
				tb.Select(Opts[item]{Index: "owner", Key: I(int64(g)), Limit: 5, Desc: true})
			}
		}(g)
	}
	wg.Wait()
	if tb.Len() != 4000 {
		t.Fatal(tb.Len())
	}
}
