# envis-node

-----

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
- [License](#license)
- [Contact](#contact)

## Overview
Envisible is a simple, secure secret management platform. The SDK allows users to integrate their secrets (configured [on our web dashboard](https://envisible.netlify.app)) directly into their Node.js code.

## Installation

```console
cd node-sdk
npm install
```

## Usage

1. Sign up for an Envisible account on the dashboard.
2. Install the SDK and authenticate when prompted (the SDK will open a secure browser window).
3. Read and manage secrets from any Node.js runtime:

### Fetching a secret

```javascript
const envis = require("envis-node");

async function main() {
  const secret = await envis.get("project_id", "secret_name");
  console.log(secret);
}

main();
```

### Logging out

```javascript
const envis = require("envis-node");

envis.logout();
```

## License

`envis-node` is released under a private source-available license: you can use the SDK to integrate with [our web dashboard](https://envisible.netlify.app), but the backend platform remains proprietary.

## Contact
- [Website](https://envisible.netlify.app)
- [Email](mailto:contact@uarham.me)
