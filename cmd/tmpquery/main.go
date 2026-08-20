package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	for _, q := range strings.Split(os.Args[1], ";;") {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		fmt.Println("── " + q)
		rows, err := conn.Query(ctx, q)
		if err != nil {
			fmt.Println("   ERROR:", err)
			continue
		}
		fds := rows.FieldDescriptions()
		names := make([]string, len(fds))
		for i, fd := range fds {
			names[i] = fd.Name
		}
		fmt.Println("   " + strings.Join(names, " | "))
		n := 0
		for rows.Next() {
			vals, _ := rows.Values()
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = fmt.Sprintf("%v", v)
			}
			fmt.Println("   " + strings.Join(parts, " | "))
			n++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			fmt.Println("   ERR:", err)
		}
		fmt.Printf("   (%d rows)\n\n", n)
	}
}
