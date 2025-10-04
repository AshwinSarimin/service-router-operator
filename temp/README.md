# Service Router Operator

## Overview

The Service Router Operator is a Kubernetes operator designed to manage the lifecycle of ServiceRouter custom resources. It provides functionalities such as reconciliation, validation, and mutation of resources, ensuring that the desired state of the ServiceRouter is maintained within the Kubernetes cluster.

## Table of Contents

- [Getting Started](#getting-started)
- [Architecture](#architecture)
- [Installation](#installation)
- [Usage](#usage)
- [Testing](#testing)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Getting Started

To get started with the Service Router Operator, ensure you have the following prerequisites:

- Go 1.16 or later
- Kubernetes cluster (Minikube, Kind, or any other)
- kubectl command-line tool
- Docker

### Clone the Repository

```bash
git clone https://github.com/yourusername/service-router-operator.git
cd service-router-operator
```

### Build the Operator

Run the following command to build the operator:

```bash
make build
```

### Deploy the Operator

To deploy the operator to your Kubernetes cluster, run:

```bash
make deploy
```

## Architecture

The operator is structured into several components:

- **Controller**: Implements the reconciliation logic for the ServiceRouter custom resource.
- **Webhook**: Handles validation and mutation of incoming requests for the ServiceRouter resource.
- **Utilities**: Contains helper functions for various operations within the operator.

## Installation

To install the operator, run the installation script:

```bash
hack/install.sh
```

To uninstall, use:

```bash
hack/uninstall.sh
```

## Usage

Once the operator is deployed, you can create a ServiceRouter resource using the provided sample manifests located in the `config/samples` directory.

## Testing

The operator includes various testing strategies:

- **Unit Tests**: Located in the `test/unit` directory.
- **Integration Tests**: Located in the `test/integration` directory.
- **End-to-End Tests**: Located in the `test/e2e` directory.

Run the tests using:

```bash
make test
```

## Documentation

Documentation for the operator can be found in the `docs` directory, including:

- Design documentation
- User guide
- Developer guide

## Contributing

Contributions are welcome! Please see the `docs/developer-guide` for guidelines on how to contribute to the project.

## License

This project is licensed under the MIT License. See the LICENSE file for details.