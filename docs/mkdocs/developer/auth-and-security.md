# Auth & Security

Two independent auth schemes exist in this codebase, scoped to different route groups. They don't share code and aren't interchangeable.

## Web UI: JWT-in-cookie + CSRF

Applies to every route mounted via `webhandler.RegisterWebRoutes` (dashboard, documents, chunks, knowledge, retrieval, check, evidence).

- `auth.Auth` (`auth/auth.go`) issues an access/refresh JWT pair (HS256, `APP_JWT_SECRET`, `APP_JWT_EXPIRY_HOURS`) on login/registration. Claims carry `user_id`, `name`, `email`, `role`.
- `handler.JWTFromCookieMiddleware` pulls the token out of a named cookie; `handler.JWTParserMiddleware` validates it and, on success, attaches the claims to the request context via `handler.ClaimsFromContext`. An invalid/missing token is **not** an error at this stage — it just leaves the request unauthenticated.
- `webhandler.RequireAuthMiddleware` is what actually enforces auth — it redirects to `/login` if `ClaimsFromContext` returns `nil`. It's applied to the protected route group in `webhandler/entry.go`.
- `webhandler.RequireRoleMiddleware(minimum)` exists for role-gating but **isn't applied to any route yet** — reserved for future admin-only pages.
- CSRF protection (`middleware.CSRFWithConfig`, form-field lookup `_csrf`) wraps the whole web route group in `core/app.go`.
- Passwords are hashed with bcrypt (cost 12 in production, `auth.WithMinCost()` for tests/seed scripts) via `auth/password.go`.
- `/login` has its own IP-based rate limiter (5 req/min, burst 2) — see `webhandler/ratelimit.go`.

## REST API: none today

`/api/v1/*` has no authentication applied — see [API Authentication](../api/authentication.md) for the full picture. Two pieces of evidence this was planned but never finished:

- A comment in `handler/web_middleware.go` says the API group "uses `JWTFromHeaderMiddleware` instead, if/when it needs auth" — **that function doesn't exist anywhere in the codebase.** It's an aspirational reference, not a real code path.
- `handler.PSKAuthMiddleware` (pre-shared-key checker) exists and is fully implemented, but isn't attached to any route.

If you're adding API authentication, these are the two most-referenced starting points in the existing code — but treat both as unfinished scaffolding, not working features.

## Secrets

`config.Config` marks every credential field `json:"-"` (JWT secret, DB URI, S3/VLM/embedding/extraction/translation/claim-check API keys) so `Config` can be logged/inspected without leaking them. Never add a new secret field without the same tag.
