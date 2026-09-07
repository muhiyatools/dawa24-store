// Command dbq runs a single query against the database and prints the result
// as tab-separated text. Scratch tool for schema inspection.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

func main() {
	dsn := os.Getenv("DBQ_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DBQ_DSN not set")
		os.Exit(1)
	}
	var sql string
	if len(os.Args) > 1 {
		sql = strings.Join(os.Args[1:], " ")
	} else {
		b, _ := io.ReadAll(os.Stdin)
		sql = string(b)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	defer rows.Close()
	fds := rows.FieldDescriptions()
	names := make([]string, len(fds))
	for i, fd := range fds {
		names[i] = fd.Name
	}
	fmt.Println(strings.Join(names, "\t"))
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			fmt.Fprintln(os.Stderr, "values:", err)
			os.Exit(1)
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			if v == nil {
				cells[i] = "NULL"
				continue
			}
			cells[i] = strings.ReplaceAll(fmt.Sprintf("%v", v), "\n", " ")
		}
		fmt.Println(strings.Join(cells, "\t"))
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "rows:", err)
		os.Exit(1)
	}
}
