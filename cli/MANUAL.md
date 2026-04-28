# ENVIS-CLI

## Name
`envis` - Envisible CLI for managing projects and secrets.

## Synopsis
`envis <command> [options]`

Common usage:
- `envis login`
- `envis pull`
- `envis push`
- `envis secret-get`

## Description
`envis` is a terminal-first CLI for interacting with Envisible projects and secrets.
It prompts for missing input in normal terminal use and supports CI-token authentication for non-interactive workflows.

## Authentication
User auth (default):
- Run `envis login` and complete the device flow in your browser.
- Session is cached at `~/.envis/session.json`.

CI token auth:
- Set `ENVIS_CI_TOKEN` and use non-interactive commands (e.g. `pull`, `push`, `secret-get`, `secret-names`, `get-many`).
- Some admin commands require user auth and will error if `ENVIS_CI_TOKEN` is set.
- When `ENVIS_CI_TOKEN` is set, the CLI never prompts. Provide required values with flags or environment variables.

## Environment
`ENVIS_PROJECT_ID`
- Default project id used by commands that accept `--project-id`.
- Example: `export ENVIS_PROJECT_ID="<uuid>"` or `envis project-set --project-id <uuid>`

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
- `project-set`
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
- Show auth mode, session info, and the current project.
- Example: `envis status`

## pull
- Download all secrets for a project and write a dotenv file.
- Prompts for project selection and output file when needed.
- Options: `--project-id <uuid>`, `--output <path>` (default `.env`), `--no-env-example`
- Populates `.env.example` with any missing variable names unless `--no-env-example` is set.
- Example: `envis pull`
- Non-interactive example: `ENVIS_CI_TOKEN=<token> ENVIS_PROJECT_ID=<uuid> envis pull --output .env`

## push
- Upload all secrets from a dotenv file into a project.
- Prompts for project selection, source file, and confirmation when needed.
- Options: `--project-id <uuid>`, `--file <path>` (default `.env`)
- Example: `envis push`

## secret-names
- List secret names for a project.
- Options: `--project-id <uuid>`
- Example: `envis secret-names`

## secret-get
- Fetch a single secret and print `NAME=VALUE`.
- Prompts for project and secret name when needed.
- Options: `--project-id <uuid>`, `--name <key>`
- Example: `envis secret-get`

## secret-set
- Create or update a secret value.
- Prompts for project, secret name, and hidden value when needed.
- Options: `--project-id <uuid>`, `--name <key>`, `--value <value>`
- Example: `envis secret-set`

## secret-delete
- Delete a secret by name.
- Prompts for confirmation before deleting.
- Options: `--project-id <uuid>`, `--name <key>`
- Example: `envis secret-delete`

## get-many
- Fetch multiple secrets and print `NAME=VALUE` per line.
- Prompts for project and secret names when needed.
- Options: `--project-id <uuid>` followed by secret names.
- Example: `envis get-many`

## projects
- List projects available to the current user.
- Shows project names, roles, CI-token status, and marks the default project with `*`.
- Example: `envis projects`

## project-set
- Set the default project id for this machine.
- Options: `--project-id <uuid>`
- Example: `envis project-set`

## project-create
- Create a new project.
- Prompts to set it as the default project.
- Options: `--name <name>`
- Example: `envis project-create`

## project-rename
- Rename a project.
- Options: `--project-id <uuid>`, `--name <new-name>`
- Example: `envis project-rename`

## project-delete
- Delete a project.
- Requires typing the project name before deleting in interactive mode.
- Options: `--project-id <uuid>`
- Example: `envis project-delete`

## project-members
- List members of a project.
- Options: `--project-id <uuid>`
- Example: `envis project-members`

## project-member-remove
- Remove a member from a project.
- Prompts for member selection and confirmation when needed.
- Options: `--project-id <uuid>`, `--user-id <uuid>`
- Example: `envis project-member-remove`

## invites
- List pending invites for the current user by project and sender.
- Example: `envis invites`

## invite-respond
- Accept or reject an invite.
- Options: `--invite-id <uuid>` and exactly one of `--accept` or `--reject`
- Example: `envis invite-respond`

## invite-create
- Invite a user to a project (owner only).
- Options: `--project-id <uuid>`, `--email <user@example.com>`
- Example: `envis invite-create`

## ci-token-generate
- Generate a CI token for a project (owner only).
- Options: `--project-id <uuid>`
- Example: `envis ci-token-generate`

## ci-token-reset
- Reset (rotate) a CI token for a project (owner only).
- Prompts for confirmation before resetting.
- Options: `--project-id <uuid>`
- Example: `envis ci-token-reset`

## ci-token-verify
- Verify a CI token.
- Prompts for a hidden token when needed.
- Options: `--project-id <uuid>`, `--token <token>`
- Example: `envis ci-token-verify`

## help
- Print the commands index or command-specific help.
- Example: `envis help`
- Example: `envis help pull`

## man
- Print this manual page.
- Example: `envis man`

## Notes
- Project commands use `--project-id`, `ENVIS_PROJECT_ID`, or the default project from `envis project-set` before prompting.
- If only one project is available, commands auto-select it.
- In non-interactive mode, missing inputs are errors instead of prompts.
- Commands that modify user/account state require user auth and will not run with `ENVIS_CI_TOKEN`.
