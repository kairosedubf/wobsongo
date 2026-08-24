# Authentication

**`/api/v1/*` currently has no authentication applied.** This is a known limitation of the current release, not an intentional "public by design" choice — if you're deploying this beyond a trusted internal network, put it behind your own auth layer (a gateway, a reverse proxy with API-key checking, a VPN, etc.) until first-class API authentication lands.

## What exists in the codebase but isn't wired up

The codebase includes `handler.PSKAuthMiddleware` — a pre-shared-key checker that expects an `Authorization: PSK <key>` header and compares it in constant time — along with `BOT_EXTRACT_PSK`/`BOT_CALLBACK_PSK` environment variables. **Neither is currently attached to any route.** They exist as groundwork for a future bot-integration auth scheme, not as something you can rely on today. Don't set those env vars expecting them to protect anything yet.

## How the web UI differs

The bundled web UI (`/login`, `/dashboard`, `/documents`, etc.) is authenticated separately — via a JWT stored in an HTTP-only cookie plus CSRF token validation on state-changing requests. That scheme is scoped to the HTML routes and doesn't extend to `/api/v1/*`. See [Auth & Security](../developer/auth-and-security.md) for how it works if you're extending the web layer.

## If you need to lock this down today

Options, roughly in order of effort:

1. Put the API behind a reverse proxy/gateway that enforces your own API-key or mTLS scheme.
2. Restrict network access (VPC/firewall rules, VPN-only exposure).
3. Wire up `PSKAuthMiddleware` yourself on the `/api/v1` group in `handler/entry.go` if a shared-secret scheme is sufficient for your deployment.
