// Package store — встроенное транзакционно-журналируемое хранилище на стандартной библиотеке.
//
// Данные живут в памяти (map + упорядоченный список id + вторичные индексы),
// каждая запись пишется в WAL (JSON-lines), периодически делается снимок.
// Чтение — под RLock без аллокаций на фильтре, запись — под Lock таблицы.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Row — любая запись таблицы.
type Row interface {
	GetID() int64
	SetID(int64)
}

// Base встраивается в каждую модель.
type Base struct {
	ID int64 `json:"id"`
}

func (b *Base) GetID() int64   { return b.ID }
func (b *Base) SetID(id int64) { b.ID = id }

type table interface {
	tname() string
	applyPut(raw json.RawMessage) error
	applyDel(id int64)
	dump(w func(name string, raw []byte) error) error
}

// DB — корневой объект хранилища.
type DB struct {
	dir string

	mu     sync.Mutex // защищает WAL
	wal    *os.File
	walw   *bufio.Writer
	gen    int64
	walLen int64
	dirty  bool

	tmu    sync.RWMutex
	tables map[string]table

	// Lock — глобальная блокировка для составных операций (перевод денег и т.п.)
	Lock sync.Mutex

	stop chan struct{}
	done sync.WaitGroup

	CompactBytes int64
}

type walEntry struct {
	T string          `json:"t"`
	O string          `json:"o"` // p|d
	I int64           `json:"i,omitempty"`
	V json.RawMessage `json:"v,omitempty"`
}

// Open открывает хранилище в каталоге dir. Таблицы нужно создать (NewTable) ДО Load().
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &DB{dir: dir, tables: map[string]table{}, stop: make(chan struct{}), CompactBytes: 64 << 20}, nil
}

// Load загружает снимок и журнал, запускает фоновые задачи. Вызывать один раз после регистрации всех таблиц.
func (db *DB) Load() error {
	snapGen, err := db.latestSnap()
	if err != nil {
		return err
	}
	if snapGen > 0 {
		if err := db.readFile(db.snapPath(snapGen)); err != nil {
			return fmt.Errorf("snapshot: %w", err)
		}
	}
	gens, _ := db.walGens()
	var maxGen int64 = snapGen
	for _, g := range gens {
		if g < snapGen {
			continue
		}
		if err := db.readFile(db.walPath(g)); err != nil {
			return fmt.Errorf("wal %d: %w", g, err)
		}
		if g > maxGen {
			maxGen = g
		}
	}
	if maxGen == 0 {
		maxGen = 1
	}
	db.gen = maxGen
	f, err := os.OpenFile(db.walPath(db.gen), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if st, e := f.Stat(); e == nil {
		db.walLen = st.Size()
	}
	db.wal = f
	db.walw = bufio.NewWriterSize(f, 1<<16)
	db.done.Add(1)
	go db.background()
	return nil
}

func (db *DB) snapPath(g int64) string {
	return filepath.Join(db.dir, fmt.Sprintf("snap-%06d.jsonl", g))
}
func (db *DB) walPath(g int64) string { return filepath.Join(db.dir, fmt.Sprintf("wal-%06d.jsonl", g)) }

func (db *DB) latestSnap() (int64, error) {
	ents, err := os.ReadDir(db.dir)
	if err != nil {
		return 0, err
	}
	var best int64
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "snap-") && strings.HasSuffix(n, ".jsonl") {
			g, err := strconv.ParseInt(n[5:len(n)-6], 10, 64)
			if err == nil && g > best {
				best = g
			}
		}
	}
	return best, nil
}

func (db *DB) walGens() ([]int64, error) {
	ents, err := os.ReadDir(db.dir)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "wal-") && strings.HasSuffix(n, ".jsonl") {
			if g, err := strconv.ParseInt(n[4:len(n)-6], 10, 64); err == nil {
				out = append(out, g)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (db *DB) readFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var e walEntry
			if jerr := json.Unmarshal(line, &e); jerr != nil {
				// оборванная последняя запись — игнорируем
				if err == io.EOF {
					break
				}
				return jerr
			}
			db.tmu.RLock()
			t := db.tables[e.T]
			db.tmu.RUnlock()
			if t != nil {
				if e.O == "d" {
					t.applyDel(e.I)
				} else if perr := t.applyPut(e.V); perr != nil {
					return perr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}

func (db *DB) append(name, op string, id int64, v any) {
	var raw json.RawMessage
	if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		raw = b
	}
	line, _ := json.Marshal(walEntry{T: name, O: op, I: id, V: raw})
	db.mu.Lock()
	if db.walw != nil {
		db.walw.Write(line)
		db.walw.WriteByte('\n')
		db.walLen += int64(len(line)) + 1
		db.dirty = true
	}
	db.mu.Unlock()
}

func (db *DB) background() {
	defer db.done.Done()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-db.stop:
			return
		case <-t.C:
			db.Sync()
			db.mu.Lock()
			big := db.walLen > db.CompactBytes
			db.mu.Unlock()
			if big {
				_ = db.Compact()
			}
		}
	}
}

// Sync сбрасывает буфер журнала на диск.
func (db *DB) Sync() {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.walw != nil && db.dirty {
		db.walw.Flush()
		db.wal.Sync()
		db.dirty = false
	}
}

// Compact делает снимок всех таблиц и ротирует журнал.
func (db *DB) Compact() error {
	db.mu.Lock()
	db.walw.Flush()
	db.wal.Sync()
	oldGen := db.gen
	newGen := oldGen + 1
	nf, err := os.OpenFile(db.walPath(newGen), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		db.mu.Unlock()
		return err
	}
	old := db.wal
	db.wal = nf
	db.walw = bufio.NewWriterSize(nf, 1<<16)
	db.gen = newGen
	db.walLen = 0
	db.mu.Unlock()
	old.Close()

	tmp := db.snapPath(newGen) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	db.tmu.RLock()
	names := make([]string, 0, len(db.tables))
	for n := range db.tables {
		names = append(names, n)
	}
	db.tmu.RUnlock()
	sort.Strings(names)
	for _, n := range names {
		db.tmu.RLock()
		t := db.tables[n]
		db.tmu.RUnlock()
		err = t.dump(func(name string, raw []byte) error {
			line, _ := json.Marshal(walEntry{T: name, O: "p", V: raw})
			w.Write(line)
			return w.WriteByte('\n')
		})
		if err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	f.Sync()
	f.Close()
	if err := os.Rename(tmp, db.snapPath(newGen)); err != nil {
		return err
	}
	// чистим старое
	ents, _ := os.ReadDir(db.dir)
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "snap-") && strings.HasSuffix(n, ".jsonl") {
			if g, _ := strconv.ParseInt(n[5:len(n)-6], 10, 64); g < newGen {
				os.Remove(filepath.Join(db.dir, n))
			}
		}
		if strings.HasPrefix(n, "wal-") && strings.HasSuffix(n, ".jsonl") {
			if g, _ := strconv.ParseInt(n[4:len(n)-6], 10, 64); g < newGen {
				os.Remove(filepath.Join(db.dir, n))
			}
		}
	}
	return nil
}

// Close завершает работу и сбрасывает журнал.
func (db *DB) Close() error {
	select {
	case <-db.stop:
	default:
		close(db.stop)
	}
	db.done.Wait()
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.walw != nil {
		db.walw.Flush()
		db.wal.Sync()
		err := db.wal.Close()
		db.walw = nil
		return err
	}
	return nil
}

// Dir — каталог данных.
func (db *DB) Dir() string { return db.dir }

// ---------------------------------------------------------------- Table

type index[T any] struct {
	name string
	fn   func(*T) string
	m    map[string]map[int64]struct{}
}

// Table — типизированная таблица.
type Table[T any, PT interface {
	*T
	Row
}] struct {
	db   *DB
	name string
	mu   sync.RWMutex
	rows map[int64]*T
	ord  []int64 // порядок вставки (возрастающие id); удалённые чистятся лениво
	dead int
	next int64
	idx  map[string]*index[T]
	// UniqueNext — если false, id назначается автоматически
}

// NewTable регистрирует таблицу в БД.
func NewTable[T any, PT interface {
	*T
	Row
}](db *DB, name string) *Table[T, PT] {
	t := &Table[T, PT]{db: db, name: name, rows: map[int64]*T{}, next: 1, idx: map[string]*index[T]{}}
	db.tmu.Lock()
	if _, ok := db.tables[name]; ok {
		panic("store: duplicate table " + name)
	}
	db.tables[name] = t
	db.tmu.Unlock()
	return t
}

func (t *Table[T, PT]) tname() string { return t.name }

// Index добавляет вторичный индекс. Вызывать до Load().
func (t *Table[T, PT]) Index(name string, fn func(*T) string) *Table[T, PT] {
	t.idx[name] = &index[T]{name: name, fn: fn, m: map[string]map[int64]struct{}{}}
	return t
}

func (t *Table[T, PT]) idxAdd(id int64, v *T) {
	for _, ix := range t.idx {
		k := ix.fn(v)
		s := ix.m[k]
		if s == nil {
			s = map[int64]struct{}{}
			ix.m[k] = s
		}
		s[id] = struct{}{}
	}
}

func (t *Table[T, PT]) idxDel(id int64, v *T) {
	for _, ix := range t.idx {
		k := ix.fn(v)
		if s := ix.m[k]; s != nil {
			delete(s, id)
			if len(s) == 0 {
				delete(ix.m, k)
			}
		}
	}
}

func (t *Table[T, PT]) applyPut(raw json.RawMessage) error {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	id := PT(&v).GetID()
	t.mu.Lock()
	t.putLocked(id, &v)
	t.mu.Unlock()
	return nil
}

func (t *Table[T, PT]) putLocked(id int64, v *T) {
	if old, ok := t.rows[id]; ok {
		t.idxDel(id, old)
	} else {
		t.ord = append(t.ord, id)
		if n := len(t.ord); n > 1 && t.ord[n-2] > id {
			// id вставлен не по порядку (загрузка/ручной id) — восстановим сортировку лениво
			sort.Slice(t.ord, func(i, j int) bool { return t.ord[i] < t.ord[j] })
		}
	}
	t.rows[id] = v
	t.idxAdd(id, v)
	if id >= t.next {
		t.next = id + 1
	}
}

func (t *Table[T, PT]) applyDel(id int64) {
	t.mu.Lock()
	t.delLocked(id)
	t.mu.Unlock()
}

func (t *Table[T, PT]) delLocked(id int64) bool {
	old, ok := t.rows[id]
	if !ok {
		return false
	}
	t.idxDel(id, old)
	delete(t.rows, id)
	t.dead++
	if t.dead > 1024 && t.dead > len(t.ord)/3 {
		n := t.ord[:0]
		for _, x := range t.ord {
			if _, ok := t.rows[x]; ok {
				n = append(n, x)
			}
		}
		t.ord = n
		t.dead = 0
	}
	return true
}

func (t *Table[T, PT]) dump(w func(name string, raw []byte) error) error {
	t.mu.RLock()
	ids := make([]int64, 0, len(t.rows))
	vals := make([]T, 0, len(t.rows))
	for _, id := range t.ord {
		if v, ok := t.rows[id]; ok {
			ids = append(ids, id)
			vals = append(vals, *v)
		}
	}
	t.mu.RUnlock()
	for i := range vals {
		b, err := json.Marshal(&vals[i])
		if err != nil {
			return err
		}
		if err := w(t.name, b); err != nil {
			return err
		}
	}
	return nil
}

// Insert добавляет запись, назначая id (если v.ID == 0). Возвращает id.
func (t *Table[T, PT]) Insert(v *T) int64 {
	t.mu.Lock()
	p := PT(v)
	if p.GetID() == 0 {
		p.SetID(t.next)
	}
	id := p.GetID()
	cp := *v
	t.putLocked(id, &cp)
	t.db.append(t.name, "p", id, &cp)
	t.mu.Unlock()
	return id
}

// Put заменяет (или создаёт) запись по v.ID.
func (t *Table[T, PT]) Put(v T) {
	id := PT(&v).GetID()
	if id == 0 {
		t.Insert(&v)
		return
	}
	t.mu.Lock()
	cp := v
	t.putLocked(id, &cp)
	t.db.append(t.name, "p", id, &cp)
	t.mu.Unlock()
}

// Get возвращает копию записи.
func (t *Table[T, PT]) Get(id int64) (T, bool) {
	t.mu.RLock()
	v, ok := t.rows[id]
	var out T
	if ok {
		out = *v
	}
	t.mu.RUnlock()
	return out, ok
}

// Has — существует ли id.
func (t *Table[T, PT]) Has(id int64) bool {
	t.mu.RLock()
	_, ok := t.rows[id]
	t.mu.RUnlock()
	return ok
}

// Update атомарно изменяет запись. Возвращает копию новой записи и признак существования.
func (t *Table[T, PT]) Update(id int64, fn func(*T)) (T, bool) {
	t.mu.Lock()
	v, ok := t.rows[id]
	if !ok {
		t.mu.Unlock()
		var z T
		return z, false
	}
	t.idxDel(id, v)
	fn(v)
	PT(v).SetID(id)
	t.idxAdd(id, v)
	cp := *v
	t.db.append(t.name, "p", id, &cp)
	t.mu.Unlock()
	return cp, true
}

// Delete удаляет запись.
func (t *Table[T, PT]) Delete(id int64) bool {
	t.mu.Lock()
	ok := t.delLocked(id)
	if ok {
		t.db.append(t.name, "d", id, nil)
	}
	t.mu.Unlock()
	return ok
}

// DeleteWhere удаляет все записи по предикату, возвращает число. Если idx != "", кандидаты берутся из индекса.
func (t *Table[T, PT]) DeleteWhere(pred func(*T) bool) int {
	t.mu.Lock()
	var ids []int64
	for _, id := range t.ord {
		if v, ok := t.rows[id]; ok && pred(v) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		t.delLocked(id)
		t.db.append(t.name, "d", id, nil)
	}
	t.mu.Unlock()
	return len(ids)
}

// DeleteBy удаляет все записи с ключом key в индексе idx.
func (t *Table[T, PT]) DeleteBy(idx, key string) int {
	t.mu.Lock()
	ix := t.idx[idx]
	if ix == nil {
		t.mu.Unlock()
		panic("store: no index " + idx)
	}
	ids := make([]int64, 0, len(ix.m[key]))
	for id := range ix.m[key] {
		ids = append(ids, id)
	}
	for _, id := range ids {
		t.delLocked(id)
		t.db.append(t.name, "d", id, nil)
	}
	t.mu.Unlock()
	return len(ids)
}

// UpdateWhere изменяет все записи, удовлетворяющие предикату.
func (t *Table[T, PT]) UpdateWhere(pred func(*T) bool, fn func(*T)) int {
	t.mu.Lock()
	var out []T
	for _, id := range t.ord {
		if v, ok := t.rows[id]; ok && pred(v) {
			t.idxDel(id, v)
			fn(v)
			PT(v).SetID(id)
			t.idxAdd(id, v)
			out = append(out, *v)
		}
	}
	for i := range out {
		t.db.append(t.name, "p", PT(&out[i]).GetID(), &out[i])
	}
	t.mu.Unlock()
	return len(out)
}

// Len — число записей.
func (t *Table[T, PT]) Len() int {
	t.mu.RLock()
	n := len(t.rows)
	t.mu.RUnlock()
	return n
}

// Opts — параметры выборки.
type Opts[T any] struct {
	Index string // имя индекса (опционально)
	Key   string // значение ключа индекса
	Where func(*T) bool
	// Less — порядок «a раньше b». Если nil — порядок по id (Desc — по убыванию).
	Less   func(a, b *T) bool
	Desc   bool // обратный порядок (для Less — инвертирует его)
	Offset int
	Limit  int // 0 — без ограничения
}

// Select возвращает страницу записей и общее число подходящих (до Offset/Limit).
func (t *Table[T, PT]) Select(o Opts[T]) ([]T, int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var cand []*T
	if o.Index != "" {
		ix := t.idx[o.Index]
		if ix == nil {
			panic("store: no index " + o.Index)
		}
		set := ix.m[o.Key]
		ids := make([]int64, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		cand = make([]*T, 0, len(ids))
		for _, id := range ids {
			v := t.rows[id]
			if o.Where == nil || o.Where(v) {
				cand = append(cand, v)
			}
		}
	} else if o.Where == nil && o.Less == nil {
		// быстрый путь: только порядок по id и пагинация
		return t.fastPage(o)
	} else {
		cand = make([]*T, 0, 64)
		for _, id := range t.ord {
			if v, ok := t.rows[id]; ok && (o.Where == nil || o.Where(v)) {
				cand = append(cand, v)
			}
		}
	}
	total := len(cand)
	if o.Less != nil {
		less := o.Less
		if o.Desc {
			sort.SliceStable(cand, func(i, j int) bool { return less(cand[j], cand[i]) })
		} else {
			sort.SliceStable(cand, func(i, j int) bool { return less(cand[i], cand[j]) })
		}
	} else if o.Desc {
		for i, j := 0, len(cand)-1; i < j; i, j = i+1, j-1 {
			cand[i], cand[j] = cand[j], cand[i]
		}
	}
	return slice(cand, o.Offset, o.Limit), total
}

func (t *Table[T, PT]) fastPage(o Opts[T]) ([]T, int) {
	total := len(t.rows)
	if o.Offset < 0 {
		o.Offset = 0
	}
	lim := o.Limit
	if lim <= 0 || lim > total {
		lim = total
	}
	out := make([]T, 0, lim)
	skip := o.Offset
	if o.Desc {
		for i := len(t.ord) - 1; i >= 0 && len(out) < lim; i-- {
			if v, ok := t.rows[t.ord[i]]; ok {
				if skip > 0 {
					skip--
					continue
				}
				out = append(out, *v)
			}
		}
	} else {
		for _, id := range t.ord {
			if len(out) >= lim {
				break
			}
			if v, ok := t.rows[id]; ok {
				if skip > 0 {
					skip--
					continue
				}
				out = append(out, *v)
			}
		}
	}
	return out, total
}

func slice[T any](c []*T, off, lim int) []T {
	if off < 0 {
		off = 0
	}
	if off > len(c) {
		off = len(c)
	}
	end := len(c)
	if lim > 0 && off+lim < end {
		end = off + lim
	}
	out := make([]T, 0, end-off)
	for _, v := range c[off:end] {
		out = append(out, *v)
	}
	return out
}

// All возвращает все записи в порядке id.
func (t *Table[T, PT]) All() []T {
	r, _ := t.Select(Opts[T]{})
	return r
}

// Find — записи по предикату в порядке id.
func (t *Table[T, PT]) Find(pred func(*T) bool) []T {
	r, _ := t.Select(Opts[T]{Where: pred})
	return r
}

// FindOne — первая запись по предикату.
func (t *Table[T, PT]) FindOne(pred func(*T) bool) (T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, id := range t.ord {
		if v, ok := t.rows[id]; ok && pred(v) {
			return *v, true
		}
	}
	var z T
	return z, false
}

// By — все записи с ключом key в индексе idx, по возрастанию id.
func (t *Table[T, PT]) By(idx, key string) []T {
	r, _ := t.Select(Opts[T]{Index: idx, Key: key})
	return r
}

// OneBy — первая запись по индексу.
func (t *Table[T, PT]) OneBy(idx, key string) (T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ix := t.idx[idx]
	if ix == nil {
		panic("store: no index " + idx)
	}
	var best int64 = -1
	for id := range ix.m[key] {
		if best == -1 || id < best {
			best = id
		}
	}
	if best == -1 {
		var z T
		return z, false
	}
	return *t.rows[best], true
}

// CountBy — число записей с ключом в индексе.
func (t *Table[T, PT]) CountBy(idx, key string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ix := t.idx[idx]
	if ix == nil {
		panic("store: no index " + idx)
	}
	return len(ix.m[key])
}

// Count — число записей по предикату (nil — все).
func (t *Table[T, PT]) Count(pred func(*T) bool) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if pred == nil {
		return len(t.rows)
	}
	n := 0
	for _, v := range t.rows {
		if pred(v) {
			n++
		}
	}
	return n
}

// Scan обходит записи в порядке id; fn возвращает false для остановки. Запись нельзя изменять и сохранять указатель.
func (t *Table[T, PT]) Scan(fn func(*T) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, id := range t.ord {
		if v, ok := t.rows[id]; ok {
			if !fn(v) {
				return
			}
		}
	}
}

// Max возвращает максимальный id.
func (t *Table[T, PT]) MaxID() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.next - 1
}

// Key — вспомогательная функция для составных ключей индексов.
func Key(parts ...any) string {
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteByte('|')
		}
		switch x := p.(type) {
		case int64:
			sb.WriteString(strconv.FormatInt(x, 10))
		case int:
			sb.WriteString(strconv.Itoa(x))
		case string:
			sb.WriteString(x)
		case bool:
			if x {
				sb.WriteByte('1')
			} else {
				sb.WriteByte('0')
			}
		default:
			fmt.Fprint(&sb, x)
		}
	}
	return sb.String()
}

// I — int64 → ключ индекса.
func I(n int64) string { return strconv.FormatInt(n, 10) }
