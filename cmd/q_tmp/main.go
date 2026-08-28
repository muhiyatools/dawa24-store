package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DBURL"))
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, os.Args[1])
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for _, f := range rows.FieldDescriptions() {
		fmt.Print(f.Name, "\t")
	}
	fmt.Println()
	for rows.Next() {
		v, _ := rows.Values()
		fmt.Println(v...)
	}
	if rows.Err() != nil {
		panic(rows.Err())
	}
}
