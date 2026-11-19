# ARTEX scoped proxy patch

Runtime packages and upstream tests copied from go-mitmproxy v1.9.2 under its
included license. Local changes expose a destination resolver before both direct
and upstream-proxy dialing, reject the separate-client rewrite path when scoped,
and verify TLS against the CONNECT destination rather than client-supplied SNI.
Issued interception certificates also use that destination, including IP SANs.
Target-specific CA roots are supported without disabling verification. Active
error/stream/WebSocket logging omits request URLs containing credentials.
ARTEX's traffic package supplies a default-deny, DNS-pinned host:port policy.
Keep these changes when updating the dependency.
