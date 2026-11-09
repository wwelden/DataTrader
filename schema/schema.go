// Package schema holds the SQLite schema shared by the app and its tests.
package schema

import (
	"database/sql"
	"log"
)

// SQL creates every table and index the app uses. It is idempotent.
const SQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS stock_trades (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    ticker TEXT NOT NULL,
    date TEXT NOT NULL,
    code TEXT NOT NULL,
    price REAL NOT NULL,
    amount REAL NOT NULL,
    quantity REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS option_trades (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    ticker TEXT NOT NULL,
    date TEXT NOT NULL,
    code TEXT NOT NULL,
    price REAL NOT NULL,
    amount REAL NOT NULL,
    quantity REAL NOT NULL,
    strike REAL NOT NULL,
    exp_date TEXT NOT NULL,
    option_type TEXT NOT NULL,
    premium REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS stock_positions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    open_date TEXT NOT NULL,
    ticker TEXT NOT NULL,
    quantity REAL NOT NULL,
    cost_basis REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, ticker, open_date)
);

CREATE TABLE IF NOT EXISTS closed_stocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    ticker TEXT NOT NULL,
    open_date TEXT NOT NULL,
    close_date TEXT NOT NULL,
    quantity REAL NOT NULL,
    cost_basis REAL NOT NULL,
    sell_price REAL NOT NULL,
    profit_loss REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS option_positions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    ticker TEXT NOT NULL,
    price REAL NOT NULL,
    premium REAL NOT NULL,
    strike REAL NOT NULL,
    exp_date TEXT NOT NULL,
    type TEXT NOT NULL,
    collateral REAL NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    purchase_date TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS closed_options (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    ticker TEXT NOT NULL,
    price REAL NOT NULL,
    premium REAL NOT NULL,
    strike REAL NOT NULL,
    exp_date TEXT NOT NULL,
    type TEXT NOT NULL,
    collateral REAL NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    purchase_date TEXT NOT NULL,
    close_date TEXT NOT NULL,
    sell_price REAL NOT NULL,
    profit_loss REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_stock_trades_user_id ON stock_trades(user_id);
CREATE INDEX IF NOT EXISTS idx_stock_trades_ticker ON stock_trades(ticker);
CREATE INDEX IF NOT EXISTS idx_option_trades_user_id ON option_trades(user_id);
CREATE INDEX IF NOT EXISTS idx_option_trades_ticker ON option_trades(ticker);
CREATE INDEX IF NOT EXISTS idx_stock_positions_user_id ON stock_positions(user_id);
CREATE INDEX IF NOT EXISTS idx_stock_positions_ticker ON stock_positions(ticker);
CREATE INDEX IF NOT EXISTS idx_closed_stocks_user_id ON closed_stocks(user_id);
CREATE INDEX IF NOT EXISTS idx_closed_options_user_id ON closed_options(user_id);
`

// Apply creates the schema on db and runs migrations for older databases.
func Apply(db *sql.DB) error {
	if _, err := db.Exec(SQL); err != nil {
		return err
	}
	runMigrations(db)
	return nil
}

func runMigrations(db *sql.DB) {
	_, err := db.Exec(`
		ALTER TABLE option_positions ADD COLUMN quantity REAL NOT NULL DEFAULT 1
	`)
	if err != nil {
		log.Println("Migration note: quantity column may already exist in option_positions")
	}

	_, err = db.Exec(`
		ALTER TABLE closed_options ADD COLUMN quantity REAL NOT NULL DEFAULT 1
	`)
	if err != nil {
		log.Println("Migration note: quantity column may already exist in closed_options")
	}
}
