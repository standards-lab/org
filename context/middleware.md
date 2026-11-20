# The sourced middleware set

v1.middleware adopts CORS, real client IP, and compression as declared dependencies
(`go-web-sdk/context/middleware-sourcing.md` holds the sourcing convention). Rate limiting has
landed as `go-web-sdk/middleware/rate-limit`.

## Rate limiting's next release

rate-limit's main carries untagged go-core v0.6.0, httprate v0.16.1, and the go-web-sdk v0.14.0
requirement. Its next release tags them and can move to go-web-sdk v0.15.0.

## CORS

CORS's trigger is the first client served cross-origin, such as an external client or a
Pages-hosted page that calls a service. v1.client.reference's embedded client is same-origin and
sends no cross-origin requests (`web-client.md`). The trigger has fired: spike-html-apps' external
client calls two services. It needed:

- one allowed-origins list shared by the CORS middleware and the NATS WebSocket listener;
- Authorization and Content-Type allowed;
- a preflight on every upload sent with a progress listener;
- the caller naming the methods.

rs/cors reads an empty list as admitting every origin, which the spike's wrapper turns into a
pass-through. rs/cors's latest tag is v1.11.1 while its main stays active, with #220 (credentialed
wildcard) and #222 (malformed origins) open. jub0bs/cors is the alternative to weigh, and herald's
CORS middleware (`HERALD_CORS_*`) is prior art.

## Real client IP and compression

Neither has a live consumer, so each waits for one. Real IP's only other use, keying rate
limiting, is covered by `RemoteAddr`. Compression has no large-response case until more of the
data layer lands.
