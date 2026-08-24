# Rate Limits

There is no rate limiting enforced on `/api/v1/*` today. (The web UI's `/login` endpoint has its own IP-based rate limiter, but that's a separate concern from the REST API.)

The only bound you'll run into is pagination: list endpoints accept `page` and `per_page` query parameters (`dto.PaginationDTO`), defaulting to 20 items per page with a hard cap of 100 per page — requesting more than 100 silently clamps to 100 rather than erroring.

If you're integrating a bot or automated pipeline against this API, be a good citizen and throttle client-side — a future release may add server-side limits without notice.
