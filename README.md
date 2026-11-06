# webkit

A small Go web framework for server-rendered apps: stdlib `net/http` handlers,
[htmx](https://htmx.org), [gomponents](https://maragu.dev/gomponents) and
assets embedded into a single binary. Shared by splitgo and calendar.

Handlers report errors by panicking; `web.Recover` turns them into an error
page (or an htmx toast) with the status code taken from the error.

## Packages

| Package | Purpose |
|---|---|
| `webkit` | Base errors carrying an HTTP status (`NotFound`, `Forbidden`, ...) |
| `web` | `NewHandler`, middleware (`Chain`, `Recover`, `Logger`, `CrossOrigin`, `Timeout`), `Render`, htmx-aware `Redirect`, `Decode` (httpin), typed `ContextKey` |
| `session` | scs session manager, flash messages, `LoadUser` / `RequireUser` |
| `assets` | Embedded static assets with CRC cache-busting paths |
| `sqlite` | bun SQLite client, error normalization, periodic optimizer |
| `migrate` | golang-migrate up/down from an `fs.FS` |
| `snowflake` | Snowflake IDs |
| `app` | Run a server with background tasks and graceful shutdown |
| `testutil` | ginkgo `Must` helpers |

## Example

```go
//go:embed assets
var embedded embed.FS

var Assets = assets.New(must(fs.Sub(embedded, "assets")))

h := web.NewHandler(web.Options{
	Errors: web.ErrorRenderer{
		Page:        func(r *http.Request, code int, msg string) gomponents.Node { /* layout */ },
		Toast:       html.Toast,
		ToastTarget: "#toast",
	},
}, Assets.Register, myHandler.Register)

srv := &http.Server{Addr: ":8080", Handler: h}
err := app.Run(context.Background(), srv,
	func(ctx context.Context) error { return sqlite.RunOptimizer(ctx, db.DB) },
)
```
