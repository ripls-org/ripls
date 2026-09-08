# Terraform Infrastructure Configuration

This directory contains Terraform configurations for deploying the gear-library
service using a modular, environment-specific approach. While currently
configured for Google Cloud Platform, the modular structure allows for
adaptation to other cloud providers.

## Directory Structure

```
terraform/
├── environments/          # Environment-specific configurations
│   ├── dev/              # Development environment
│   │   ├── main.tf       # Environment resource orchestration
│   │   └── variables.tf  # Environment-specific variable definitions
│   └── prod/             # Production environment
│       ├── main.tf       # Environment resource orchestration
│       └── variables.tf  # Environment-specific variable definitions
└── modules/              # Reusable infrastructure modules
    ├── containers/       # Application hosting module
    │   ├── main.tf       # Container service resources
    │   └── variables.tf  # Container module variables
    └── storage/          # Database and networking module
        ├── main.tf       # Database, networking, and connectivity
        ├── variables.tf  # Storage module variables
        └── outputs.tf    # Connection details and networking info
```

## Architecture Overview

### Module Design Philosophy

**Storage Module**: Provides managed database services with private networking.
Outputs connection details for consumption by other modules. Handles database
provisioning, user management, and secure networking setup.

**Containers Module**: Manages containerized application deployment with
auto-scaling capabilities. Consumes database connection details from the storage
module and configures the application environment.

**Environment Orchestration**: Each environment directory composes modules with
environment-specific parameters, allowing identical infrastructure patterns with
different sizing and availability configurations.

### Cloud Provider Considerations

The current implementation uses Google Cloud Platform services, but the modular
structure facilitates adaptation:

- **Storage Module**: Uses Cloud SQL (PostgreSQL). Could be adapted for AWS RDS,
  Azure Database, or other managed database services.
- **Containers Module**: Uses Cloud Run. Could be adapted for AWS Fargate, Azure
  Container Instances, or Kubernetes-based solutions.
- **Networking**: Uses GCP VPC connectors. Other providers have equivalent
  private networking solutions.

Provider-specific details are contained within modules, while environment
configurations remain largely provider-agnostic.

## Configuration Approach

### Environment Separation

**Development Environment**: Optimized for cost efficiency and rapid iteration.
Uses minimal resource allocations and allows resource deletion for cost
management.

**Production Environment**: Configured for reliability and performance.
Implements high availability, appropriate resource sizing, and protection
mechanisms.

### Variable Management

Environment-specific variables are defined in each environment's `variables.tf`.
Sensitive values (passwords, secrets) should be provided via `.tfvars` files or
environment variables, not committed to version control.

### Module Communication

Modules communicate through Terraform's input/output system:

- Storage module outputs connection details and networking configuration
- Containers module accepts these outputs as input variables
- Environment configurations orchestrate this data flow

## Remote State

Terraform state is stored in a GCS bucket per environment, with versioning and
state locking. Each environment declares its own bucket in its `backend` block;
the buckets themselves are deployment-specific and are not named here.

State is loaded automatically on `terraform init`. All operators see the same
state, and concurrent `terraform apply` calls are blocked by native GCS
locking.

**First-time setup for a new operator:**

```bash
cd terraform/environments/dev   # or prod
terraform init                  # downloads remote state automatically
terraform plan                  # verify you see the current infrastructure
```

**If a lock gets stuck** (e.g., operator's machine crashes during apply):

```bash
terraform force-unlock <LOCK_ID>
```

**Access requirements:** Operators need `roles/storage.objectAdmin` on the
state bucket for their environment.

## Local Development and Testing

### Prerequisites

- Terraform CLI installed
- Cloud provider CLI tools configured
- Appropriate authentication credentials set up

### Testing Commands

The project includes npm scripts for consistent testing:

```bash
# Format and validate configurations
npm run lint:terraform

# Test deployment plans without applying
npm run test:terraform

# Test specific environments
npm run plan:terraform:dev
npm run plan:terraform:prod
```

### Configuration Validation

The testing workflow validates:

- Terraform syntax and structure
- Module references and variable passing
- Provider configuration compatibility
- Resource dependency resolution

This local testing provides confidence before any cloud resources are created or
modified.

### Deployment Workflow

1. **Format**: Ensure consistent code style
2. **Validate**: Check syntax and configuration structure
3. **Plan**: Preview changes without applying them
4. **Apply**: Execute the planned changes (manual step)

The first three steps are automated through npm scripts and suitable for CI/CD
integration.

## Security and Best Practices

### Secret Management

- Database passwords and sensitive configuration via variables
- `.tfvars` files excluded from version control
- Environment variables for CI/CD authentication

### Network Security

- Database services deployed with private networking
- Application-to-database communication via secure internal networks
- Public access limited to application endpoints only

### Resource Management

- Environment-specific resource sizing and availability
- Development environments allow deletion for cost control
- Production environments include appropriate protection mechanisms

This configuration provides a foundation for reliable, secure, and
cost-effective infrastructure management while maintaining flexibility for
different deployment targets.
