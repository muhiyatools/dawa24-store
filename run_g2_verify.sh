echo "=== GATE 2 VERIFICATION SUITE ==="

echo "--- 1. gofmt -l ./cmd ./internal ---"
gofmt -l ./cmd ./internal

echo "--- 2. go vet ./... ---"
go vet ./...

echo "--- 3. go test ./... -count=1 ---"
go test ./... -count=1

echo "--- 4. grep -oh 'style=\"' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l ---"
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l

echo "--- 5. grep -oh '!important' internal/ui/static/css/*.css | wc -l ---"
grep -oh '!important' internal/ui/static/css/*.css | wc -l
