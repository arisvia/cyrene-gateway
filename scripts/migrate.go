package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	var dataDir string
	flag.StringVar(&dataDir, "data-dir", "", "path to data directory (defaults to ~/.cyrene-gateway)")
	flag.Parse()

	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("failed to resolve user home dir: %v", err)
		}
		dataDir = filepath.Join(home, ".cyrene-gateway")
	}

	dbPath := filepath.Join(dataDir, "data.sqlite")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("database file does not exist at %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Printf("Running data migration on %s...\n", dbPath)

	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "providerConnections: codebuddy-intl -> codebuddy",
			sql:  "UPDATE providerConnections SET provider = 'codebuddy' WHERE provider = 'codebuddy-intl'",
		},
		{
			name: "providerConnections: alicode-intl -> alicode",
			sql:  "UPDATE providerConnections SET provider = 'alicode' WHERE provider = 'alicode-intl'",
		},
		{
			name: "requestDetails: codebuddy-intl -> codebuddy",
			sql:  "UPDATE requestDetails SET provider = 'codebuddy' WHERE provider = 'codebuddy-intl'",
		},
		{
			name: "requestDetails: alicode-intl -> alicode",
			sql:  "UPDATE requestDetails SET provider = 'alicode' WHERE provider = 'alicode-intl'",
		},
		{
			name: "usageHistory: codebuddy-intl -> codebuddy",
			sql:  "UPDATE usageHistory SET provider = 'codebuddy' WHERE provider = 'codebuddy-intl'",
		},
		{
			name: "usageHistory: alicode-intl -> alicode",
			sql:  "UPDATE usageHistory SET provider = 'alicode' WHERE provider = 'alicode-intl'",
		},
		{
			name: "kv: codebuddy-intl -> codebuddy",
			sql:  "UPDATE kv SET key = 'codebuddy' WHERE scope = 'providerModelCache' AND key = 'codebuddy-intl'",
		},
		{
			name: "kv: alicode-intl -> alicode",
			sql:  "UPDATE kv SET key = 'alicode' WHERE scope = 'providerModelCache' AND key = 'alicode-intl'",
		},
		{
			name: "providerConnections: opencode none auth -> api-key",
			sql:  "UPDATE providerConnections SET authType = 'api-key', isActive = 0 WHERE provider = 'opencode' AND authType = 'none'",
		},
	}

	for _, m := range migrations {
		res, err := db.Exec(m.sql)
		if err != nil {
			fmt.Printf("  [SKIP/ERR] %s: %v\n", m.name, err)
			continue
		}
		affected, _ := res.RowsAffected()
		fmt.Printf("  [OK] %s (rows affected: %d)\n", m.name, affected)
	}

	fmt.Println("Data migration completed successfully.")
}
