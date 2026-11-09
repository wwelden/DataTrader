package schema

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestApplyCreatesTables(t *testing.T) {
	db := openDB(t)
	if err := Apply(db); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"users", "stock_trades", "option_trades", "stock_positions", "closed_stocks", "option_positions", "closed_options"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	db := openDB(t)
	for i := range 3 {
		if err := Apply(db); err != nil {
			t.Fatalf("Apply run %d: %v", i+1, err)
		}
	}
}

func TestQuantityDefaultsToOne(t *testing.T) {
	db := openDB(t)
	if err := Apply(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date)
		VALUES (1, 'AAPL', 1, 1, 1, 'e', 'Call', 0, 'p')`); err != nil {
		t.Fatal(err)
	}
	var qty float64
	if err := db.QueryRow("SELECT quantity FROM option_positions").Scan(&qty); err != nil {
		t.Fatal(err)
	}
	if qty != 1 {
		t.Errorf("default quantity = %v, want 1", qty)
	}
}

func TestMigrationAddsQuantityToOldTables(t *testing.T) {
	db := openDB(t)
	// An option_positions table from before the quantity column existed.
	if _, err := db.Exec(`CREATE TABLE option_positions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, ticker TEXT NOT NULL,
		price REAL NOT NULL, premium REAL NOT NULL, strike REAL NOT NULL, exp_date TEXT NOT NULL,
		type TEXT NOT NULL, collateral REAL NOT NULL, purchase_date TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date)
		VALUES (1, 'AAPL', 1, 1, 1, 'e', 'Call', 0, 'p')`); err != nil {
		t.Fatal(err)
	}

	if err := Apply(db); err != nil {
		t.Fatal(err)
	}

	var qty float64
	if err := db.QueryRow("SELECT quantity FROM option_positions").Scan(&qty); err != nil {
		t.Fatalf("quantity column not added: %v", err)
	}
	if qty != 1 {
		t.Errorf("migrated quantity = %v, want 1", qty)
	}
}

func TestUniqueConstraints(t *testing.T) {
	db := openDB(t)
	if err := Apply(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (username, password) VALUES ('a', 'x')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (username, password) VALUES ('a', 'y')"); err == nil {
		t.Error("duplicate usernames should be rejected")
	}

	insert := "INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date) VALUES (1, 'AAPL', 1, 1, '2025-01-01')"
	if _, err := db.Exec(insert); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert); err == nil {
		t.Error("duplicate (user, ticker, open_date) stock positions should be rejected")
	}
}
