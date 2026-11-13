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
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	// The SQLite driver: SQLite translated to Go, so gnosh needs no C
	// compiler and no C library. It registers itself as "sqlite".
	_ "modernc.org/sqlite"
)

// openSQLite opens the SQLite database file at path, making it if it is not
// there, and checks that it can be opened. pragmas, name to value, are set
// on every connection the returned handle makes.
//
// It is the one way mem's index and vectors are opened (memindex.go,
// memvectors.go). Each of those is a file beside mem's journal that one
// process at a time writes, in transactions that read before they write,
// while other gnosh processes may have it open to read. The settings below
// are chosen for that from SQLite's documentation:
//
//   - The pragmas go into the connection string as the driver's "_pragma"
//     parameters instead of being run once on the handle. database/sql opens
//     connections as it needs them, and busy_timeout and secure_delete are
//     settings of a connection, not of the file (pragma.html): run once,
//     they would hold for one connection and be missing on the next, which
//     for secure_delete means deleted text left in the file. journal_mode
//     WAL, which the callers ask for so that readers do not block the
//     writer nor the writer the readers (wal.html), is kept by the file
//     itself, and asking for it again is harmless.
//
//   - busy_timeout is set before the other pragmas. Switching a file to WAL
//     needs the database's lock, and without a busy handler in place a
//     moment's contention fails the open with SQLITE_BUSY instead of
//     waiting.
//
//   - Transactions begin with BEGIN IMMEDIATE ("_txlock=immediate"). Every
//     transaction the callers begin goes on to write. SQLite's default,
//     DEFERRED, takes no lock at BEGIN: the transaction starts as a reader
//     at its first SELECT and is promoted to a writer at its first write,
//     and if another connection has written in between, the promotion
//     fails at once with SQLITE_BUSY, however long the busy timeout, since
//     waiting cannot make what was read current again
//     (lang_transaction.html; rescode.html, SQLITE_BUSY_SNAPSHOT). Taking
//     the write lock at BEGIN puts the wait where busy_timeout is honoured,
//     and nothing the transaction reads can go stale.
//
// Nothing else is set: the journal the index is rebuilt from is the record,
// so the file keeps SQLite's own defaults for the rest.
func openSQLite(path string, pragmas map[string]string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path, pragmas))
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening %s: %w", path, err)
	}
	// sql.Open only notes the connection string. Making a connection now
	// means a file that cannot be opened is reported here, by the call that
	// was asked to open it, and not by whichever statement runs first.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: opening %s: %w", path, err)
	}
	return db, nil
}

// sqliteDSN is the driver's connection string for the database file at path
// with pragmas set: a "file:" URI, as SQLite reads one, whose query holds one
// "_pragma=name(value)" for each pragma, busy_timeout first and the rest by
// name, and "_txlock=immediate". The driver sets them in that order too.
func sqliteDSN(path string, pragmas map[string]string) string {
	names := make([]string, 0, len(pragmas))
	for name := range pragmas {
		names = append(names, name)
	}
	first := func(name string) bool { return strings.EqualFold(name, "busy_timeout") }
	sort.Slice(names, func(i, j int) bool {
		if first(names[i]) != first(names[j]) {
			return first(names[i])
		}
		return names[i] < names[j]
	})
	query := url.Values{}
	for _, name := range names {
		query.Add("_pragma", name+"("+pragmas[name]+")")
	}
	query.Set("_txlock", "immediate")
	return sqliteURI(path) + "?" + query.Encode()
}

// sqliteURI is the "file:" URI of the file at path (https://www.sqlite.org/uri.html).
// A URI, not the bare path, because the driver takes everything after the
// first "?" of its connection string for parameters, and a path may hold a
// "?" of its own: in the URI, that and every other character with a meaning
// there ("#", "%") is written as an escape, which SQLite undoes. An absolute
// path follows an empty authority ("file:///dir/file"); a relative one
// follows the scheme ("file:dir/file"). A Windows path with a drive is
// written the way SQLite expects it, "file:///C:/dir/file".
func sqliteURI(path string) string {
	p := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = (&url.URL{Path: p}).EscapedPath()
	if strings.HasPrefix(p, "/") {
		return "file://" + p
	}
	return "file:" + p
}
