echo "=== 1. GOFMT CHECK ==="
gofmt -l ./cmd ./internal

echo "=== 2. GO VET ==="
go vet ./...

echo "=== 3. INLINE STYLES COUNT ==="
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l | tr -d ' '

echo "=== 4. RAW DIALOG COUNT ==="
grep -oh '<dialog' internal/ui/pages/*.templ 2>/dev/null | wc -l | tr -d ' '

echo "=== 5. IMPORTANT COUNT ==="
grep -oh '!important' internal/ui/static/css/*.css | wc -l | tr -d ' '

echo "=== 6. OVERSIZED GO FILES COUNT ==="
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l | tr -d ' '

echo "=== 7. ARABIC LITERALS COUNT ==="
pat=$(printf '\330'); LC_ALL=C grep -rc "\"[^\"]*$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd | grep -v "_test\|_templ\|/i18n/" | awk -F: '{s+=$2} END{print s+0}'

echo "=== 8. CSS BUNDLE BYTES ==="
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c | tr -d ' '

echo "=== 9. GO TEST SHORT ==="
go test ./... -short -count=1
