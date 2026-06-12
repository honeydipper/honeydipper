# FAQ

## General questions

### What is Honeydipper?
Honeydipper is an event-driven, policy-based orchestration system that is tailored towards SREs and DevOps workflows, and has a pluggable open architecture. The purpose is to fill the gap between the various components used in DevOps operations, to act as an orchestration hub, and to replace the ad-hoc integrations between the components so that all the integrations can also be composed as code.

### What is DipperCL?
DipperCL stands for Dipper Control Language. It is a yaml based language used for configuring Honeydipper, and defining assets within Honeydipper.

### What are the assets used in Honeydipper?
Honeydipper uses 4 type of assets. They are driver, system, workflow and rules.

### What is a driver?
Honeydipper driver is a golang program that is dynamically loaded by the daemon to perform various tasks or ingest events. For example, webhook driver, web driver, kubernetes driver, etc.

### What is a system?
A system is an abstract representation of a physical system in Honeydipper. It is defined by a group triggers and functions through which the system interacts with Honeydipper and the world. For example, github system has a group of triggers implemented through webhook driver, and a group of functions that calls github APIs through web the driver.

### What is a workflow?
A workflow is a data structure that defines the work needed to complete a task. It can contains steps, threads or invoking functions, other workflows. It can also contain definition of context variables, and evaluate conditions before taking actions.

### What is a rule?
A rule defines a trigger, a set of conditions and a workflow indicating that the workflow needs to be executed when the trigger is fired and the conditions are met.

## Driver Types

### What are the different types of drivers?
Honeydipper supports three types of drivers:
1. **Builtin**: A driver that is compiled into the daemon or available as a local binary. It runs as a child process of the daemon.
2. **Remote**: A driver that is downloaded from a registry or URL at runtime, verified for integrity, and cached locally before execution.
3. **Null**: A no-op driver used for testing or as a placeholder. It does not perform any actions.

### How do I configure a remote driver?
See the [Remote Driver And Registry Guide](./remote_driver_registry.md) for detailed instructions on configuring remote drivers, registries, and source policies.

### Can I use a local binary as a driver?
Yes, you can use a local binary as a driver by configuring it as a `remote` driver with `localPath` in the `handlerData`. You will also need to enable the `local` source policy.

## v4 Features

### What are the new features in Honeydipper v4?
Honeydipper v4 introduces several new features:
- **AI Agents**: Integration with AI models (OpenAI, Gemini, Ollama) for intelligent workflow execution, tool calls, and conversation management.
- **Multi-Service Architecture**: The daemon can run up to 5 services concurrently: Engine, Receiver, Operator, API, and Agent.
- **HTTP REST API**: A new Gin-based HTTP API for managing events, agent conversations, secrets, and user profiles.
- **Enhanced Workflow Engine**: Support for loops, conditions, caching, sub-workflows, and error handling.
- **MCP Server Integration**: Support for Model Context Protocol (MCP) servers to extend AI agent capabilities.

### What is an Agent in Honeydipper?
An Agent is an AI-powered component that can execute workflows, call tools, and engage in conversations. Agents can be configured with different AI models (OpenAI, Gemini, Ollama), system prompts, and tool sets. They support multi-turn conversations, streaming responses, and sub-agent delegation.

### How do I configure an Agent?
Agents are configured under the `agents` section in your configuration. Each agent has a name, driver (AI model), system prompt, and optional tools. See the [Configuration Guide](./configuration.md) for detailed examples.

### What is the Honeydipper API?
The Honeydipper API is an HTTP REST API that allows you to interact with the daemon programmatically. It supports endpoints for managing events, agent conversations, secrets, and user profiles. The API uses JWT authentication and supports SAML and GitHub OAuth for user authentication.

### How do I authenticate with the Honeydipper API?
The API supports multiple authentication methods:
1. **JWT Tokens**: Use `Authorization: Bearer <JWT>` header with a valid JWT token.
2. **SAML**: Use the `/auth/saml/*` endpoints for SAML-based authentication.
3. **GitHub OAuth**: Use the `/auth/github/callback` endpoint for GitHub OAuth.

## Common Operations

### How do I start Honeydipper?
Honeydipper can be started by running the daemon binary with the appropriate configuration. The daemon will load the configuration, initialize the services, and start processing events.

```bash
honeydipper -c /path/to/config.yaml
```

### How do I reload the configuration?
Honeydipper automatically reloads the configuration every 60 seconds by checking the configured git repositories for changes. You can also trigger a reload by sending a `broadcast:reload` message through the event bus.

### How do I check if my configuration is correct?
Use the `configcheck` mode to validate your configuration without starting the daemon:

```bash
honeydipper -configcheck -c /path/to/config.yaml
```

### How do I run a workflow manually?
You can trigger a workflow manually by sending an event through the API or by using the `job` mode to run a workflow once and exit:

```bash
honeydipper -job -c /path/to/config.yaml
```

### How do I use secrets in my configuration?
Honeydipper supports encrypted secrets using the `ENC[...]` prefix and secret lookups using the `LOOKUP[...]` prefix. See the [Interpolation Guide](./interpolation.md) for detailed examples.

## Troubleshooting

### Why is my driver not starting?
Common reasons for driver startup failures:
1. **Incorrect driver type**: Ensure the driver type (`builtin`, `remote`, `null`) is correctly configured.
2. **Missing binary**: For builtin drivers, ensure the binary exists in the expected path.
3. **Source policy**: For remote drivers, ensure the source policy allows the configured source type (`registry`, `direct`, `local`).
4. **Signature verification**: If signature verification is enabled, ensure the public key and signature are correct.
5. **Package dependencies**: For remote drivers with `requiredPackages`, ensure the required packages are defined for your package manager.

### How do I debug workflow execution?
You can debug workflow execution by:
1. Enabling verbose logging in the daemon.
2. Using the `export` feature to inspect workflow context data.
3. Checking the session state in the cache (if using Redis).

### What should I do if the daemon crashes on configuration reload?
Honeydipper has a rollback mechanism that restores the last known good configuration if a reload fails. Check the logs for errors and validate your configuration using `configcheck` mode.

### How do I report a bug or request a feature?
Please open an issue on the [Honeydipper GitHub repository](https://github.com/honeydipper/honeydipper/issues) with a detailed description of the bug or feature request.
