# Security Policy

## Reporting a vulnerability

Please do **not** open a public issue for security vulnerabilities.

Use the repository's **Security → Report a vulnerability** (GitHub private
advisory) flow. You can expect an acknowledgement within 72 hours and a fix
plan shortly after.

## Scope

The Go entrypoint, Dockerfile, and compose files in this repository. The
Project Zomboid server itself, SteamCMD, and DepotDownloader are
third-party binaries outside this project's control.

## Deployment hardening notes

- Run the container with the supplied `USER 1000` (never as root).
- Keep `data/`, `server-files/`, `backups/`, and `.env` out of version
  control (the repo `.gitignore` already excludes them).
- Do not publish the RCON port (`27015`) to the public internet; it speaks
  plaintext Source RCON. Restrict it to trusted networks if exposed.
- `SERVER_NAME`, `SANDBOX_*`, and `BACKUP_PATH` are validated at startup;
  any configuration error aborts the container with a clear message.
