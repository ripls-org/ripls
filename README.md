# ripls

Platform for community building through sharing of goods and skills

## Directory Structure

```
├── proto/                        # Protocol buffer definitions
│   └── ripls/
│       ├── api/                  # RPC API types (for service interfaces)
│       └── models/               # Canonical / stored data models
├── server/                       # Go server implementation
│   ├── ai/                       # AI provider integrations (Gemini, Vertex AI)
│   ├── auth/                     # Authentication (JWT, OIDC)
│   ├── email/                    # Email service (Mailgun)
│   ├── location/                 # Location services (Mapbox, EXIF)
│   ├── notifications/            # Push notifications (FCM, noop)
│   ├── services/                 # RPC service implementations
│   │   ├── community/            # Community management
│   │   ├── device/               # User device registration
│   │   ├── gear/                 # Gear (items) management
│   │   ├── loan/                 # Loan tracking
│   │   ├── location/             # Location/place management
│   │   ├── login/                # Authentication & user registration
│   │   ├── media/                # Media upload & storage
│   │   ├── search/               # Search across gear & communities
│   │   └── user/                 # User profile management
│   └── storage/                  # Proto-SQL storage abstraction
├── app/                          # Flutter mobile application
│   ├── lib/
│   │   ├── core/                 # Core utilities & configuration
│   │   │   ├── config/           # Environment configuration
│   │   │   ├── router/           # Navigation & routing
│   │   │   ├── theme/            # App theming & colors
│   │   │   └── utils/            # Utility functions
│   │   ├── data/                 # Data layer
│   │   │   ├── cache/            # Caching infrastructure
│   │   │   ├── repositories/     # Data access with caching
│   │   │   └── gen/              # Generated protobuf code (Dart)
│   │   ├── presentation/         # UI layer
│   │   │   ├── controllers/      # State management
│   │   │   ├── screens/          # Full-page widgets
│   │   │   └── widgets/          # Reusable UI components
│   │   └── services/             # API clients & business logic
│   ├── android/                  # Android-specific configuration
│   ├── ios/                      # iOS-specific configuration
│   └── macos/                    # macOS-specific configuration
├── docs/                         # Architectural documentation
├── terraform/                    # Infrastructure as code
├── buf.yaml                      # Buf configuration
├── buf.gen.yaml                  # Code generation config
├── go.mod                        # Go module definition
└── package.json                  # Build automation
```

### Package Structure

Protocol buffer packages are organized by purpose:

- **API Package** (`ripls.api`): Types exposed in RPC service interfaces
- **Models Package** (`ripls.models`): Types used for storage/persistence

**IMPORTANT**: API and Models types must never import each other. This separation allows the API and storage schemas to evolve independently. Conversion between API and storage types is handled by the server's service layer.

See [`docs/proto_conventions.md`](docs/proto_conventions.md) for the full convention, including API message shape, naming parity where a value genuinely round-trips, and round-trip test expectations.

## Protocol Buffer Development

We use the Buf toolchain for protocol buffer linting, breaking change detection,
and code generation.

Style follows the Buf style guide:
https://buf.build/docs/best-practices/style-guide/#categories

**IMPORTANT**: Generated code from protocol buffers is **not checked into source
control**. It is automatically generated during CI/CD builds and Docker builds.
For local development, you must generate the code before building or testing.

### Setup Dependencies

Install required tools:

```bash
# Install Homebrew (macOS package manager, if not already installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Go (if not already installed)
# Download the macOS installer from https://go.dev/dl/ and run it,
# or install via Homebrew:
brew install go

# Install Flutter (required for app code generation)
# Follow the official guide: https://docs.flutter.dev/get-started/install/macos/mobile-ios
# After installing, ensure `flutter` is in your PATH.

# Install the Flutter VS Code extension (provides Dart/Flutter tooling, debugging, hot reload)
code --install-extension Dart-Code.flutter

# Install Android Studio (required for Android SDK, emulators, and building the Android app)
brew install --cask android-studio
# After installing, launch Android Studio once to complete SDK setup, then accept licenses:
flutter doctor --android-licenses

# Install Node.js / npm (if not already installed)
# Download the macOS installer from https://nodejs.org or install via Homebrew:
brew install node

# Install CocoaPods (required for building the iOS app)
brew install cocoapods

# Install Java (required for Android Gradle builds and keytool)
brew install openjdk

# Install Docker (required for local PostgreSQL)
brew install --cask docker

# Install Google Cloud SDK + both auth contexts.
#  - gcloud auth login                       (Secret Manager secret fetches)
#  - gcloud auth application-default login   (Vertex AI / Firebase from the server)
# Both are required for `npm run start:server`; they expire independently.
# The startup script preflights both and prints the reauth command on failure.
brew install --cask google-cloud-sdk
gcloud auth login
gcloud auth application-default login

# Install ONNX Runtime (required for local embedding model). Must be a version
# whose C-API is at least the ORT_API_VERSION vendored by onnxruntime_go, or the
# embedder fails to initialize with "Error setting ORT API base" and every test
# that spawns the server hangs to its timeout. versions.env ONNXRUNTIME_VERSION
# is the version CI and the images use; `npm run check:onnx-tarballs` verifies a
# candidate publishes the release tarballs those images download.
brew install onnxruntime

# If the Homebrew bottle fails to load with a missing libabsl_*.dylib, it was
# built against a different abseil ABI than the one installed. Point at the
# official self-contained release tarball instead — ONNXRUNTIME_LIB_PATH is
# checked before the system paths, so it wins:
#   curl -LO https://github.com/microsoft/onnxruntime/releases/download/v1.29.0/onnxruntime-osx-arm64-1.29.0.tgz
#   tar xzf onnxruntime-osx-arm64-1.29.0.tgz
#   export ONNXRUNTIME_LIB_PATH=$PWD/onnxruntime-osx-arm64-1.29.0/lib/libonnxruntime.dylib

# Install Git LFS (required to fetch the embedding model file)
brew install git-lfs
git lfs install
git lfs pull -I model_tuning/ripls_embedding.onnx

# Install Terraform (required for infrastructure validation and deployment)
brew install terraform

# Install pandoc (required for release notes generation)
brew install pandoc

# Install pre-commit (runs the gitleaks secret-scanning hook on every commit;
# see docs/secrets.md "Detection and prevention (DIY stack)")
brew install pre-commit

# Install Go code generators (one-time setup)
go install github.com/bufbuild/buf/cmd/buf@latest
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
go install mvdan.cc/gofumpt@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Ensure Go binaries are in your PATH — add this to ~/.zshrc to make it permanent:
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
source ~/.zshrc

# Install project dependencies (the postinstall script also wires up the
# gitleaks pre-commit hook automatically — no separate `pre-commit install` needed)
npm install
```

**Optional: auto-install pre-commit hooks on every future `git clone`.** If you
want any future repo you clone (this one or any other) to automatically pick up
its `.pre-commit-config.yaml`, do this once per machine:

```bash
pre-commit init-templatedir ~/.git-template
git config --global init.templateDir ~/.git-template
```

This affects all your clones, not just this repo. Skip if you'd rather rely on
each repo's `npm install` (or equivalent) to wire up its own hooks.

**Note**: Docker Desktop must be running for tests to work. Tests use testcontainers
to automatically start PostgreSQL containers.

Dependencies are managed in `go.mod` with module name:
the value in `go.mod`

### Development Commands

All protocol buffer operations are run from the repository root:

```bash
# Lint protocol buffer files
buf lint

# Generate Go and Dart code (MUST run before building/testing locally)
npm run generate

# Check for breaking changes
buf breaking --against '.git#branch=main'
```

## Server

The Go server provides gRPC-compatible HTTP/JSON APIs using Connect-Go. See [`server/README.md`](server/README.md) for detailed documentation.

### Quick Start

**IMPORTANT**: Before running the server, you must generate code from protocol buffers:

```bash
# Generate code (required first step)
npm run generate
```

Start the server with optional port and authentication configuration:

```bash
# Default: PostgreSQL on localhost:5432 (no auth endpoints)
go run ./server

# Enable development mode (dev auth, simulation clock, database reset)
go run ./server --dev-mode

# Custom database and port with development features
go run ./server --db "postgres://user:pass@localhost/dbname?sslmode=disable" --port 8081 --dev-mode

# Full development setup with all features
npm run start:server
```

For detailed API usage, authentication, and testing information, see [`server/README.md`](server/README.md).

## Infrastructure as Code

This project uses [Terraform](https://terraform.io) for declarative
infrastructure deployment management. Terraform allows us to define cloud
resources (databases, compute, networking) as code, enabling version control,
reproducible deployments, and automated infrastructure management.

Key benefits:

- **Declarative**: Describe desired end state, Terraform handles the "how"
- **Version Controlled**: Infrastructure changes tracked alongside application
  code
- **Environment Parity**: Identical infrastructure across dev/staging/production
- **Automated**: Deploy and manage infrastructure through CI/CD pipelines

See [`terraform/README.md`](terraform/README.md) for detailed configuration
documentation.

### Database Connection Details

To view database connection details for deployed environments:

```bash
# View all connection details including password
cd terraform/environments/dev
terraform output database_connection

# Get password only
gcloud secrets versions access latest --secret="<db-password-secret>" \
  --project="$(scripts/gcp_project.sh dev)"

# View as JSON
terraform output -json database_connection | jq
```

### Infrastructure Testing

Test Terraform configurations locally before deployment:

```bash
# Lint and validate all configurations
npm run lint:terraform

# Test deployment plans for all environments
npm run test:terraform
```

## Testing

**IMPORTANT**: Before running tests, you must generate code from protocol buffers:

```bash
# Generate code (required first step)
npm run generate
```

Run all tests in the project:

```bash
npm run test
```

For server-specific testing details, see [`server/README.md`](server/README.md).
