# Migration: Local Mode and In-Process Transport

This guide covers migrating apps from the old auto-serve/DevAuth pattern to the new in-process transport. All appbase apps need these changes.

## What changed

The old pattern routed CLI local mode through real HTTP/TCP with DevAuth creating sessions. The new pattern calls the handler directly in-process, injecting identity at the transport layer. No sessions, no cookies, no TCP for local mode.

**Removed:** `AUTH_MODE=dev` env var, `AutoServeHandler`, `ResolveServerWithAutoServe`, `ensureDataDir` boilerplate.

**Added:** `Config.LocalMode`, `ClientForCommand`, `LocalHandler`, `LocalUserID`, `LocalDataPath`.

## Step 1: Update setup()

```go
// Before
func setup() error {
    app, err = appbase.New(appbase.Config{Name: "myapp", Quiet: !appcli.IsServeCommand})
    if err != nil { return err }
    // ...
    appcli.AutoServeHandler = app.Server().Router()  // remove this
    return nil
}

// After
func setup() error {
    cfg := appbase.Config{
        Name:      "myapp",
        Quiet:     !appcli.IsServeCommand,
        LocalMode: appcli.IsLocalMode,
    }
    if appcli.LocalDataPath != "" {
        cfg.DB.SQLitePath = appcli.LocalDataPath + "/app.db"
    }
    app, err = appbase.New(cfg)
    if err != nil { return err }
    // ...
    return nil
}
```

Key changes:
- `LocalMode: appcli.IsLocalMode` — tells appbase to skip DevAuth and configure auth status for local use
- `cfg.DB.SQLitePath` — explicit config instead of `os.Setenv("SQLITE_DB_PATH", ...)`
- Remove `appcli.AutoServeHandler = ...` — no longer needed

## Step 2: Update CLI commands (API-first apps)

```go
// Before
serverURL, cleanup, err := appcli.ResolveServerWithAutoServe(cmd, "myapp")
if err != nil { return err }
defer cleanup()
httpClient, err := appcli.AuthenticatedClient("myapp")
if err != nil {
    httpClient = http.DefaultClient
}
client, err := api.NewClientWithResponses(serverURL, api.WithHTTPClient(httpClient))

// After
httpClient, baseURL, cleanup, err := appcli.ClientForCommand(cmd, "myapp", app.Handler())
if err != nil { return err }
defer cleanup()
client, err := api.NewClientWithResponses(baseURL, api.WithHTTPClient(httpClient))
```

`ClientForCommand` handles both modes:
- **Local** (no `--server` flag): returns in-process transport, identity injected, no TCP
- **Remote** (`--server` flag or keychain): returns real HTTP client with keychain auth

## Step 3: Update CLI commands (direct store access)

For simpler apps that call the store directly instead of going through HTTP:

```go
// Before
todos.Create("cli-user", args[0])
todos.List("cli-user")

// After
todos.Create(appcli.LocalUserID(), args[0])
todos.List(appcli.LocalUserID())
```

`LocalUserID()` returns `DEV_USER_EMAIL` env var or `"dev@localhost"` — consistent with what the in-process transport uses.

## Step 4: Update desktop (Wails) apps

```go
// Before
appcli.SetupLocalMode("myapp")
app, _ := appbase.New(appbase.Config{Name: "myapp", Quiet: true})
// ... register routes ...
app.Server().Router().Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
    server.RespondJSON(w, http.StatusOK, map[string]interface{}{"loggedIn": true, "email": "desktop-user"})
})
wails.Run(&options.App{
    AssetServer: &assetserver.Options{Handler: app.Handler()},
})

// After
app, _ := appbase.New(appbase.Config{Name: "myapp", Quiet: true, LocalMode: true})
// ... register routes ...
wails.Run(&options.App{
    AssetServer: &assetserver.Options{Handler: app.LocalHandler()},
})
```

Key changes:
- `LocalMode: true` instead of `SetupLocalMode()`
- `app.LocalHandler()` instead of `app.Handler()` — wraps handler with identity injection
- Remove the `/api/auth/status` override — `LocalMode` handles it

## Step 5: Clean up

Remove from your app:
- `ensureDataDir()` functions (appbase handles data dir setup)
- `AUTH_MODE` references
- `appcli.AutoServeHandler` assignments
- `appcli.SetupLocalMode()` calls
- Any `os.Setenv("SQLITE_DB_PATH", ...)` calls
- Unused imports (`net/http` in commands that used `http.DefaultClient`)

## Other changes to be aware of

These don't require migration but affect your app:

- **CORS**: No longer defaults to `Access-Control-Allow-Origin: *`. If your frontend dev server is on a different port, set `Config.AllowedOrigins`.
- **SQLite driver**: Now `modernc.org/sqlite` (pure Go). If you import `go-sqlite3` directly in tests, switch to `modernc.org/sqlite` and change `sql.Open("sqlite3", ...)` to `sql.Open("sqlite", ...)`.
- **`App.Router()`**: Now returns `chi.Router` directly instead of an anonymous interface. More methods available, no behavior change.
- **`db.New()`**: Accepts optional `db.DBConfig`. Your app doesn't need to change — env var fallbacks still work.
- **Graceful shutdown**: `app.Serve()` now handles SIGINT/SIGTERM with a 10-second drain.
