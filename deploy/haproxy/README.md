# Pi reverse proxy

`haproxy.example.cfg` is a template for the Raspberry Pi. Replace the two `192.0.2.x` documentation addresses with the measured laptop LAN addresses before using it. The proxy listens on port `8088` and routes to the primary initially. The standby has the [HAProxy `disabled` server option](https://docs.haproxy.org/3.0/configuration.html), so it receives no requests until activation.

The runtime admin socket is local at `/run/haproxy/admin.sock`; create that directory and restrict socket access to the Go controller/operator on the Pi. To activate manually during Phase 3, first request `http://<standby-address>:8000/health` directly and confirm HTTP 200. Then issue `enable server demo_servers/standby` through the [HAProxy runtime API](https://docs.haproxy.org/3.0/management.html). HAProxy continues to check `/health` and excludes unhealthy targets. The later Go controller will perform this sequence automatically.

The addresses in this template are placeholders; no Pi or proxy integration has been measured yet.
