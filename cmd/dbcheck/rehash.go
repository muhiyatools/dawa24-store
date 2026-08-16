package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

// rehash recomputes the recorded checksum of already-applied migrations from
// the files currently on disk, but ONLY where the content is identical apart
// from line endings.
//
// The migration runner refuses to start when a recorded hash differs from the
// file, which is correct: an edited migration means environments have silently
// diverged. Line endings are the one exception. A file applied from a CRLF
// working tree and later normalised to LF by .gitattributes has not changed in
// any way PostgreSQL can observe, but its sha256 has.
//
// This refuses to touch anything whose content genuinely differs, so it cannot
// be used to paper over a real edit.
func rehash(dsn, dir string, apply bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `SELECT version, name, hash FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read schema_migrations: %v\n", err)
		os.Exit(1)
	}

	type record struct {
		version int
		name    string
		hash    string
	}
	var applied []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.version, &r.name, &r.hash); err != nil {
			break
		}
		applied = append(applied, r)
	}
	rows.Close()

	for _, r := range applied {
		path := filepath.Join(dir, fmt.Sprintf("%03d_%s.up.sql", r.version, r.name))
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  %03d %-32s NO FILE ON DISK — a fresh database would never receive this\n", r.version, r.name)
			continue
		}

		sum := sha256.Sum256(raw)
		current := hex.EncodeToString(sum[:])
		if current == r.hash {
			continue
		}

		// Only a line-ending difference is forgivable.
		crlf := sha256.Sum256(normaliseToCRLF(raw))
		if hex.EncodeToString(crlf[:]) != r.hash {
			fmt.Printf("  %03d %-32s CONTENT DIFFERS — not a line-ending change, refusing\n", r.version, r.name)
			continue
		}

		fmt.Printf("  %03d %-32s line endings only (%s -> %s)\n",
			r.version, r.name, r.hash[:12], current[:12])
		if apply {
			if _, err := conn.Exec(ctx,
				`UPDATE public.schema_migrations SET hash = $2 WHERE version = $1`,
				r.version, current); err != nil {
				fmt.Fprintf(os.Stderr, "    update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("    recorded hash updated")
		}
	}

	if !apply {
		fmt.Println("\ndry run; pass -rehash-apply to write the corrected hashes")
	}
}

func normaliseToCRLF(b []byte) []byte {
	out := make([]byte, 0, len(b)+len(b)/20)
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' && (i == 0 || b[i-1] != '\r') {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, b[i])
	}
	return out
}
