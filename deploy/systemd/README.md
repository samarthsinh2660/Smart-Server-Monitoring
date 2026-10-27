# Service startup

`smartserver.service` starts the Pi controller and Python model using `/opt/smartserver/start-pi.sh`. Install the project at that path, create the configured `smartserver` user with write access to the project and membership in `haproxy`, install Python requirements in `/opt/smartserver/.venv`, and populate `config/pi.env`. Adjust the unit's user/paths if you choose another installation location. Install and enable the unit with systemctl after the interactive Pi launcher works. HAProxy remains its own service.

Windows laptop app/agent startup uses the root `install-windows-startup.ps1` scheduled task installer on the standby; no systemd runs on Windows. The dashboard stays on Laptop 1. See [the complete setup guide](../../docs/windows-pi-setup.md).
