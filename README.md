# webkit

A small Go framework for server-rendered web apps that ship as a single binary.

- Routing and handlers are plain `net/http`: `http.ServeMux` patterns and
  `http.HandlerFunc`.
- Views are [gomponents](https://maragu.dev/gomponents) and interactivity
  comes from [htmx](https://htmx.org).
- Static assets and SQL migrations are embedded with `go:embed`.
- Data lives in SQLite through [bun](https://bun.uptrace.dev), with
  [scs](https://github.com/alexedwards/scs) sessions stored in the same
  database.

Each package can be used on its own; take only what you need.

## Packages

| Package | Purpose |
|---|---|
| `webkit` | Base errors that carry an HTTP status: `NotFound`, `Forbidden`, `InvalidValue`, `AlreadyExists`, `PreconditionFailed`, `Timeout`, `Internal` |
| `web` | `NewHandler`, middleware (`Chain`, `Recover`, `Logger`, `CrossOrigin`, `Timeout`), `Render`, htmx-aware `Redirect`, form and path decoding with `Decode`, typed `ContextKey` |
| `session` | Session manager, flash messages, `LoadUser` and `RequireUser` middleware |
| `assets` | Embedded static assets served with long-lived caching and checksum cache-busting paths |
| `sqlite` | bun SQLite client, error normalization, `Optimize`, `MigrateUp` / `MigrateDown` from an `fs.FS` |
| `snowflake` | Snowflake IDs |
| `app` | Run an HTTP server with background tasks and graceful shutdown |
| `testutil` | [Ginkgo](https://onsi.github.io/ginkgo/) `Must` helpers |

## Installation

```sh
go get github.com/mgnsk/webkit
```

## Error handling

Handlers report errors by panicking. `web.Recover`, which `web.NewHandler`
installs, recovers the panic, logs it and renders a response:

- Errors created from the base errors use their status code and message:
  `webkit.NotFound.New("Group not found")` renders a 404 with
  "Group not found".
- `Decode` failures become 400 "Invalid request", and
  `context.DeadlineExceeded` becomes 504 "Timeout".
- Anything else is a 500 "Something went wrong". Stack traces are logged for
  5xx responses only.

Full page requests get `ErrorRenderer.Page`. When `ErrorRenderer.Toast` is
set, htmx requests instead get the toast, swapped into `ToastTarget` with
`HX-Retarget` and `HX-Reswap`, so a failed form submission shows a message
without replacing the page.

`sqlite.NormalizeError` maps database errors onto the base errors: no rows
becomes `NotFound` and unique constraint violations become `AlreadyExists`.

## Example

```go
//go:embed assets
var assetsDir embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

var static = assets.New(must(fs.Sub(assetsDir, "assets")))

var userKey web.ContextKey[User]

func run() error {
	db := sqlite.NewDB("app.sqlite").WithForeignKeys().Connect()
	defer db.Close()

	if err := sqlite.MigrateUp(db.DB, migrations, "migrations"); err != nil {
		return err
	}

	if err := sqlite.Optimize(context.Background(), db.DB); err != nil {
		return err
	}

	store, err := bunstore.New(db)
	if err != nil {
		return err
	}

	sm := session.New(store, session.Options{Secure: true})

	// Public pages load the session and the user, if signed in.
	public := []web.Middleware{
		session.Middleware(sm),
		session.LoadUser(sm, userKey, func(ctx context.Context, sm *scs.SessionManager) (*User, error) {
			id := sm.GetInt64(ctx, "user_id")
			if id == 0 {
				return nil, nil // anonymous
			}
			return GetUser(ctx, db, id) // webkit.NotFound signs the user out
		}, "/login"),
	}

	// Private pages also require a signed-in user. Anonymous visitors are
	// redirected to /login, which must therefore be public.
	private := append(slices.Clone(public), session.RequireUser(userKey, "/login"))

	h := web.NewHandler(web.Options{
		Errors: web.ErrorRenderer{
			Page:        ErrorPage,
			Toast:       ErrorToast,
			ToastTarget: "#toast",
		},
	},
		static.Register,
		func(mux *http.ServeMux) {
			mux.Handle("GET /login", web.Chain(http.HandlerFunc(loginPage), public...))
			mux.Handle("POST /login", web.Chain(http.HandlerFunc(login), public...))
			mux.Handle("GET /{$}", web.Chain(http.HandlerFunc(home), private...))
		},
	)

	srv := &http.Server{Addr: ":8080", Handler: h}

	return app.Run(context.Background(), srv,
		app.Every(24*time.Hour, func(ctx context.Context, _ time.Time) error {
			return sqlite.Optimize(ctx, db.DB)
		}),
	)
}
```

The `login` handler signs the user in by storing their ID in the session:

```go
if err := sm.RenewToken(r.Context()); err != nil {
	panic(err)
}
sm.Put(r.Context(), "user_id", user.ID)
web.Redirect(w, r, "/")
```

A handler decodes its input, panics on errors and renders a page or a
fragment:

```go
type createGroupInput struct {
	Name string `in:"form=name"`
}

func createGroup(w http.ResponseWriter, r *http.Request) {
	in := web.Decode[createGroupInput](r)
	if in.Name == "" {
		panic(webkit.InvalidValue.New("Name is required"))
	}

	group, err := CreateGroup(r.Context(), db, userKey.Get(r.Context()), in.Name)
	if err != nil {
		panic(err)
	}

	session.Flash(r.Context(), sm, "Group created")
	web.Redirect(w, r, "/groups/"+group.ID.String())
}
```

`web.Redirect` sets `HX-Redirect` for htmx requests and responds with a
303 otherwise. Reference assets in views with `static.Path("app.css")`, which
returns `/assets/app.css?crc=<checksum>`.

## Middleware order

`web.NewHandler` wraps the mux, outermost first, in:

1. `Logger`: request logging with `log/slog`.
2. `Recover`: error pages and toasts.
3. `CrossOrigin`: rejects unsafe cross-origin browser requests (disable with
   `Options.AllowCrossOrigin`).
4. `Timeout`: request context deadline (`Options.RequestTimeout`, default one
   minute).

Add per-route middleware such as sessions with `web.Chain`.

## Development

```sh
make test # go test ./...
make lint # go vet and revive
```
