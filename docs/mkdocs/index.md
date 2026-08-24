# Welcome to Wobsongo

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![DPG Candidate](https://img.shields.io/badge/DPG-Candidate-green)](https://digitalpublicgoods.net/)

This documentation provides a comprehensive overview of Wobsongo, an open-source misinformation detection tool. It covers the core architecture, the document processing pipeline, and local development practices.

## Project Overview & Features

Wobsongo is an open-source backend engine for misinformation detection, powered by a knowledge management and hybrid retrieval system. It provides infrastructure to ingest content, build a structured knowledge base, and verify claims against it.

- **REST API**: Written in Go using the [Echo](https://echo.labstack.com/) framework, providing a high-performance, low-latency API.
- **Async Job Queue**: Powered by [River](https://riverqueue.com/) on PostgreSQL for reliable processing of long-running background tasks.
- **Document Storage**: PostgreSQL for structured data with S3-compatible object storage (MinIO locally) for documents and media assets.
- **Extensible Architecture**: Clean layered design (handlers → services → repositories) with interface-based abstractions, making it straightforward to add new storage backends, job workers, or verification modules.

## SDG Alignment

This project contributes to:

- **SDG 3 (Good Health & Well-being)**: By providing infrastructure to combat health misinformation.
- **SDG 16 (Peace, Justice, Strong Institutions)**: By enabling transparent fact-checking of public information.

## License

Distributed under the Apache 2.0 License. See `LICENSE` for more information.

---

## Important Links

- [GitHub Repository](https://github.com/kairosedubf/wobsongo)

## Local Development

### Prerequisites

- [Jetify's Devbox](https://www.jetify.com/docs/devbox/quickstart/) (ensure it's installed on your system).

Devbox uses the Nix package manager under the hood to provide reproducible development environments. 

By running `devbox shell`, it will automatically download and set up the required versions of the tools specified in the `devbox.json` file, such as:

- Go
- Node.js and pnpm
- Make, PostgreSQL, Air, Swag, etc.

> Note that subsequent commands assume you are inside the `devbox shell`.

### Setup

1.  **Clone the repository**:

    ```bash
    git clone git@github.com:kairosedubf/wobsongo.git
    cd wobsongo
    ```

2.  **Activate Devbox environment**:

    ```bash
    devbox shell
    ```

    You should see your shell prompt change, indicating you are now inside the devbox environment:

    ```bash
    (devbox) ➜  wobsongo git:(main) _
    ```

3.  **Install backend dependencies (inside devbox shell)**:

    ```bash
    go mod download
    ```

    Optionally, run the linters:

    ```bash
    make check
    ```

    See `Makefile` for more details.

    You can run the unit tests with:

    ```bash
    make test-unit
    ```

    ...or the entire test suite with:

    ```bash
    make dbtestup test dbtestdown
    ```

### Running the Project

- **Run the backend (with hot-reloading)**:

  ```bash
  make dev
  ```

- You should see the server running:

```
Starting the server at localhost:8000
```