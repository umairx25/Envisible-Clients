# ENVIS-CLI

## Name
`envis` - Envisible CLI for managing projects and secrets.

## Synopsis
`envis <command> [options]`

Common usage:
- `envis login`
- `envis pull --project-id <uuid> --output .env`
- `envis push --project-id <uuid> --file .env`
- `envis secret-get --project-id <uuid> --name API_KEY`

## Description
`envis` is a terminal-first CLI for interacting with Envisible projects and secrets.
It supports user authentication (device login) and CI-token authentication for non-interactive workflows.

## Authentication
User auth (default):
- Run `envis login` and complete the device flow in your browser.
- Session is cached at `~/.envis/session.json`.

CI token auth:
- Set `ENVIS_CI_TOKEN` and use non-interactive commands (e.g. `pull`, `push`, `secret-get`, `secret-names`, `get-many`).
- Some admin commands require user auth and will error if `ENVIS_CI_TOKEN` is set.

## Environment
`ENVIS_PROJECT_ID`
- Default project id used by commands that accept `--project-id`.
- Example: `export ENVIS_PROJECT_ID="<uuid>"`

`ENVIS_CI_TOKEN`
- Use CI-token auth instead of user auth.

## Commands
Run `envis help <command>` for details.

- `login`
- `logout`
- `status`
- `pull`
- `push`
- `secret-names`
- `secret-get`
- `secret-set`
- `secret-delete`
- `get-many`
- `projects`
- `project-create`
- `project-rename`
- `project-delete`
- `project-members`
- `project-member-remove`
- `invites`
- `invite-respond`
- `invite-create`
- `ci-token-generate`
- `ci-token-reset`
- `ci-token-verify`
- `help`, `man`

## login
- Start device login and cache a session.
- Example: `envis login`

## logout
- Remove the cached session.
- Example: `envis logout`

## status
- Show auth mode and session info.
- Example: `envis status`

## pull
- Download all secrets for a project and write a dotenv file.
- Options: `--project-id <uuid>`, `--output <path>` (default `.env`), `--no-gitignore`
- By default, adds the output filename to a `.gitignore` in the same directory (creates one if missing).
- Example: `envis pull --project-id <uuid> --output .env`

## push
- Upload all secrets from a dotenv file into a project.
- Options: `--project-id <uuid>`, `--file <path>` (default `.env`)
- Example: `envis push --project-id <uuid> --file .env`

## secret-names
- List secret names for a project.
- Options: `--project-id <uuid>`
- Example: `envis secret-names --project-id <uuid>`

## secret-get
- Fetch a single secret and print `NAME=VALUE`.
- Options: `--project-id <uuid>`, `--name <key>`
- Example: `envis secret-get --project-id <uuid> --name API_KEY`

## secret-set
- Create or update a secret value.
- Options: `--project-id <uuid>`, `--name <key>`, `--value <value>`
- Example: `envis secret-set --project-id <uuid> --name API_KEY --value abc123`

## secret-delete
- Delete a secret by name.
- Options: `--project-id <uuid>`, `--name <key>`
- Example: `envis secret-delete --project-id <uuid> --name API_KEY`

## get-many
- Fetch multiple secrets and print `NAME=VALUE` per line.
- Options: `--project-id <uuid>` followed by secret names.
- Example: `envis get-many --project-id <uuid> API_KEY DB_URL`

## projects
- List projects available to the current user.
- Example: `envis projects`

## project-create
- Create a new project.
- Options: `--name <name>`
- Example: `envis project-create --name "My App"`

## project-rename
- Rename a project.
- Options: `--project-id <uuid>`, `--name <new-name>`
- Example: `envis project-rename --project-id <uuid> --name "New Name"`

## project-delete
- Delete a project.
- Options: `--project-id <uuid>`
- Example: `envis project-delete --project-id <uuid>`

## project-members
- List members of a project.
- Options: `--project-id <uuid>`
- Example: `envis project-members --project-id <uuid>`

## project-member-remove
- Remove a member from a project.
- Options: `--project-id <uuid>`, `--user-id <uuid>`
- Example: `envis project-member-remove --project-id <uuid> --user-id <uuid>`

## invites
- List pending invites for the current user.
- Example: `envis invites`

## invite-respond
- Accept or reject an invite.
- Options: `--invite-id <uuid>` and exactly one of `--accept` or `--reject`
- Example: `envis invite-respond --invite-id <uuid> --accept`

## invite-create
- Invite a user to a project (owner only).
- Options: `--project-id <uuid>`, `--email <user@example.com>`
- Example: `envis invite-create --project-id <uuid> --email user@example.com`

## ci-token-generate
- Generate a CI token for a project (owner only).
- Options: `--project-id <uuid>`
- Example: `envis ci-token-generate --project-id <uuid>`

## ci-token-reset
- Reset (rotate) a CI token for a project (owner only).
- Options: `--project-id <uuid>`
- Example: `envis ci-token-reset --project-id <uuid>`

## ci-token-verify
- Verify a CI token.
- Options: `--project-id <uuid>`, `--token <token>`
- Example: `envis ci-token-verify --project-id <uuid> --token <token>`

## help
- Print the commands index or command-specific help.
- Example: `envis help`
- Example: `envis help pull`

## man
- Print this manual page.
- Example: `envis man`

## Notes
- Commands requiring `--project-id` will use `ENVIS_PROJECT_ID` if set.
- Commands that modify user/account state require user auth and will not run with `ENVIS_CI_TOKEN`.
