package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	dbURL := "postgres://postgres:RBSW2NW9-dy4d-63ZLK0DC@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require"
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	fmt.Println("=== 1. ALL tables with row counts ===")
	queryTables := `
		SELECT schemaname, relname, n_live_tup
		FROM pg_stat_user_tables
		ORDER BY n_live_tup DESC;
	`
	rows, err := conn.Query(ctx, queryTables)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, name string
		var count int64
		rows.Scan(&schema, &name, &count)
		fmt.Printf("%s.%s: %d rows\n", schema, name, count)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 2. ALL columns for each table (name, type, nullable, default) ===")
	queryCols := `
		SELECT table_schema, table_name, column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name, ordinal_position;
	`
	rows, err = conn.Query(ctx, queryCols)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, col, dataType, isNullable string
		var colDef *string
		rows.Scan(&schema, &table, &col, &dataType, &isNullable, &colDef)
		defVal := "NULL"
		if colDef != nil {
			defVal = *colDef
		}
		fmt.Printf("%s.%s.%s: %s, Nullable: %s, Default: %s\n", schema, table, col, dataType, isNullable, defVal)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 3. ALL indexes (table, index name, indexdef) ===")
	queryIdx := `
		SELECT schemaname, tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schemaname, tablename, indexname;
	`
	rows, err = conn.Query(ctx, queryIdx)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, idxName, idxDef string
		rows.Scan(&schema, &table, &idxName, &idxDef)
		fmt.Printf("%s.%s - %s: %s\n", schema, table, idxName, idxDef)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 4. ALL foreign keys ===")
	queryFKs := `
		SELECT
			tc.table_schema, 
			tc.constraint_name, 
			tc.table_name, 
			kcu.column_name, 
			ccu.table_schema AS foreign_table_schema,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name 
		FROM 
			information_schema.table_constraints AS tc 
			JOIN information_schema.key_column_usage AS kcu
			  ON tc.constraint_name = kcu.constraint_name
			  AND tc.table_schema = kcu.table_schema
			JOIN information_schema.constraint_column_usage AS ccu
			  ON ccu.constraint_name = tc.constraint_name
			  AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY';
	`
	rows, err = conn.Query(ctx, queryFKs)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, constraint, table, col, fSchema, fTable, fCol string
		rows.Scan(&schema, &constraint, &table, &col, &fSchema, &fTable, &fCol)
		fmt.Printf("%s.%s.%s -> %s.%s.%s (Constraint: %s)\n", schema, table, col, fSchema, fTable, fCol, constraint)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 5. ALL constraints (check, unique, exclusion) ===")
	queryConstraints := `
		SELECT conname, contype, pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema');
	`
	rows, err = conn.Query(ctx, queryConstraints)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var conName, conType, conDef string
		rows.Scan(&conName, &conType, &conDef)
		fmt.Printf("Constraint: %s, Type: %s, Def: %s\n", conName, conType, conDef)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 6. Tables WITHOUT primary keys ===")
	queryNoPK := `
		SELECT table_schema, table_name
		FROM information_schema.tables t
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND table_type = 'BASE TABLE'
		  AND NOT EXISTS (
			  SELECT 1
			  FROM information_schema.table_constraints tc
			  WHERE tc.table_schema = t.table_schema
				AND tc.table_name = t.table_name
				AND tc.constraint_type = 'PRIMARY KEY'
		  );
	`
	rows, err = conn.Query(ctx, queryNoPK)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table string
		rows.Scan(&schema, &table)
		fmt.Printf("%s.%s\n", schema, table)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 7. Columns that look like they should be indexed but aren't ===")
	queryNoIdxID := `
		SELECT c.table_schema, c.table_name, c.column_name
		FROM information_schema.columns c
		WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND c.column_name LIKE '%_id'
		  AND NOT EXISTS (
			  SELECT 1
			  FROM pg_index i
			  JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			  JOIN pg_class cls ON cls.oid = i.indrelid
			  JOIN pg_namespace n ON n.oid = cls.relnamespace
			  WHERE n.nspname = c.table_schema
				AND cls.relname = c.table_name
				AND a.attname = c.column_name
		  );
	`
	rows, err = conn.Query(ctx, queryNoIdxID)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, col string
		rows.Scan(&schema, &table, &col)
		fmt.Printf("%s.%s.%s\n", schema, table, col)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 8. Tables without any indexes beyond the PK ===")
	queryOnlyPKIdx := `
		SELECT schemaname, tablename
		FROM pg_indexes
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		GROUP BY schemaname, tablename
		HAVING COUNT(*) <= 1;
	`
	rows, err = conn.Query(ctx, queryOnlyPKIdx)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table string
		rows.Scan(&schema, &table)
		fmt.Printf("%s.%s\n", schema, table)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 9. Large tables without appropriate indexes ===")
	fmt.Println("(Refer to cross-referencing table counts and indexes output)")
	fmt.Println()

	fmt.Println("=== 10. Any columns with type TEXT that might be better as VARCHAR ===")
	queryTextCols := `
		SELECT table_schema, table_name, column_name
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND data_type = 'text';
	`
	rows, err = conn.Query(ctx, queryTextCols)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, col string
		rows.Scan(&schema, &table, &col)
		fmt.Printf("%s.%s.%s\n", schema, table, col)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 11. Any tables without created_at/updated_at timestamps ===")
	queryNoTimestamps := `
		SELECT t.table_schema, t.table_name
		FROM information_schema.tables t
		WHERE t.table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND t.table_type = 'BASE TABLE'
		  AND NOT EXISTS (
			  SELECT 1
			  FROM information_schema.columns c
			  WHERE c.table_schema = t.table_schema
				AND c.table_name = t.table_name
				AND c.column_name IN ('created_at', 'updated_at')
		  );
	`
	rows, err = conn.Query(ctx, queryNoTimestamps)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table string
		rows.Scan(&schema, &table)
		fmt.Printf("%s.%s\n", schema, table)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 12. Database roles and permissions ===")
	queryRoles := `
		SELECT rolname, rolsuper, rolinherit, rolcreaterole, rolcreatedb, rolcanlogin
		FROM pg_roles;
	`
	rows, err = conn.Query(ctx, queryRoles)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var rolname string
		var sup, inh, cr, cdb, logn bool
		rows.Scan(&rolname, &sup, &inh, &cr, &cdb, &logn)
		fmt.Printf("Role: %s (Super: %v, Inherit: %v, CreateRole: %v, CreateDB: %v, Login: %v)\n", rolname, sup, inh, cr, cdb, logn)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 13. Any RLS (Row Level Security) policies ===")
	queryRLS := `
		SELECT schemaname, tablename, policyname, roles, cmd, qual, with_check
		FROM pg_policies;
	`
	rows, err = conn.Query(ctx, queryRLS)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, policy string
		var roles []string
		var cmd string
		var qual, withCheck *string
		rows.Scan(&schema, &table, &policy, &roles, &cmd, &qual, &withCheck)
		qStr := "NULL"
		if qual != nil { qStr = *qual }
		cStr := "NULL"
		if withCheck != nil { cStr = *withCheck }
		fmt.Printf("%s.%s: %s (Cmd: %s, Qual: %s, WithCheck: %s)\n", schema, table, policy, cmd, qStr, cStr)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 14. Check for any functions/triggers defined ===")
	queryFuncs := `
		SELECT n.nspname as schema, p.proname as function_name
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schema, function_name;
	`
	rows, err = conn.Query(ctx, queryFuncs)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, fn string
		rows.Scan(&schema, &fn)
		fmt.Printf("Function: %s.%s\n", schema, fn)
	}
	rows.Close()

	queryTriggers := `
		SELECT event_object_schema, event_object_table, trigger_name, event_manipulation, action_statement, action_timing
		FROM information_schema.triggers
		WHERE event_object_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY event_object_schema, event_object_table, trigger_name;
	`
	rows, err = conn.Query(ctx, queryTriggers)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, table, trig, event, action, timing string
		rows.Scan(&schema, &table, &trig, &event, &action, &timing)
		fmt.Printf("Trigger: %s.%s.%s (Timing: %s, Event: %s) -> %s\n", schema, table, trig, timing, event, action)
	}
	rows.Close()
	fmt.Println()

	fmt.Println("=== 15. Check for any views or materialized views ===")
	queryViews := `
		SELECT table_schema, table_name
		FROM information_schema.views
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema');
	`
	rows, err = conn.Query(ctx, queryViews)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, view string
		rows.Scan(&schema, &view)
		fmt.Printf("View: %s.%s\n", schema, view)
	}
	rows.Close()
	
	queryMatViews := `
		SELECT schemaname, matviewname
		FROM pg_matviews
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema');
	`
	rows, err = conn.Query(ctx, queryMatViews)
	if err != nil {
		log.Fatalf("Query failed: %v\n", err)
	}
	for rows.Next() {
		var schema, view string
		rows.Scan(&schema, &view)
		fmt.Printf("Materialized View: %s.%s\n", schema, view)
	}
	rows.Close()
	fmt.Println()
}
