package layouts

// Asset resolves a static asset path to the URL the browser should request.
//
// It is a variable rather than a direct call because the asset table lives in
// package ui, which imports this package: calling into it from here would be a
// cycle. ui installs the real resolver at start-up (see internal/ui/static.go);
// until then, and in any test that renders a layout without the rest of the
// server, the identity function below returns a path that still resolves.
//
// The alternative was the previous arrangement, where every asset URL carried a
// version string typed into the layout by hand — and those had already drifted
// apart from each other by two days.
var Asset = func(path string) string { return path }
