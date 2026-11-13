// Copyright (C) 2026 Jae Kwon
//
// This file is part of gnosh's GnoVM machine subsystem. It is licensed under
// the GNO Network General Public License, version 6, as published by
// NewTendermint, LLC or its successor, or any later version of that License
// that the Licensor accepts as the proxy under its section 14, by a statement
// published in the canonical copy of internal/gnovm/LICENSE.md; together with
// the Additional Terms (Strong Attribution) in that file.

package gnovm

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// memPragmas are the pragmas mem's index and vectors are opened with.
var memPragmas = map[string]string{
	"journal_mode":  "WAL",
	"secure_delete": "ON",
	"busy_timeout":  "5000",
}

func TestSQLiteURI(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("these are Unix paths")
	}
	for path, want := range map[string]string{
		"/data/gnovm/index-1.db":   "file:///data/gnovm/index-1.db",
		"/a dir/what?/50%/#1/x.db": "file:///a%20dir/what%3F/50%25/%231/x.db",
		"//twice/x.db":             "file:////twice/x.db",
		"index.db":                 "file:index.db",
		"some dir/in:dex.db":       "file:some%20dir/in:dex.db",
	} {
		require.Equal(t, want, sqliteURI(path), path)
	}
}

func TestSQLiteDSN(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("these are Unix paths")
	}
	require.Equal(t,
		"file:///d/i.db?_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29&_pragma=secure_delete%28ON%29&_txlock=immediate",
		sqliteDSN("/d/i.db", memPragmas), "busy_timeout first, then by name")
	require.Equal(t, "file:///d/i.db?_txlock=immediate", sqliteDSN("/d/i.db", nil))
}

// The file is made at the path given, whatever characters the path holds.
func TestOpenSQLiteAtAnOddPath(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "a dir", "what?", "50%", "#1")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, "index&x=1.db")
	db, err := openSQLite(path, memPragmas)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE t(x)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	st, err := os.Stat(path)
	require.NoError(t, err, "the database is at the path asked for")
	require.NotZero(t, st.Size())

	db, err = openSQLite(path, memPragmas)
	require.NoError(t, err)
	defer db.Close()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM t`).Scan(&n), "and is found there again")
}

// A file that cannot be opened is reported by openSQLite itself.
func TestOpenSQLiteReportsAFileItCannotOpen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "index.db")
	db, err := openSQLite(path, memPragmas)
	if err == nil {
		db.Close()
	}
	require.ErrorContains(t, err, path)
}

// Every connection the handle makes has the pragmas, not only the first.
func TestOpenSQLiteSetsPragmasOnEveryConnection(t *testing.T) {
	t.Parallel()
	db, err := openSQLite(filepath.Join(t.TempDir(), "index.db"), memPragmas)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	var conns []*sql.Conn
	for range 3 {
		// Each is held while the next is asked for, so each is a
		// connection of its own.
		c, err := db.Conn(ctx)
		require.NoError(t, err)
		defer c.Close()
		conns = append(conns, c)
	}
	for i, c := range conns {
		var timeout, secure int
		var mode string
		require.NoError(t, c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout))
		require.NoError(t, c.QueryRowContext(ctx, `PRAGMA secure_delete`).Scan(&secure))
		require.NoError(t, c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode))
		require.Equal(t, 5000, timeout, "connection %d", i)
		require.Equal(t, 1, secure, "connection %d", i)
		require.Equal(t, "wal", mode, "connection %d", i)
	}
}

// A transaction holds the write lock from its BEGIN: a second writer is
// turned away there, before it has read anything, and a reader is not held
// up by either.
func TestOpenSQLiteBeginsTransactionsAsWriters(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "index.db")
	// No busy timeout, so that being turned away shows at once.
	pragmas := map[string]string{"journal_mode": "WAL", "busy_timeout": "0"}
	one, err := openSQLite(path, pragmas)
	require.NoError(t, err)
	defer one.Close()
	_, err = one.Exec(`CREATE TABLE t(x); INSERT INTO t VALUES(1)`)
	require.NoError(t, err)
	// A handle of its own, as another process would have.
	two, err := openSQLite(path, pragmas)
	require.NoError(t, err)
	defer two.Close()

	tx, err := one.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	var n int
	require.NoError(t, tx.QueryRow(`SELECT count(*) FROM t`).Scan(&n))

	_, err = two.Begin()
	var busy *sqlite.Error
	require.ErrorAs(t, err, &busy, "a second writer is refused at BEGIN")
	require.Equal(t, sqlite3.SQLITE_BUSY, busy.Code()&0xff)
	require.NoError(t, two.QueryRow(`SELECT count(*) FROM t`).Scan(&n), "a reader goes on")
	require.Equal(t, 1, n)

	_, err = tx.Exec(`INSERT INTO t VALUES(2)`)
	require.NoError(t, err, "the first writer's read did not go stale")
	require.NoError(t, tx.Commit())
	tx2, err := two.Begin()
	require.NoError(t, err, "the lock is free once the first writer commits")
	require.NoError(t, tx2.Rollback())
	require.NoError(t, two.QueryRow(`SELECT count(*) FROM t`).Scan(&n))
	require.Equal(t, 2, n)
}
