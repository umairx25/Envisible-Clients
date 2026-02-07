# envis

[![PyPI - Version](https://img.shields.io/pypi/v/envis.svg)](https://pypi.org/project/envis)
[![PyPI - Python Version](https://img.shields.io/pypi/pyversions/envis.svg)](https://pypi.org/project/envis)

-----

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
- [License](#license)
- [Contact](#contact)

## Overview
Envisible is a simple, secure secret management platform. The sdk allows users to integrate their secrets (configured [on our web dashboard](https://envisible.dev)) directly into their code.

## Installation

```console
pip install envis
```

## Usage

1. Sign up for an Envisible account on the dashboard.  
2. Install the SDK and authenticate when prompted (the CLI will open a secure browser window).  
3. Read and manage secrets from any Python runtime:

### Fetching a secret

```python
import envis

SECRET = envis.get(project_id="project_id", secret_name="secret_name")

print(SECRET)
```

### Logging out

```python
import envis

envis.logout()
```


## License

`envis` is released under a private source-available license: you can use the SDK to integrate with [our web dashboard](https://envisible.dev), but the backend platform remains proprietary.


## Contact
- [Website](https://envisible.dev)
- [Email](mailto:contact@uarham.me)
