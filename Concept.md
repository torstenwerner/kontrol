# Kontrol: Kubernetes Pod Monitoring TUI

## Project Overview

Kontrol is a lightweight, terminal-based user interface (TUI) tool designed for monitoring Kubernetes pods in real-time. Built in Go, it leverages the Kubernetes client-go library to interact with the Kubernetes API server, fetching and displaying pod information in a user-friendly, interactive format. The TUI is powered by the Bubble Tea framework (from Charm Bracelet), enabling a responsive and intuitive experience with keyboard navigation.

The primary goal is to provide a simple, focused tool for cluster administrators and developers to quickly view pod statuses across contexts and namespaces without leaving the terminal. It emphasizes ease of use, automatic refreshing, and visual cues for pod health, while persisting user preferences for contexts and namespaces across sessions.

Key technologies:
- **Language**: Go (targeting Go 1.21+ for modern features like generics and improved error handling).
- **Kubernetes Integration**: Use `k8s.io/client-go` for API interactions, loading configuration from the default kubeconfig file (typically `~/.kube/config`).
- **TUI Framework**: Bubble Tea for building the interactive UI, combined with Lip Gloss for styling (e.g., colors, borders).
- **Persistence**: Use a simple JSON file in the user's home directory (e.g., `~/.kontrol/config.json`) to store the last selected context and namespace.
- **Build and Distribution**: Cross-compiled binaries for Windows, macOS (amd64/arm64), and Linux (amd64/arm64) via Go's built-in cross-compilation support. Distributed as standalone binaries via GitHub Releases.

The tool assumes users have a valid kubeconfig file with authentication details (e.g., certificates, tokens). No in-cluster or alternative auth methods are supported.

## Functional Requirements

### Core Features
1. **Context Management**:
    - Load available Kubernetes contexts from the user's kubeconfig file.
    - Display the current context in the UI (e.g., in a header or status bar).
    - Allow switching contexts via a selector popup triggered by a hotkey (e.g., 'c').
    - The selector lists all contexts sorted alphabetically, with keyboard navigation (up/down arrows, enter to select).
    - On startup, load the last selected context from persistence; fallback to the kubeconfig's current-context if not set.
    - Persist the selected context to the config file upon change or quit.

2. **Namespace Management**:
    - Fetch and list all namespaces in the current context from the Kubernetes API.
    - Display the current namespace in the UI (e.g., next to the context in the status bar).
    - Allow switching namespaces via a selector popup triggered by a hotkey (e.g., 'n').
    - The selector lists all namespaces sorted alphabetically, with keyboard navigation (up/down arrows, enter to select).
    - On startup, load the last selected namespace from persistence; fallback to 'default' if not set.
    - Persist the selected namespace to the config file upon change or quit.
    - Handle API errors (e.g., permission denied) by displaying an error message in the UI.

3. **Pod Listing and Display**:
    - Fetch pods from the current namespace using the Kubernetes API (ListPods with appropriate field selectors if needed for efficiency).
    - Display pods in a table format within the TUI, with columns: Name, Status, Ready (e.g., "2/3"), Restarts, Age (human-readable, e.g., "5m"), IP, Node, Labels (comma-separated key=value pairs).
    - Sort the pod list alphabetically by name.
    - Use colors for status indicators:
        - Green for "Running".
        - Red for "Failed", "CrashLoopBackOff", or "Error".
        - Yellow for "Pending" or "Initializing".
        - Gray or default for other statuses (e.g., "Completed", "Terminating").
    - The table should be scrollable if there are many pods (using Bubble Tea's viewport component).
    - No filtering, additional sorting, or advanced interactions (e.g., selecting a pod for details).

4. **Refreshing**:
    - Automatically refresh pod data every 5 seconds using Bubble Tea's tick mechanism.
    - Refresh should re-fetch pods, namespaces (if needed), and update the display without disrupting user interaction.
    - Manual refresh option via hotkey (e.g., 'r') for on-demand updates.

5. **User Interface Layout**:
    - **Main View**: A full-screen TUI with:
        - Header: Tool name, current context, current namespace, and last refresh time.
        - Body: Scrollable table of pods.
        - Footer/Status Bar: Hotkeys (e.g., "c: Contexts | n: Namespaces | r: Refresh | q: Quit") and any error messages.
    - Selectors (contexts/namespaces) appear as centered popups/modals when triggered, overlaying the main view.
    - Use Lip Gloss for styling: Borders around tables/popups, padding, and color themes.
    - Ensure the UI is responsive to terminal resizing.

6. **Quit Action**:
    - Exit the application gracefully via hotkey (e.g., 'q' or Ctrl+C).
    - Before quitting, persist the current context and namespace.

7. **Error Handling**:
    - Display errors in the UI (e.g., in the status bar or as a temporary popup) for issues like:
        - Invalid kubeconfig.
        - API connection failures (e.g., "Unable to connect to API server: timeout").
        - Permission errors (e.g., "Access denied to list pods in namespace").
    - Log detailed errors to stderr for debugging, but keep UI messages concise and user-friendly.
    - Graceful fallback: If pods can't be fetched, show an empty table with an error message.

### Non-Functional Requirements
1. **Performance**:
    - Efficient API calls: Use watchers or informers from client-go for potential future optimizations, but stick to simple List calls for initial implementation.
    - Target low memory/CPU usage suitable for terminal tools (e.g., <50MB RAM).
    - Handle large namespaces (up to 1000 pods) without lagging the TUI.

2. **Security**:
    - Rely solely on kubeconfig for auth; no additional credentials handling.
    - Do not store sensitive data in the persistence file (only context/namespace names).

3. **Compatibility**:
    - Support Kubernetes API versions v1.20+.
    - Cross-platform: Binaries for Windows (exe), macOS (Darwin amd64/arm64), Linux (amd64/arm64).
    - Test on common terminals (e.g., iTerm, Windows Terminal, GNOME Terminal).

4. **Persistence**:
    - Use a JSON file (`~/.kontrol/config.json`) with structure: `{ "context": "string", "namespace": "string" }`.
    - Handle file creation/read/write with proper error checking.

5. **Logging**:
    - Use Go's standard log package for debug/info logs to stderr.
    - No configurable levels; keep it minimal.

6. **Accessibility**:
    - Ensure keyboard-only navigation (no mouse support needed).
    - Use high-contrast colors for status indicators.

## Development Approach

### Dependencies
- `k8s.io/client-go`: For Kubernetes API interactions.
- `k8s.io/apimachinery`: For API types and utilities.
- `github.com/charmbracelet/bubbletea`: For TUI.
- `github.com/charmbracelet/lipgloss`: For styling.
- `gopkg.in/yaml.v2`: For parsing kubeconfig (if needed beyond client-go).
- Standard libraries for JSON persistence, time handling, etc.

### Project Structure
```
kontrol/
├── cmd/
│   └── kontrol/
│       └── main.go  # Entry point, initializes TUI
├── internal/
│   ├── config/      # Persistence logic
│   ├── k8s/         # Kubernetes client and fetchers
│   ├── ui/          # Bubble Tea models, views, updates
│   └── utils/       # Helpers (e.g., sorting, coloring)
├── go.mod
├── go.sum
└── README.md        # Usage, build instructions
```

### Build and Distribution
- Use Go modules for dependency management.
- Cross-compilation script (e.g., Bash or Makefile):
  ```
  GOOS=linux GOARCH=amd64 go build -o kontrol-linux-amd64 ./cmd/kontrol
  GOOS=darwin GOARCH=arm64 go build -o kontrol-darwin-arm64 ./cmd/kontrol
  # Similarly for others
  ```
- Distribute via GitHub Releases: Upload binaries with checksums, no installers or packages.

### Testing Approach
For a lightweight approach, focus on unit and integration tests without heavy dependencies:
- **Unit Tests**: Use Go's testing package.
    - Test Kubernetes fetchers with mocked clients (using client-go's fake package).
    - Test UI models in isolation (e.g., simulate key presses, verify view output).
    - Test persistence (read/write JSON).
- **Integration Tests**: Use Minikube or Kind for a local cluster.
    - Script to start a test cluster, apply sample pods, run kontrol in headless mode (if possible) or manually verify.
    - Keep it manual-light: Run the binary against the test cluster and assert via logs or simple scripts.
- **Coverage Target**: 70%+ for core logic.
- Tools: Go test, no external frameworks needed for simplicity.

## Potential Risks and Mitigations
- **API Changes**: Pin client-go versions; monitor Kubernetes deprecations.
- **TUI Complexity**: Start with basic Bubble Tea examples; iterate on layout.
- **Cross-Platform Issues**: Test binaries on each OS; handle file paths portably.
- **Error Proneness**: Extensive error wrapping with fmt.Errorf for clear messages.

This concept provides a solid foundation for implementing kontrol as a minimal viable product, aligning with Kubernetes best practices and Go's simplicity. If needed, it can be extended later while keeping the core focused.