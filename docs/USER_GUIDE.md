# User Guide

## 1. Concepts

| Term | Meaning |
|------|---------|
| Connection | Credentials plus base URL for one upstream system |
| Integration | A named set of endpoints on a connection, plus mappings |
| Endpoint | One method + path template on an integration |
| Execution | One call of an endpoint, with its request and response |
| Schedule | A cron rule that queues an execution |
| Mapping | Rule copying a source field of the response to a target field |

## 2. First connection

```bash
curl -s -X POST localhost:8080/api/v1/connections \
  -H 'X-API-Key: <admin key>' -H 'Content-Type: application/json' \
  -d '{"name":"GitHub","type":"rest","auth_type":"api_key",
       "config":{"base_url":"https://api.github.com"},
       "credentials":{"api_key":"ghp_xxx"}}'
```

Test it before saving an integration:

```bash
curl -s -X POST localhost:8080/api/v1/connections/<id>/test -H 'X-API-Key: <key>'
```

## 3. First integration

```bash
curl -s -X POST localhost:8080/api/v1/integrations \
  -H 'X-API-Key: <key>' -H 'Content-Type: application/json' \
  -d '{"name":"Issues","type":"rest","connection_id":"<conn>",
       "endpoints":[{"name":"list","method":"GET","path":"/repos/o/r/issues"}]}'
```

Run it synchronously, or with `"async": true` to get `202` and a pollable id:

```bash
curl -s -X POST localhost:8080/api/v1/integrations/<id>/execute \
  -H 'X-API-Key: <key>' -H 'Content-Type: application/json' \
  -d '{"endpoint":"list"}'
```

## 4. Normalizing responses

Add mappings so downstream consumers see a stable shape:

```json
"mappings": [
  { "source": "data.id",   "target": "id",     "transform": "to_int" },
  { "source": "data.name", "target": "title",  "transform": "trim" },
  { "source": "missing",   "target": "region", "transform": "default:eu" }
]
```

Supported transforms: `none`, `upper`, `lower`, `trim`, `to_int`, `to_string`,
`to_bool`, `default:<value>`, `template:<text>`. Sources use dotted paths with
array indexes (`items.0.sku`).

Preview before saving:

```bash
curl -s -X POST localhost:8080/api/v1/integrations/<id>/mapping/preview \
  -H 'X-API-Key: <key>' -H 'Content-Type: application/json' \
  -d '{"payload":{"data":{"id":"7","name":"  Acme  "}}}'
```

For logic that mappings cannot express, set `"transform": {"type":"lua",
"script":"out = { total = payload.count * 2 }"}`. The script reads the global
`payload` and writes the global `out`.

## 5. Scheduling

```bash
curl -s -X POST localhost:8080/api/v1/schedules \
  -H 'X-API-Key: <key>' -H 'Content-Type: application/json' \
  -d '{"name":"nightly","integration_id":"<int>","endpoint":"list","cron":"0 3 * * *"}'
```

The scheduler ticks once a minute by default (`INTEGRATIONHUB_SCHED_TICK_MS`).

## 6. Receiving webhooks

Point the sender at `POST /api/v1/webhooks/<integrationId>/<endpoint>` and store
the shared secret in the connection's `config.webhook_secret`. Requests are
verified against `webhook-signature: t=<unix>,v1=<hex>`; send `webhook-id` to
make retries idempotent.

## 7. Inspecting and replaying

- `GET /api/v1/executions?integration_id=<id>` — history
- `GET /api/v1/executions/dead` — failed for good
- `POST /api/v1/executions/<id>/replay` — queue it again
- `GET /api/v1/data/schema/<integrationId>` — inferred response schema
- `GET /api/v1/data/export?integration_id=<id>&format=csv` — download responses

## 8. Errors you will see

| Code | Meaning |
|------|---------|
| 400 | Validation failed, unknown endpoint, bad cron |
| 401 | Missing/invalid credentials or webhook signature |
| 403 | Authenticated but the role lacks the resource/action |
| 404 | Unknown connection, integration, execution or schedule |
| 413 | Body over 1 MiB |
| 415 | Content type is not JSON |
| 422 | Transform (template/Lua) failed |
| 429 | Rate limited; retry after `Retry-After` seconds |