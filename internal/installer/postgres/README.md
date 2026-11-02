# PostgreSQL migration access

For Kubernetes-managed PostgreSQL, local component migrations run outside the
cluster. The platform installation phase creates a temporary NodePort Service
using the selector and PostgreSQL target port of CNPG's `masterdata-rw` Service.
Kubernetes allocates the node port; OMS uses a node's internal IP, falling back
to its external IP. The installer must be able to reach that address and port.
The temporary Service is deleted when the platform phase returns, including
on failure or cancellation. A process kill can leave it behind; these Services
have names beginning with `oms-masterdata-nodeport-`.

Only the temporary installation config receives this migration endpoint.
Application connections continue to use the in-cluster read-write Service.
Migrations retain PostgreSQL TLS and verify the server certificate against
`masterdata-rw.codesphere.svc.cluster.local`, rather than the node IP.

## Why use a NodePort instead of port-forward?

The client-go port-forward implementation can close the entire forwarding
session when just one PostgreSQL connection ends. PostgreSQL TLS session
shutdown can produce a TCP reset when the backend exits with the client's TLS
`close_notify` still unread. This does not require a pod or database restart.
The runtime reports the reset through that connection's error stream, and
client-go closes the shared session and local listener. Subsequent migration
connections then fail with `ECONNREFUSED`; any other connections sharing the
tunnel can also be interrupted.

This behavior is documented in:

- [kubectl #1169](https://github.com/kubernetes/kubectl/issues/1169): reports of
  PostgreSQL TLS disconnects closing forwarding, with disabling SSL and
  restarting forwarding suggested as workarounds.
- [Kubernetes #113568](https://github.com/kubernetes/kubernetes/issues/113568):
  explains how a nonfatal connection reset becomes fatal to the tunnel and
  proposes structured runtime errors rather than matching error strings.
- [CloudNativePG #8767](https://github.com/cloudnative-pg/cloudnative-pg/issues/8767):
  reproduces the failure by exiting `psql`, including interruption of a second
  connection; recommends direct access from another pod as an alternative.
- [kubectl #1746](https://github.com/kubernetes/kubectl/issues/1746): follow-up
  investigation describes the difference between containerd 1.7's reset
  handling and the streaming implementation used by containerd 2.x. It also
  reports that switching between WebSockets and SPDY does not resolve it and
  proposes handling resets in the runtime without terminating the tunnel.
- [Kubernetes PR #143050](https://github.com/kubernetes/kubernetes/pull/143050):
  proposes keeping the client-side tunnel alive after per-connection errors.
  This was still open when NodePort access was restored on 2026-11-02.

NodePort access avoids this forwarding lifecycle entirely and lets PostgreSQL
connections close independently. A background port-forward reconnect loop
would leave a gap before the listener is available again and would not preserve
other active connections. Disabling PostgreSQL TLS is a reported workaround,
but the NodePort setup keeps certificate verification enabled.
