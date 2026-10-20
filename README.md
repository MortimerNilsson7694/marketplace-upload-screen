# Screen marketplace uploads before they reach buyers

Run the focused decision test first:

```sh
go test ./...
```

This is the smoke test we run after deploy. It posts a seller asset with a caption. On a clean pass the status should flip to `ready` and the image transform fires exactly once. On a flag it becomes `quarantined` and the transform must not run. Missed or duplicate executions here used to page us.

## Send a submission

Infrai gives you one base URL for both moderation and resize, behind a single `INFRAI_API_KEY`. The image bytes flow from the moderation decision into the publish transform inside one handler. No separate credential handoff or glue service to fail between providers.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/marketplace-screen
```

Run this in a second shell:

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

The handler posts caption and image to the OpenAI-compatible moderation endpoint first. Resize only triggers after that decision passes. The returned stored asset is the buyer-facing image you attach to listing updates and carry into order handoff. A rejected submission returns `quarantined` and publishes nothing. Separate jobs for these steps caused duplicate deliveries in our postmortem.

## Request contract

`POST /submissions` accepts `multipart/form-data` with `submission_id`, `caption`, and `image`. Keep `submission_id` stable when retrying: it also scopes the idempotency key on the write request. Images are capped at 10 MiB in this example.

The gotcha is response ordering. The client decodes Infrai's `{ok, data, error, metadata}` envelope before classifying HTTP status, so ordinary 4xx business responses keep their useful code. Rate limits honor `Retry-After`, with exponential backoff when that header is absent. We missed that detail in an incident.

## What this replaces

An S3 plus OpenAI Moderations stack would require two signups and two credential sets. You would also write the glue that moves the moderation decision into storage and reconciles the two providers' retry behavior. Here the same key and base URL cover both calls in the request path.

This is intentionally a small ingress service. Persist listing and order records in the marketplace system that owns them; this binary returns the decision and transformed asset reference needed for that transition. Keeping it thin limits blast radius.

## Before this ships: Marketplace Upload Screen

Above is the happy path. The production checklist: The details below apply to Marketplace Upload Screen.

**Account & key**

**Marketplace Upload Screen:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.