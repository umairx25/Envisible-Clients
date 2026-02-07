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
2. Build the CLI and authenticate when prompted.
3. Manage projects and secrets from your terminal:

### Log in

```bash
./envis login
```

### List projects

```bash
./envis projects
```

### Fetch a secret

```bash
./envis secret-get --project-id <uuid> --name API_KEY
```

### Pull all secrets to a file

```bash
./envis pull --project-id <uuid> --output .env
```

### Log out

```bash
./envis logout
```

## License

`envis-cli` is released under a private source-available license: you can use the CLI to integrate with [our web dashboard](https://envisible.netlify.app), but the backend platform remains proprietary.

## Contact
- [Website](https://envisible.netlify.app)
- [Email](mailto:contact@uarham.me)
