# Architecture: Local Mode and In-Process API Access

## The Problem

appbase has three runtime modes — server, CLI, and desktop — that all need to call the same API handlers. The current implementation routes all three through real HTTP/TCP, even when the caller and handler are in the same process. This creates unnecessary complexity:

- **DevAuth middleware** exists solely to fake sessions for local HTTP requests
- **Auto-serve** exists solely to give the CLI a TCP server to talk to
- **Session storms** happen because DevAuth creates real DB rows for ephemeral requests
- **Cookie jars** exist to carry sessions across in-process HTTP requests
- **Desktop overrides** auth endpoints because the middleware chain is confusing
- **`"cli-user"` hardcoding** creates data silos between CLI and web in the same DB

## The Principle

Everything should go through the **API contract**. That's not the same as "everything goes through HTTP."

The API contract is the handler interface — request in, response out, serialization enforced. HTTP/TCP is just one transport for that contract. When caller and handler are in the same process, you don't need TCP, sessions, cookies, or auth middleware — you need the handler and a way to call it.

## Reference Implementation: electrician

The [electrician](../../../projects/electrician) project demonstrates this principle cleanly:

**Handler is an `http.Handler`** — the shared contract boundary:
```go
// internal/api/handler.go
type Handler struct {
    storage *project.Storage
    mux     *http.ServeMux
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    h.mux.ServeHTTP(w, r)
}
```

**Three modes, same handler, different transports:**

| Mode | How it calls the handler | TCP? | Auth? |
|------|--------------------------|------|-------|
| Web/CLI server | `http.ListenAndServe(addr, handler)` | Yes | N/A |
| Desktop (Wails) | `AssetServer.Handler: handler` | No | N/A |
| Tests | `handler.ServeHTTP(recorder, request)` | No | N/A |

The handler doesn't know or care which transport is calling it. There's no auto-serve, no session management, no middleware gymnastics. Each mode just calls `ServeHTTP` through whatever mechanism is appropriate.

Electrician doesn't have auth, but the principle extends: when you're in-process, inject identity at the transport layer rather than faking sessions through middleware.

## Current appbase Architecture

```
Server mode:   HTTP client → TCP → middleware → DevAuth(no-op) → Auth → handler → store → DB
CLI remote:    HTTP client → TCP → middleware → DevAuth(no-op) → Auth → handler → store → DB
CLI local:     HTTP client → TCP* → middleware → DevAuth(session!) → Auth → handler → store → DB
Desktop:       Wails       → handler (with DevAuth and auth override hack)

* TCP to localhost via auto-serve ephemeral server
```

Problems with this:
- CLI local starts a real TCP server (`AutoServe`) just to send requests to itself
- `DevAuthMiddleware` creates a real session row in the DB for every request
- `sessionCookieJar` carries that session cookie for in-process HTTP calls
- Desktop mode has to override `/api/auth/status` to work around the auth pipeline
- Pattern A apps (todo, todo-store, bookmarks) bypass HTTP entirely and hardcode `"cli-user"`, creating data that doesn't match the web user (`"dev@localhost"`)

## Proposed Architecture

```
Server mode:   HTTP client → TCP → middleware → Auth → handler → store → DB
CLI remote:    HTTP client → TCP → middleware → Auth → handler → store → DB
CLI local:     handlerTransport → handler → store → DB  (identity injected at transport)
Desktop:       Wails handler    → handler → store → DB  (identity injected at transport)
Tests:         httptest         → handler → store → DB  (identity injected at transport)
```

### In-process transport

A custom `http.RoundTripper` that calls the handler directly via `httptest`:

```go
// cli/transport.go
type handlerTransport struct {
    handler http.Handler
    userID  string
    email   string
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    // Inject identity into request context — no cookies, no sessions, no middleware
    if t.userID != "" {
        ctx := context.WithValue(req.Context(), auth.UserIDKey, t.userID)
        ctx = context.WithValue(ctx, auth.EmailKey, t.email)
        req = req.WithContext(ctx)
    }

    rec := httptest.NewRecorder()
    t.handler.ServeHTTP(rec, req)
    return rec.Result(), nil
}
```

This preserves the API contract — serialization, validation, and handler logic all execute — but eliminates TCP, sessions, and cookies.

### Unified CLI client helper

```go
// cli/client.go

// ClientForCommand returns an HTTP client and base URL appropriate for the
// current mode. In local mode, returns an in-process transport that calls
// the handler directly. In remote mode, returns a real HTTP client with
// keychain auth.
func ClientForCommand(cmd *cobra.Command, appName string, handler http.Handler) (
    client *http.Client, baseURL string, cleanup func(), err error,
) {
    serverFlag, _ := cmd.Flags().GetString("server")
    serverURL := ResolveServerURL(serverFlag, appName)

    // Remote mode: real HTTP with keychain auth
    if serverFlag != "" || serverURL != "http://localhost:3000" {
        httpClient, err := AuthenticatedClient(appName)
        if err != nil {
            return nil, "", nil, fmt.Errorf("not logged in — run: %s login", appName)
        }
        return httpClient, serverURL, func() {}, nil
    }

    // Local mode: in-process transport, no TCP
    email := os.Getenv("DEV_USER_EMAIL")
    if email == "" {
        email = "dev@localhost"
    }
    transport := &handlerTransport{
        handler: handler,
        userID:  email,
        email:   email,
    }
    return &http.Client{Transport: transport}, "http://local", func() {}, nil
}
```

### What CLI commands look like after

```go
// Today (todo-api/main.go — repeated for every command):
if err := setup(); err != nil { return err }
serverURL, cleanup, err := appcli.ResolveServerWithAutoServe(cmd, "todo-api")
if err != nil { return err }
defer cleanup()
httpClient, err := appcli.AuthenticatedClient("todo-api")
if err != nil {
    httpClient = http.DefaultClient
}
client, err := api.NewClientWithResponses(serverURL, api.WithHTTPClient(httpClient))

// After:
if err := setup(); err != nil { return err }
httpClient, baseURL, cleanup, err := appcli.ClientForCommand(cmd, "todo-api", app.Handler())
if err != nil { return err }
defer cleanup()
client, err := api.NewClientWithResponses(baseURL, api.WithHTTPClient(httpClient))
```

### What desktop mode looks like after

```go
// Today (desktop.go):
appcli.SetupLocalMode("todo-api")
app, _ := appbase.New(...)
// ... register routes ...
// Override auth status because DevAuth is confusing:
app.Server().Router().Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
    server.RespondJSON(w, http.StatusOK, map[string]interface{}{
        "loggedIn": true, "email": "desktop-user",
    })
})
wails.Run(&options.App{AssetServer: &assetserver.Options{Handler: app.Handler()}})

// After:
app, _ := appbase.New(appbase.Config{Name: "todo-api", LocalMode: true})
// ... register routes ...
// No auth override needed — LocalMode configures auth status automatically
wails.Run(&options.App{AssetServer: &assetserver.Options{Handler: app.Handler()}})
```

## What Gets Eliminated

| Component | Lines | Current purpose | After |
|-----------|-------|-----------------|-------|
| `DevAuthMiddleware` | ~60 | Fake sessions for local HTTP | **Removed** |
| `AutoServe` | ~30 | Start ephemeral TCP server | **Removed** |
| `waitForReady` | ~15 | Poll ephemeral server health | **Removed** |
| `sessionCookieJar` | ~10 | Carry session for local requests | **Removed** |
| `SetupLocalMode` | ~10 | Set env vars for desktop | **Simplified** |
| Session storms | N/A | DevAuth creates row per request | **Gone** |
| Desktop auth override | ~6 | Manual auth status handler | **Gone** |
| `ensureDataDir` no-ops | ~20 | Copy-pasted dead code in all examples | **Gone** |
| Per-command boilerplate | ~40 | ResolveServer + AuthClient + fallback | **Reduced to 1 call** |

Total: ~190 lines removed, ~50 lines added (`handlerTransport` + `ClientForCommand`).

## What Stays the Same

- `App`, `DB`, `Server`, `store.Collection` — unchanged
- `GoogleAuth`, `SessionStore`, `auth.Middleware` — production auth unchanged
- CLI framework (`cobra`, built-in login/logout/whoami commands) — unchanged
- `LoginBrowser`, keychain storage — remote mode unchanged
- OpenAPI codegen, generated clients — unchanged
- Test harness — unchanged (already uses httptest, already works like this)

## Migration Path

This can be done incrementally:

1. **Add `handlerTransport` and `ClientForCommand`** — new code, no breaking changes
2. **Add `LocalMode` option to `appbase.Config`** — sets up DB path and auth without env var mutation
3. **Update examples** to use `ClientForCommand` instead of `ResolveServerWithAutoServe`
4. **Update desktop example** to use `LocalMode` instead of `SetupLocalMode` + auth overrides
5. **Deprecate** `AutoServe`, `DevAuthMiddleware`, `SetupLocalMode`, `AutoServeHandler`
6. **Remove** in a later release

Steps 1-2 are additive. Steps 3-4 are example updates. Steps 5-6 are cleanup. No existing app breaks at any step.

## Pattern A Apps (Direct Store Access)

Apps like todo, todo-store, and bookmarks bypass HTTP entirely — CLI commands call the store directly. This is fine for simple apps, but they hardcode `"cli-user"` as the user ID, which doesn't match what DevAuth uses (`"dev@localhost"`).

Fix: expose the local user identity so these apps use it consistently:

```go
// Instead of: todos.Create("cli-user", args[0])
todos.Create(appcli.LocalUserID(), args[0])
```

`LocalUserID()` returns `DEV_USER_EMAIL` or `"dev@localhost"` — the same identity used by the in-process transport and by web mode's auth middleware.
