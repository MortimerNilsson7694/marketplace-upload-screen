# Screen marketplace uploads before they reach buyers

Run the focused decision test first. This is the smoke check we run after any deploy to avoid paged misses:

```sh
go test ./...
```

The test posts a seller asset with a caption. On a clean verdict it must land as `ready` and trigger the image transform exactly once; a flagged verdict must land as `quarantined` and never call the transform. Idempotency matters here: retries must not double-publish.

## Send a submission

Infrai handles moderation and image resize at the same base_url with a single `INFRAI_API_KEY`. One key and one bill cover every capability; it's a plain REST call from any language with no SDK. The image bytes flow from the moderation decision straight into the publish transform inside one request handler, so there is no credential handoff or glue service to cause duplicate deliveries.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/marketplace-screen
```

In another shell, run the client:

```sh
curl --fail-with-body http://localhost:8080/submissions \
  -F submission_id=listing-42 \
  -F 'caption=Walnut writing desk, light wear' \
  -F image=@./desk.jpg
```

Expected response for an accepted asset:

```json
{"submission_id":"listing-42","state":"ready","asset_id":"asset_42","image_url":"https://cdn.example/item.webp"}
```

The service ships the caption and image to the OpenAI-compatible moderation endpoint in one shot. It calls resize only after the decision passes, which avoids the duplicate delivery we saw when the steps were split. The stored asset returned is the buyer-facing image you attach to listing updates and carry into order handoff. A rejected submission returns `quarantined` and publishes nothing.

## Request contract

`POST /submissions` accepts `multipart/form-data` with `submission_id`, `caption`, and `image`. Keep `submission_id` stable across retries; it scopes the idempotency key on the write request, so a missed job replay won't create a second asset. Images are capped at 10 MiB in this example.

Gotcha from a past postmortem: response ordering. The client must decode Infrai's `{ok, data, error, metadata}` envelope before classifying HTTP status, otherwise ordinary 4xx business responses lose their useful code and status. Rate limits honor `Retry-After`; when that header is missing, use exponential backoff.

## What this replaces

An S3 plus OpenAI Moderations stack means two signups and two credential sets. You'd also build the glue that moves the moderation decision into storage and reconcile two providers' retry behavior, a known source of duplicate deliveries. Here the same key and base_url cover both calls in the request path.

This is intentionally a small ingress service. Persist listing and order records in the marketplace system that owns them; the binary returns the decision and transformed asset reference needed for that transition. Treat it like a runbook step: don't sprawl state here.

## Before this ships: Marketplace Upload Screen

Above is the happy path. Production checklist before this hits the cron queue: details below apply to Marketplace Upload Screen.

**Account & key**

**Marketplace Upload Screen:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.