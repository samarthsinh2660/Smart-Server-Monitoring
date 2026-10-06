# Laptop application deployment

Docker Compose files for the same stateless demo application on both laptops belong here. Configure the standby so its application starts automatically after WoL and boot. Each app instance must expose `GET /health` and identify which laptop served a request.
