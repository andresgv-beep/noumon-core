package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestUpgradeFromOldSchema simula una actualización in-place: una DB creada por
// la versión anterior (tablas SIN item_id) reabierta por el nuevo openStore.
// Regresión de: "SQL logic error: no such column: item_id" al crear el índice
// de item_id antes de que migrateItemIDs añada la columna.
func TestUpgradeFromOldSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")

	old, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	oldSchema := `
CREATE TABLE favorites (lib TEXT NOT NULL, path TEXT NOT NULL, title TEXT, book TEXT, on_home INTEGER NOT NULL DEFAULT 0, created INTEGER NOT NULL, PRIMARY KEY (lib, path));
CREATE TABLE notes (lib TEXT NOT NULL, path TEXT NOT NULL, title TEXT, book TEXT, body TEXT NOT NULL, updated INTEGER NOT NULL, PRIMARY KEY (lib, path));
CREATE TABLE history (id INTEGER PRIMARY KEY AUTOINCREMENT, lib TEXT NOT NULL, path TEXT NOT NULL, title TEXT, book TEXT, visited INTEGER NOT NULL);
CREATE TABLE tags (lib TEXT NOT NULL, path TEXT NOT NULL, tag TEXT NOT NULL, title TEXT, book TEXT, created INTEGER NOT NULL, PRIMARY KEY (lib, path, tag));
`
	if _, err := old.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO favorites (lib, path, title, book, on_home, created) VALUES ('wiki','A/Foo','Foo','wiki',0,1)`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	st, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore falló en upgrade in-place: %v", err)
	}
	defer st.db.Close()

	// Tras el upgrade, los datos legacy quedan bajo el usuario invitado ("").
	favs, err := st.ListFavorites("")
	if err != nil {
		t.Fatalf("ListFavorites tras upgrade: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("esperaba 1 favorito preservado, got %d", len(favs))
	}
	// Un artículo ZIM conserva su identidad por lib/path: item_id vacío.
	if favs[0].ItemID != "" {
		t.Fatalf("favorito ZIM no debería tener itemId, got %q", favs[0].ItemID)
	}
}

func TestUpgradeRecreatesLegacyStudioFTSForPages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "studio-fts-old.db")
	st, err := openStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.db.Close(); err != nil {
		t.Fatal(err)
	}

	old, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`
		DROP TABLE studio_published_fts;
		CREATE VIRTUAL TABLE studio_published_fts USING fts5(
			document_id UNINDEXED,
			title,
			summary,
			plain_text,
			tags,
			work_type,
			topics,
			author_label,
			tokenize='unicode61 remove_diacritics 2'
		);
		INSERT INTO studio_published_fts (
			document_id, title, summary, plain_text, tags,
			work_type, topics, author_label
		) VALUES ('legacy', 'Viejo', '', 'dato antiguo', '', '', '', '');
	`); err != nil {
		old.Close()
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore con FTS antiguo: %v", err)
	}
	defer upgraded.db.Close()

	rows, err := upgraded.db.Query(`PRAGMA table_info(studio_published_fts)`)
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(
			&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey,
		); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !columns["page_id"] || !columns["page_title"] {
		t.Fatalf("FTS sin columnas multipágina tras upgrade: %#v", columns)
	}
	var count int
	if err := upgraded.db.QueryRow(`SELECT COUNT(*) FROM studio_published_fts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("el índice antiguo no se reconstruyó limpiamente: %d filas", count)
	}
}
