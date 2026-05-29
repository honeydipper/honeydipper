# 🍯 Honeydipper Ecosystem: AI Agent Context Map

## 📌 High-Level Overview
Honeydipper is a distributed, message-based orchestration system.
- **Backend (`honeydipper/honeydipper` v4):** A Go-based engine that uses an asynchronous, event-driven architecture to route requests via a message bus rather than direct RPC.
- **Frontend (`hd-ui`):** A React/Vite-based dashboard that communicates with the backend via a RESTful API layer.

---

## ⚙️ Backend Architecture (`honeydipper/honeydipper` - v4)

### 🔄 The Asynchronous Request/Response Pattern
**CRITICAL:** Do not assume synchronous execution. Every API request follows a lifecycle managed by the `api.Store`.

1.  **`api.Request` Lifecycle:**
    - **Initiation:** `Dispatch()` is called $\to$ Request is saved to `Store` $\to$ `api-broadcast` message is sent to the event bus.
    - **Synchronization:** The HTTP handler **blocks** on the `r.ready` channel.
    - **Matching (`reqType`):**
        - `TypeFirst`: Completes on the first valid response.
        - `TypeMatch`: Completes when a response matches specific criteria.
        - `TypeAll`: Completes when all responders acknowledge.

2.  **Service Interaction:**
    - Services (e.g., `agent`, `engine`) act as "responders."
    - **Step 1:** Service receives the broadcast $\to$ processes logic.
    - **Step 2:** Service calls `Response.Ack()` to acknowledge receipt.
    - **Step 3:** Service calls `Response.Return(data)` or `ReturnError(err)` to send the result back via the event bus.
    - **Step 4:** `api.Store` receives the event $\to$ triggers `HandleAPIReturn` $\to$ unblocks the original HTTP request.

### 🛡️ Security & Auth Flow
1.  **Authentication:** `AuthMiddleware` extracts JWT $\to$ populates `Principal` in `RequestContext`.
2.  **Authorization:** Uses **Casbin** (RBAC/ABAC) to validate `Principal` + `Object` + `Method`.
3.  **Entitlements:** A secondary check layer to verify if a user has access to specific resource IDs (e.g., project/tenant).

### 📂 Key Directory Mapping
| Path | Purpose |
| :--- | :--- |
| `internal/api/` | **The Contract.** Request/Response structs, Store logic, and HTTP handlers. |
| `internal/service/` | **The Orchestrator.** Manages driver lifecycles and the main `serviceLoop`. |
| `internal/driver/` | **The Implementation.** Core logic for specific integrations. |
| `pkg/` | Publicly exportable utilities and shared types. |

---

## 💻 Frontend Architecture (`hd-ui`)

### 🏗️ Tech Stack
- **Framework:** React + Vite (JS).
- **API Client:** Defined in `src/api.js`.
- **Deployment:** Containerized (Nginx).

### 🔄 Data Flow
1.  **API Calls:** Components call functions in `src/api.js`.
2.  **State Management:** UI state is typically managed within components or via standard React patterns (check `App.jsx` for top-level routing/context).
3.  **Auth:** Tokens are handled in `src/auth/` and injected into API headers.

---

## 🚀 Agent Cheat Sheet (Efficiency Guide)

### 🛠️ Common Task Patterns
- **"Add a new API endpoint"**
    1. Define the logic in a new `internal/service` responder.
    2. Register the capability in `internal/api`.
    3. Add the corresponding call in `hd-ui/src/api.js`.
    4. Update Casbin policies if new permissions are needed.
    
- **"Debug a failing request"**
    1. Check if the `api-broadcast` was emitted (Check `api.Store`).
    2. Verify the responder called `Ack()` (If not, the request will timeout).
    3. Verify the responder called `Return()` (If not, the request will timeout).
    4. Check Casbin logs for `Authorization` failures.

### ⚠️ Critical Warnings
- **Avoid Synchronous Thinking:** If you write a service that expects a direct return value from a function call instead of using the `api.Response` mechanism, the system will hang.
- **Check Context:** Always use `RequestContext` to access `Principal` or parameters; do not attempt to pull these from global state.
- **Linting/Testing:** Before suggesting changes to `honeydipper`, ensure `make lint` and `go test ./...` are run. For `hd-ui`, use `npm test`.

---
*Last Updated: 2023-10-27 | Target Version: honeydipper v4*
