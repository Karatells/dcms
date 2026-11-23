// Package store — хранилище данных на базе MySQL / MariaDB (database/sql + go-sql-driver/mysql).
//
// Каждая зарегистрированная таблица Table[T, PT] соответствует реальной таблице InnoDB (utf8mb4)
// с типизированными колонками по полям структуры T и B-tree индексами MySQL для каждого Index().
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-sql-driver/mysql"
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

type tableMeta interface {
	tname() string
	ensureSchema() error
}

// DBStats — статистика подключённой базы данных MySQL / MariaDB.
type DBStats struct {
	Version   string
	SizeBytes int64
	Tables    int
}

// DB — корневой объект хранилища MySQL / MariaDB.
type DB struct {
	sql *sql.DB
	dsn string

	tmu    sync.RWMutex
	tables map[string]tableMeta

	// Lock — глобальная блокировка для составных операций (перевод денег и т.п.)
	Lock sync.Mutex
}

// NormalizeDSN гарантирует utf8mb4 и корректные параметры подключения для MySQL / MariaDB.
func NormalizeDSN(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("store: неверный DSN MySQL: %w", err)
	}
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}
	if cfg.Collation == "" {
		cfg.Collation = "utf8mb4_unicode_ci"
	}
	cfg.ParseTime = false
	cfg.MultiStatements = false
	return cfg.FormatDSN(), nil
}

// Open открывает пул подключений к MySQL / MariaDB по строке DSN.
// Таблицы нужно создать (NewTable) ДО Load().
func Open(dsn string) (*DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("store: не указан DSN подключения к MySQL/MariaDB (DCMS_DSN)")
	}
	norm, err := NormalizeDSN(dsn)
	if err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("mysql", norm)
	if err != nil {
		return nil, fmt.Errorf("store: sql.Open: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(2 * time.Minute)
	return &DB{
		sql:    sqlDB,
		dsn:    norm,
		tables: map[string]tableMeta{},
	}, nil
}

// SQL возвращает базовый *sql.DB.
func (db *DB) SQL() *sql.DB { return db.sql }

// Load проверяет соединение с MySQL/MariaDB и создаёт/обновляет схемы всех зарегистрированных таблиц.
// Вызывать один раз после регистрации всех таблиц.
func (db *DB) Load() error {
	if err := db.sql.Ping(); err != nil {
		return fmt.Errorf("store: не удалось подключиться к MySQL/MariaDB: %w", err)
	}
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
		if err := t.ensureSchema(); err != nil {
			return fmt.Errorf("store: миграция таблицы %s: %w", n, err)
		}
	}
	return nil
}

// Close закрывает пул подключений к MySQL/MariaDB.
func (db *DB) Close() error {
	if db.sql != nil {
		return db.sql.Close()
	}
	return nil
}

// Stats возвращает версию СУБД, размер текущей базы данных и количество таблиц.
func (db *DB) Stats() (DBStats, error) {
	var st DBStats
	if err := db.sql.QueryRow("SELECT VERSION()").Scan(&st.Version); err != nil {
		return st, err
	}
	err := db.sql.QueryRow(`
		SELECT COALESCE(SUM(data_length + index_length), 0), COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
	`).Scan(&st.SizeBytes, &st.Tables)
	return st, err
}

// DumpSQL записывает полный SQL-дамп всех зарегистрированных таблиц в w.
func (db *DB) DumpSQL(w io.Writer) error {
	db.tmu.RLock()
	names := make([]string, 0, len(db.tables))
	for n := range db.tables {
		names = append(names, n)
	}
	db.tmu.RUnlock()
	sort.Strings(names)

	if _, err := fmt.Fprintf(w, "-- DCMS Social MySQL/MariaDB Dump\n-- Generated at %s\n\nSET NAMES utf8mb4;\nSET FOREIGN_KEY_CHECKS=0;\n\n",
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	for _, name := range names {
		var tname, createSQL string
		if err := db.sql.QueryRow("SHOW CREATE TABLE " + quoteIdent(name)).Scan(&tname, &createSQL); err != nil {
			return fmt.Errorf("show create table %s: %w", name, err)
		}
		if _, err := fmt.Fprintf(w, "DROP TABLE IF EXISTS %s;\n%s;\n\n", quoteIdent(name), createSQL); err != nil {
			return err
		}

		rows, err := db.sql.Query("SELECT * FROM " + quoteIdent(name) + " ORDER BY `id` ASC")
		if err != nil {
			return fmt.Errorf("dump select %s: %w", name, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		quotedCols := make([]string, len(cols))
		for i, c := range cols {
			quotedCols[i] = quoteIdent(c)
		}
		colList := strings.Join(quotedCols, ", ")

		rawVals := make([]sql.RawBytes, len(cols))
		scanArgs := make([]any, len(cols))
		for i := range rawVals {
			scanArgs[i] = &rawVals[i]
		}
		colTypes, _ := rows.ColumnTypes()

		batch := 0
		for rows.Next() {
			if err := rows.Scan(scanArgs...); err != nil {
				rows.Close()
				return err
			}
			if batch == 0 {
				if _, err := fmt.Fprintf(w, "INSERT INTO %s (%s) VALUES\n", quoteIdent(name), colList); err != nil {
					rows.Close()
					return err
				}
			} else {
				if _, err := io.WriteString(w, ",\n"); err != nil {
					rows.Close()
					return err
				}
			}
			if _, err := io.WriteString(w, "("); err != nil {
				rows.Close()
				return err
			}
			for i, b := range rawVals {
				if i > 0 {
					if _, err := io.WriteString(w, ","); err != nil {
						rows.Close()
						return err
					}
				}
				if b == nil {
					_, err = io.WriteString(w, "NULL")
				} else if isNumericColType(colTypes, i) {
					_, err = w.Write(b)
				} else {
					_, err = io.WriteString(w, quoteSQLString(string(b)))
				}
				if err != nil {
					rows.Close()
					return err
				}
			}
			if _, err := io.WriteString(w, ")"); err != nil {
				rows.Close()
				return err
			}
			batch++
			if batch >= 100 {
				if _, err := io.WriteString(w, ";\n"); err != nil {
					rows.Close()
					return err
				}
				batch = 0
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if batch > 0 {
			if _, err := io.WriteString(w, ";\n"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "SET FOREIGN_KEY_CHECKS=1;\n")
	return err
}

func isNumericColType(types []*sql.ColumnType, i int) bool {
	if i >= len(types) {
		return false
	}
	t := strings.ToUpper(types[i].DatabaseTypeName())
	return strings.Contains(t, "INT") || strings.Contains(t, "DOUBLE") || strings.Contains(t, "FLOAT") || strings.Contains(t, "DECIMAL")
}

func quoteSQLString(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + 2)
	sb.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			sb.WriteString(`\\`)
		case '\'':
			sb.WriteString(`\'`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case 0:
			sb.WriteString(`\0`)
		case 0x1a:
			sb.WriteString(`\Z`)
		default:
			sb.WriteByte(s[i])
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}

// ---------------------------------------------------------------- Table

type colInfo struct {
	name    string
	index   []int
	kind    reflect.Kind
	sqlType string
}

type indexDef[T any] struct {
	name    string
	colName string
	idxName string
	fn      func(*T) string
}

// Table — типизированная таблица MySQL / MariaDB.
type Table[T any, PT interface {
	*T
	Row
}] struct {
	db      *DB
	name    string
	qname   string
	cols    []colInfo // все колонки кроме id
	idxList []*indexDef[T]
	idxMap  map[string]*indexDef[T]

	// предсобранные фрагменты SQL
	selectCols string // `id`, `col1`, `col2`, ...
}

// NewTable регистрирует таблицу в БД.
func NewTable[T any, PT interface {
	*T
	Row
}](db *DB, name string) *Table[T, PT] {
	cols := inspectStruct[T]()
	t := &Table[T, PT]{
		db:     db,
		name:   name,
		qname:  quoteIdent(name),
		cols:   cols,
		idxMap: map[string]*indexDef[T]{},
	}
	t.rebuildSQL()
	db.tmu.Lock()
	if _, ok := db.tables[name]; ok {
		db.tmu.Unlock()
		panic("store: duplicate table " + name)
	}
	db.tables[name] = t
	db.tmu.Unlock()
	_ = t.ensureSchema()
	return t
}

func (t *Table[T, PT]) tname() string { return t.name }

// Index добавляет вторичный индекс MySQL. Вызывать до Load().
func (t *Table[T, PT]) Index(name string, fn func(*T) string) *Table[T, PT] {
	if _, exists := t.idxMap[name]; exists {
		panic("store: duplicate index " + name + " on " + t.name)
	}
	ix := &indexDef[T]{
		name:    name,
		colName: "_idx_" + name,
		idxName: "idx_" + name,
		fn:      fn,
	}
	t.idxList = append(t.idxList, ix)
	t.idxMap[name] = ix
	return t
}

func (t *Table[T, PT]) rebuildSQL() {
	parts := make([]string, 0, len(t.cols)+1)
	parts = append(parts, "`id`")
	for _, c := range t.cols {
		parts = append(parts, quoteIdent(c.name))
	}
	t.selectCols = strings.Join(parts, ", ")
}

func (t *Table[T, PT]) ensureSchema() error {
	var defs []string
	defs = append(defs, "`id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY")
	for _, c := range t.cols {
		defs = append(defs, quoteIdent(c.name)+" "+c.sqlType)
	}
	for _, ix := range t.idxList {
		defs = append(defs, quoteIdent(ix.colName)+" VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT ''")
	}
	for _, ix := range t.idxList {
		defs = append(defs, fmt.Sprintf("INDEX %s (%s)", quoteIdent(ix.idxName), quoteIdent(ix.colName)))
	}
	ddl := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
		t.qname, strings.Join(defs, ",\n  "))
	if _, err := t.db.sql.Exec(ddl); err != nil {
		return err
	}

	// Дополняем отсутствующие колонки (эволюция схемы)
	existingCols := map[string]bool{}
	rows, err := t.db.sql.Query(`
		SELECT COLUMN_NAME FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
	`, t.name)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cn string
		if err := rows.Scan(&cn); err == nil {
			existingCols[strings.ToLower(cn)] = true
		}
	}
	rows.Close()

	for _, c := range t.cols {
		if !existingCols[strings.ToLower(c.name)] {
			q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", t.qname, quoteIdent(c.name), c.sqlType)
			if _, err := t.db.sql.Exec(q); err != nil {
				return err
			}
		}
	}
	var addedIdxCols []*indexDef[T]
	for _, ix := range t.idxList {
		if !existingCols[strings.ToLower(ix.colName)] {
			q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT ''",
				t.qname, quoteIdent(ix.colName))
			if _, err := t.db.sql.Exec(q); err != nil {
				return err
			}
			addedIdxCols = append(addedIdxCols, ix)
		}
	}

	// Дополняем отсутствующие индексы
	existingIdx := map[string]bool{}
	irows, err := t.db.sql.Query(`
		SELECT INDEX_NAME FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ?
	`, t.name)
	if err != nil {
		return err
	}
	for irows.Next() {
		var in string
		if err := irows.Scan(&in); err == nil {
			existingIdx[strings.ToLower(in)] = true
		}
	}
	irows.Close()

	for _, ix := range t.idxList {
		if !existingIdx[strings.ToLower(ix.idxName)] {
			q := fmt.Sprintf("CREATE INDEX %s ON %s (%s)", quoteIdent(ix.idxName), t.qname, quoteIdent(ix.colName))
			if _, err := t.db.sql.Exec(q); err != nil {
				return err
			}
		}
	}

	// Если добавились новые индексные колонки в уже существующей таблице — заполняем их
	if len(addedIdxCols) > 0 {
		all := t.All()
		for i := range all {
			t.Put(all[i])
		}
	}
	return nil
}

func inspectStruct[T any]() []colInfo {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	if rt.Kind() != reflect.Struct {
		panic("store: T must be struct")
	}
	var cols []colInfo
	seen := map[string]bool{"id": true}
	var walk func(t reflect.Type, prefix []int)
	walk = func(t reflect.Type, prefix []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			idx := append(append([]int(nil), prefix...), i)
			if f.Anonymous && f.Type == reflect.TypeOf(Base{}) {
				continue
			}
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type, idx)
				continue
			}
			col := fieldColName(f)
			if seen[col] {
				panic("store: duplicate column " + col + " in " + rt.Name())
			}
			seen[col] = true
			sqlType := kindToSQLType(f.Type.Kind())
			cols = append(cols, colInfo{
				name:    col,
				index:   idx,
				kind:    f.Type.Kind(),
				sqlType: sqlType,
			})
		}
	}
	walk(rt, nil)
	return cols
}

func fieldColName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag != "" {
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			return tag
		}
	}
	return toSnakeCase(f.Name)
}

func toSnakeCase(s string) string {
	rs := []rune(s)
	var sb strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rs[i-1]
				nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
					sb.WriteByte('_')
				}
			}
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func kindToSQLType(k reflect.Kind) string {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT NOT NULL DEFAULT 0"
	case reflect.Bool:
		return "TINYINT(1) NOT NULL DEFAULT 0"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE NOT NULL DEFAULT 0"
	case reflect.String:
		return "MEDIUMTEXT"
	default:
		panic("store: unsupported field kind " + k.String())
	}
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func normIdxKey(k string) string {
	if len(k) <= 191 {
		return k
	}
	sum := sha256.Sum256([]byte(k))
	return "#h:" + hex.EncodeToString(sum[:])
}

func (t *Table[T, PT]) fieldValues(v *T) []any {
	rv := reflect.ValueOf(v).Elem()
	vals := make([]any, 0, len(t.cols)+len(t.idxList))
	for _, c := range t.cols {
		fv := rv.FieldByIndex(c.index)
		switch c.kind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			vals = append(vals, fv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			vals = append(vals, int64(fv.Uint()))
		case reflect.Bool:
			if fv.Bool() {
				vals = append(vals, int8(1))
			} else {
				vals = append(vals, int8(0))
			}
		case reflect.Float32, reflect.Float64:
			vals = append(vals, fv.Float())
		case reflect.String:
			vals = append(vals, fv.String())
		}
	}
	for _, ix := range t.idxList {
		vals = append(vals, normIdxKey(ix.fn(v)))
	}
	return vals
}

func (t *Table[T, PT]) scanRow(sc interface{ Scan(...any) error }) (T, error) {
	var v T
	rv := reflect.ValueOf(&v).Elem()
	var id int64
	dest := make([]any, 0, len(t.cols)+1)
	dest = append(dest, &id)

	// буферы для nullable string и bool/int
	type fixup struct {
		fv   reflect.Value
		kind reflect.Kind
		ns   *sql.NullString
		ni   *sql.NullInt64
		nf   *sql.NullFloat64
	}
	fixups := make([]fixup, len(t.cols))
	for i, c := range t.cols {
		fv := rv.FieldByIndex(c.index)
		switch c.kind {
		case reflect.String:
			var ns sql.NullString
			fixups[i] = fixup{fv: fv, kind: c.kind, ns: &ns}
			dest = append(dest, &ns)
		case reflect.Float32, reflect.Float64:
			var nf sql.NullFloat64
			fixups[i] = fixup{fv: fv, kind: c.kind, nf: &nf}
			dest = append(dest, &nf)
		default:
			var ni sql.NullInt64
			fixups[i] = fixup{fv: fv, kind: c.kind, ni: &ni}
			dest = append(dest, &ni)
		}
	}
	if err := sc.Scan(dest...); err != nil {
		return v, err
	}
	PT(&v).SetID(id)
	for _, fx := range fixups {
		switch fx.kind {
		case reflect.String:
			if fx.ns.Valid {
				fx.fv.SetString(fx.ns.String)
			}
		case reflect.Bool:
			fx.fv.SetBool(fx.ni.Valid && fx.ni.Int64 != 0)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if fx.ni.Valid {
				fx.fv.SetInt(fx.ni.Int64)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if fx.ni.Valid {
				fx.fv.SetUint(uint64(fx.ni.Int64))
			}
		case reflect.Float32, reflect.Float64:
			if fx.nf.Valid {
				fx.fv.SetFloat(fx.nf.Float64)
			}
		}
	}
	return v, nil
}

func (t *Table[T, PT]) allWriteCols() []string {
	out := make([]string, 0, len(t.cols)+len(t.idxList))
	for _, c := range t.cols {
		out = append(out, quoteIdent(c.name))
	}
	for _, ix := range t.idxList {
		out = append(out, quoteIdent(ix.colName))
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// Insert добавляет запись, назначая id (если v.ID == 0). Возвращает id.
func (t *Table[T, PT]) Insert(v *T) int64 {
	p := PT(v)
	id := p.GetID()
	wcols := t.allWriteCols()
	vals := t.fieldValues(v)

	if id == 0 {
		var q string
		if len(wcols) == 0 {
			q = fmt.Sprintf("INSERT INTO %s () VALUES ()", t.qname)
		} else {
			q = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", t.qname, strings.Join(wcols, ", "), placeholders(len(wcols)))
		}
		res, err := t.db.sql.Exec(q, vals...)
		if err != nil {
			log.Printf("store %s.Insert: %v", t.name, err)
			return 0
		}
		newID, err := res.LastInsertId()
		if err != nil {
			log.Printf("store %s.Insert LastInsertId: %v", t.name, err)
			return 0
		}
		p.SetID(newID)
		return newID
	}

	// Явный id: INSERT ... ON DUPLICATE KEY UPDATE
	allCols := append([]string{"`id`"}, wcols...)
	allVals := append([]any{id}, vals...)
	var q string
	if len(wcols) == 0 {
		q = fmt.Sprintf("INSERT IGNORE INTO %s (`id`) VALUES (?)", t.qname)
	} else {
		upd := make([]string, len(wcols))
		for i, c := range wcols {
			upd[i] = c + "=VALUES(" + c + ")"
		}
		q = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
			t.qname, strings.Join(allCols, ", "), placeholders(len(allCols)), strings.Join(upd, ", "))
	}
	if _, err := t.db.sql.Exec(q, allVals...); err != nil {
		log.Printf("store %s.Insert(id=%d): %v", t.name, id, err)
	}
	return id
}

// Put заменяет (или создаёт) запись по v.ID.
func (t *Table[T, PT]) Put(v T) {
	t.Insert(&v)
}

// Get возвращает копию записи.
func (t *Table[T, PT]) Get(id int64) (T, bool) {
	q := fmt.Sprintf("SELECT %s FROM %s WHERE `id` = ? LIMIT 1", t.selectCols, t.qname)
	v, err := t.scanRow(t.db.sql.QueryRow(q, id))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("store %s.Get(%d): %v", t.name, id, err)
		}
		var z T
		return z, false
	}
	return v, true
}

// Has — существует ли id.
func (t *Table[T, PT]) Has(id int64) bool {
	var one int
	q := fmt.Sprintf("SELECT 1 FROM %s WHERE `id` = ? LIMIT 1", t.qname)
	err := t.db.sql.QueryRow(q, id).Scan(&one)
	return err == nil
}

func isDeadlock(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1205)
}

// Update атомарно изменяет запись в транзакции (SELECT ... FOR UPDATE).
// Возвращает копию новой записи и признак существования.
func (t *Table[T, PT]) Update(id int64, fn func(*T)) (T, bool) {
	for attempt := 0; attempt < 3; attempt++ {
		out, ok, err := t.updateOnce(id, fn)
		if err == nil {
			return out, ok
		}
		if isDeadlock(err) && attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
			continue
		}
		log.Printf("store %s.Update(%d): %v", t.name, id, err)
		var z T
		return z, false
	}
	var z T
	return z, false
}

func (t *Table[T, PT]) updateOnce(id int64, fn func(*T)) (T, bool, error) {
	var z T
	tx, err := t.db.sql.Begin()
	if err != nil {
		return z, false, err
	}
	defer tx.Rollback()

	q := fmt.Sprintf("SELECT %s FROM %s WHERE `id` = ? FOR UPDATE", t.selectCols, t.qname)
	v, err := t.scanRow(tx.QueryRow(q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return z, false, nil
		}
		return z, false, err
	}
	fn(&v)
	PT(&v).SetID(id)

	wcols := t.allWriteCols()
	if len(wcols) > 0 {
		sets := make([]string, len(wcols))
		for i, c := range wcols {
			sets[i] = c + "=?"
		}
		vals := append(t.fieldValues(&v), id)
		uq := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ?", t.qname, strings.Join(sets, ", "))
		if _, err := tx.Exec(uq, vals...); err != nil {
			return z, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return z, false, err
	}
	return v, true, nil
}

// Delete удаляет запись.
func (t *Table[T, PT]) Delete(id int64) bool {
	q := fmt.Sprintf("DELETE FROM %s WHERE `id` = ?", t.qname)
	res, err := t.db.sql.Exec(q, id)
	if err != nil {
		log.Printf("store %s.Delete(%d): %v", t.name, id, err)
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// DeleteWhere удаляет все записи по предикату, возвращает число удалённых.
func (t *Table[T, PT]) DeleteWhere(pred func(*T) bool) int {
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY `id` ASC", t.selectCols, t.qname)
	rows, err := t.db.sql.Query(q)
	if err != nil {
		log.Printf("store %s.DeleteWhere: %v", t.name, err)
		return 0
	}
	var ids []int64
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err != nil {
			continue
		}
		if pred(&v) {
			ids = append(ids, PT(&v).GetID())
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return 0
	}
	deleted := 0
	for i := 0; i < len(ids); i += 500 {
		end := i + 500
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}
		dq := fmt.Sprintf("DELETE FROM %s WHERE `id` IN (%s)", t.qname, placeholders(len(chunk)))
		res, err := t.db.sql.Exec(dq, args...)
		if err != nil {
			log.Printf("store %s.DeleteWhere exec: %v", t.name, err)
			continue
		}
		if n, err := res.RowsAffected(); err == nil {
			deleted += int(n)
		}
	}
	return deleted
}

// DeleteBy удаляет все записи с ключом key в индексе idx.
func (t *Table[T, PT]) DeleteBy(idx, key string) int {
	ix := t.idxMap[idx]
	if ix == nil {
		panic("store: no index " + idx + " on " + t.name)
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t.qname, quoteIdent(ix.colName))
	res, err := t.db.sql.Exec(q, normIdxKey(key))
	if err != nil {
		log.Printf("store %s.DeleteBy(%s): %v", t.name, idx, err)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// UpdateWhere изменяет все записи, удовлетворяющие предикату.
func (t *Table[T, PT]) UpdateWhere(pred func(*T) bool, fn func(*T)) int {
	tx, err := t.db.sql.Begin()
	if err != nil {
		log.Printf("store %s.UpdateWhere begin: %v", t.name, err)
		return 0
	}
	defer tx.Rollback()

	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY `id` ASC FOR UPDATE", t.selectCols, t.qname)
	rows, err := tx.Query(q)
	if err != nil {
		log.Printf("store %s.UpdateWhere select: %v", t.name, err)
		return 0
	}
	var updated []T
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err != nil {
			continue
		}
		if pred(&v) {
			id := PT(&v).GetID()
			fn(&v)
			PT(&v).SetID(id)
			updated = append(updated, v)
		}
	}
	rows.Close()

	wcols := t.allWriteCols()
	if len(updated) > 0 && len(wcols) > 0 {
		sets := make([]string, len(wcols))
		for i, c := range wcols {
			sets[i] = c + "=?"
		}
		uq := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ?", t.qname, strings.Join(sets, ", "))
		stmt, err := tx.Prepare(uq)
		if err != nil {
			log.Printf("store %s.UpdateWhere prepare: %v", t.name, err)
			return 0
		}
		for i := range updated {
			vals := append(t.fieldValues(&updated[i]), PT(&updated[i]).GetID())
			if _, err := stmt.Exec(vals...); err != nil {
				stmt.Close()
				log.Printf("store %s.UpdateWhere exec: %v", t.name, err)
				return 0
			}
		}
		stmt.Close()
	}
	if err := tx.Commit(); err != nil {
		log.Printf("store %s.UpdateWhere commit: %v", t.name, err)
		return 0
	}
	return len(updated)
}

// Len — число записей.
func (t *Table[T, PT]) Len() int {
	var n int
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s", t.qname)
	if err := t.db.sql.QueryRow(q).Scan(&n); err != nil {
		log.Printf("store %s.Len: %v", t.name, err)
		return 0
	}
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
	var whereSQL string
	var args []any
	if o.Index != "" {
		ix := t.idxMap[o.Index]
		if ix == nil {
			panic("store: no index " + o.Index + " on " + t.name)
		}
		whereSQL = " WHERE " + quoteIdent(ix.colName) + " = ?"
		args = append(args, normIdxKey(o.Key))
	}

	// Быстрый SQL-путь без Go-предиката и кастомной сортировки
	if o.Where == nil && o.Less == nil {
		var total int
		cq := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", t.qname, whereSQL)
		if err := t.db.sql.QueryRow(cq, args...).Scan(&total); err != nil {
			log.Printf("store %s.Select count: %v", t.name, err)
			return []T{}, 0
		}
		if total == 0 {
			return []T{}, 0
		}
		dir := "ASC"
		if o.Desc {
			dir = "DESC"
		}
		off := o.Offset
		if off < 0 {
			off = 0
		}
		if off >= total {
			return []T{}, total
		}
		q := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY `id` %s", t.selectCols, t.qname, whereSQL, dir)
		qArgs := append([]any(nil), args...)
		if o.Limit > 0 {
			q += " LIMIT ? OFFSET ?"
			qArgs = append(qArgs, o.Limit, off)
		} else if off > 0 {
			q += " LIMIT 18446744073709551615 OFFSET ?"
			qArgs = append(qArgs, off)
		}
		rows, err := t.db.sql.Query(q, qArgs...)
		if err != nil {
			log.Printf("store %s.Select query: %v", t.name, err)
			return []T{}, total
		}
		defer rows.Close()
		out := make([]T, 0, 16)
		for rows.Next() {
			v, err := t.scanRow(rows)
			if err == nil {
				out = append(out, v)
			}
		}
		return out, total
	}

	// Выборка кандидатов из MySQL (по индексу, если задан) + фильтрация/сортировка
	q := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY `id` ASC", t.selectCols, t.qname, whereSQL)
	rows, err := t.db.sql.Query(q, args...)
	if err != nil {
		log.Printf("store %s.Select: %v", t.name, err)
		return []T{}, 0
	}
	var cand []T
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err != nil {
			continue
		}
		if o.Where == nil || o.Where(&v) {
			cand = append(cand, v)
		}
	}
	rows.Close()

	total := len(cand)
	if o.Less != nil {
		less := o.Less
		if o.Desc {
			sort.SliceStable(cand, func(i, j int) bool { return less(&cand[j], &cand[i]) })
		} else {
			sort.SliceStable(cand, func(i, j int) bool { return less(&cand[i], &cand[j]) })
		}
	} else if o.Desc {
		for i, j := 0, len(cand)-1; i < j; i, j = i+1, j-1 {
			cand[i], cand[j] = cand[j], cand[i]
		}
	}
	return sliceVals(cand, o.Offset, o.Limit), total
}

func sliceVals[T any](c []T, off, lim int) []T {
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
	out := make([]T, end-off)
	copy(out, c[off:end])
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
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY `id` ASC", t.selectCols, t.qname)
	rows, err := t.db.sql.Query(q)
	if err != nil {
		log.Printf("store %s.FindOne: %v", t.name, err)
		var z T
		return z, false
	}
	defer rows.Close()
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err != nil {
			continue
		}
		if pred(&v) {
			return v, true
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
	ix := t.idxMap[idx]
	if ix == nil {
		panic("store: no index " + idx + " on " + t.name)
	}
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? ORDER BY `id` ASC LIMIT 1",
		t.selectCols, t.qname, quoteIdent(ix.colName))
	v, err := t.scanRow(t.db.sql.QueryRow(q, normIdxKey(key)))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("store %s.OneBy(%s): %v", t.name, idx, err)
		}
		var z T
		return z, false
	}
	return v, true
}

// CountBy — число записей с ключом в индексе.
func (t *Table[T, PT]) CountBy(idx, key string) int {
	ix := t.idxMap[idx]
	if ix == nil {
		panic("store: no index " + idx + " on " + t.name)
	}
	var n int
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", t.qname, quoteIdent(ix.colName))
	if err := t.db.sql.QueryRow(q, normIdxKey(key)).Scan(&n); err != nil {
		log.Printf("store %s.CountBy(%s): %v", t.name, idx, err)
		return 0
	}
	return n
}

// Count — число записей по предикату (nil — все).
func (t *Table[T, PT]) Count(pred func(*T) bool) int {
	if pred == nil {
		return t.Len()
	}
	q := fmt.Sprintf("SELECT %s FROM %s", t.selectCols, t.qname)
	rows, err := t.db.sql.Query(q)
	if err != nil {
		log.Printf("store %s.Count: %v", t.name, err)
		return 0
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err == nil && pred(&v) {
			n++
		}
	}
	return n
}

// Scan обходит записи в порядке id; fn возвращает false для остановки.
func (t *Table[T, PT]) Scan(fn func(*T) bool) {
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY `id` ASC", t.selectCols, t.qname)
	rows, err := t.db.sql.Query(q)
	if err != nil {
		log.Printf("store %s.Scan: %v", t.name, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		v, err := t.scanRow(rows)
		if err != nil {
			continue
		}
		if !fn(&v) {
			return
		}
	}
}

// MaxID возвращает максимальный id.
func (t *Table[T, PT]) MaxID() int64 {
	var mx int64
	q := fmt.Sprintf("SELECT COALESCE(MAX(`id`), 0) FROM %s", t.qname)
	if err := t.db.sql.QueryRow(q).Scan(&mx); err != nil {
		log.Printf("store %s.MaxID: %v", t.name, err)
		return 0
	}
	return mx
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
