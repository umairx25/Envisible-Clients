# envis-cli

-----

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
- [License](#license)
- [Contact](#contact)

## Overview
Envisible is a simple, secure secret management platform. The CLI provides a terminal-first workflow to manage projects and secrets configured in [our web dashboard](https://envisible.netlify.app).

## Installation

### Install via curl (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible-Clients/main/cli/install.sh | bash
```

By default, this installs to `~/.local/bin`. You can override with `ENVIS_INSTALL_DIR`:

```bash
ENVIS_INSTALL_DIR=/usr/local/bin \
  curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible-Clients/main/cli/install.sh | bash
```

### Build from source

```bash
cd cli
go build -o envis .
```

Optionally move the binary into your PATH:

```bash
mv ./envis /usr/local/bin/envis
```

## Usage

1. Sign up for an Envisible account on the dashboard.
2. Install the CLI and authenticate when prompted.
3. Manage projects and secrets from your terminal. In interactive terminals, `envis` prompts for missing projects, names, files, and confirmations. In CI or agent workflows, keep using flags and environment variables.

### Log in

```bash
./envis login
```

### List projects

```bash
./envis projects
```

This shows project names and marks the current default with `*`.

### Fetch a secret

```bash
./envis secret-get
```

### Pull all secrets to a file

```bash
./envis pull
```

By default, this adds any missing variable names to `.env.example`. To opt out of updating `.env.example`:

```bash
./envis pull --no-env-example
```

### Set current project

```bash
./envis project-set
```

### Non-interactive usage

```bash
ENVIS_CI_TOKEN=<token> ENVIS_PROJECT_ID=<uuid> ./envis pull --output .env
ENVIS_CI_TOKEN=<token> ENVIS_PROJECT_ID=<uuid> ./envis secret-get --name API_KEY
```

When `ENVIS_CI_TOKEN` is set, the CLI never prompts. Missing required inputs must be supplied with flags or environment variables.

### Log out

```bash
./envis logout
```

## License

`envis-cli` is released under a private source-available license: you can use the CLI to integrate with [our web dashboard](https://envisible.netlify.app), but the backend platform remains proprietary.

## Contact
- [Website](https://envisible.netlify.app)
- [Email](mailto:contact@uarham.me)
