# Proxy control

`Runtime` reads HAProxy CSV stats and changes `demo_servers/primary` and `demo_servers/standby` using a local Unix admin socket. Only the fixed target names and `ready`, `maint`, and `drain` states are accepted. Commands have deadlines; error responses are not treated as success. The controller first requires a direct app health check and then actual proxy readiness before reporting standby active. Reset drains and waits for the socket's session count to reach zero before disabling standby. The socket stays local to the Pi and restricted by Unix permissions.
