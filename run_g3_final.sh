echo "=== GATE 3 VERIFICATION COMMANDS ==="

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

echo "--- 6. cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c ---"
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c

echo "--- 7. find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v ' total$' | awk '$1>400' | wc -l ---"
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
