# Pi reverse proxy

HAProxy configuration for the Raspberry Pi belongs here. The Pi provides one client-facing address. The primary is initially enabled; the standby is enabled only after its application passes `GET /health`. A failed target must be removed from traffic routing.
