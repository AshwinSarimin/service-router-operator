# GitHub Copilot Instructions for Service Router Operator

## Go-Specific Coding Standards
- **Error Handling**: Always handle errors explicitly. Use `errors.Wrap` from the `github.com/pkg/errors` package for wrapping errors with context.
- **Context Usage**: Pass `context.Context` through function calls to manage timeouts and cancellations effectively. Always check for cancellation in long-running operations.

## Kubernetes Resource Handling Patterns
- Use the controller-runtime library for managing Kubernetes resources.
- Implement the reconcile loop correctly, ensuring idempotency in resource handling.
- Use finalizers for cleanup logic when deleting resources.

## Testing Requirements
- **Unit Tests**: Write unit tests for all public functions using the `testing` package. Use mocks for dependencies.
- **Integration Tests**: Set up integration tests that interact with a real Kubernetes cluster using `envtest` or a similar framework.
- **End-to-End Tests**: Create e2e tests that validate the entire operator functionality in a real cluster environment.

## Documentation Standards
- Maintain clear and concise documentation in the `docs` directory.
- Use Markdown format for all documentation files.
- Ensure that the README.md provides a comprehensive overview, including setup instructions, usage examples, and contribution guidelines.

## Logging and Observability Practices
- Use structured logging with `logr` or `zap` for better observability.
- Ensure logs contain sufficient context to trace issues effectively.
- Implement metrics collection using Prometheus to monitor operator performance.

## CI/CD Pipeline Requirements
- Define CI workflows in `.github/workflows/ci.yml` to run tests and linting on pull requests.
- Set up a release workflow in `.github/workflows/release.yml` to automate versioning and deployment to Azure Container Registry (ACR).
- Include security checks in `.github/workflows/security.yml` to scan for vulnerabilities in dependencies.

## Code Review Criteria
- Ensure all code changes include appropriate tests.
- Check for adherence to Go coding standards and best practices.
- Verify that documentation is updated to reflect any changes in functionality.
- Ensure that the code is free of security vulnerabilities and follows best practices for security.