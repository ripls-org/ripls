# =============================================================================
# CONTAINER SERVICE MODULE
# =============================================================================
# This module provisions a managed container service with persistent storage.
# Currently implemented for Google Cloud Run, but designed to be portable
# to other providers (AWS Fargate, Azure Container Instances, etc.)

# =============================================================================
# CONTAINER SERVICE
# =============================================================================
# Main container service that runs the server application
resource "google_cloud_run_v2_service" "server" {
  name     = var.service_name
  location = var.region
  project  = var.project_id
  ingress  = var.ingress

  template {
    # Request timeout - increased from default 300s for streaming RPCs
    timeout = "${var.request_timeout}s"

    # Maximum concurrent requests per instance. Go handles concurrency via
    # goroutines, so idle streaming connections should not trigger autoscaling.
    # Invariant: max_instances * the server's DB pool limit (storage.maxOpenConns)
    # must be < the database's max_connections.
    max_instance_request_concurrency = var.max_instance_request_concurrency

    containers {
      image = var.container_image

      # Network configuration
      ports {
        container_port = var.container_port
      }

      # Environment variables for application configuration
      env {
        name  = "DB_HOST"
        value = var.database_host
      }

      env {
        name  = "DB_PORT"
        value = tostring(var.database_port)
      }

      env {
        name  = "DB_NAME"
        value = var.database_name
      }

      env {
        name  = "DB_USER"
        value = var.database_user
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = var.db_password_secret_id
            version = "latest"
          }
        }
      }

      env {
        name  = "ENABLE_DEV_MODE"
        value = var.enable_dev_mode ? "true" : "false"
      }

      env {
        name  = "ENABLE_DEV_CLIENT"
        value = var.enable_dev_client ? "true" : "false"
      }

      env {
        name  = "LOG_SOURCE_LOCATION"
        value = var.log_source_location ? "true" : "false"
      }

      # GCS bucket for media storage
      env {
        name  = "GCS_MEDIA_BUCKET"
        value = var.gcs_media_bucket
      }

      # Vertex AI configuration for AI-powered features.
      # GOOGLE_CLOUD_LOCATION is intentionally unset so the server's
      # built-in -vertex-ai-location default ("global") takes effect;
      # newer Gemini families (3.x) are only published on the global
      # endpoint, and 'global' also spreads regional capacity.
      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }

      # Mailgun routing config (non-secret; domain and From address).
      # The API key is fetched from Secret Manager by /app/entrypoint.sh
      # at container startup (#1768/#1770).
      env {
        name  = "MAILGUN_DOMAIN"
        value = var.mailgun_domain
      }

      env {
        name  = "MAILGUN_FROM_ADDRESS"
        value = var.mailgun_from_address
      }

      # CAN-SPAM postal address for the off-app notification email footer
      # (#2492). Non-secret; public in every email.
      env {
        name  = "MAILGUN_POSTAL_ADDRESS"
        value = var.mailgun_postal_address
      }

      # Off-app email channel (#2492). Gates emailing a deviceless recipient.
      # Kept false until the CAN-SPAM postal address + deliverability are
      # signed off.
      env {
        name  = "OFF_APP_EMAIL_ENABLED"
        value = var.off_app_email_enabled ? "true" : "false"
      }

      # GitHub App configuration for feedback bot. App ID and Installation
      # ID are non-secret routing values; the private key lives in Secret
      # Manager under github-app-private-key-base64.
      env {
        name  = "GITHUB_APP_ID"
        value = tostring(var.github_app_id)
      }

      env {
        name  = "GITHUB_INSTALLATION_ID"
        value = tostring(var.github_installation_id)
      }

      # Repository the feedback bot files issues against. Required alongside
      # the App credentials: without them the server cannot build the GitHub
      # client and feedback stops creating issues.
      env {
        name  = "GITHUB_REPO_OWNER"
        value = var.github_repo_owner
      }

      env {
        name  = "GITHUB_REPO_NAME"
        value = var.github_repo_name
      }

      # Recipient of waitlist-signup notifications. Empty disables them.
      env {
        name  = "WAITLIST_NOTIFY_EMAIL"
        value = var.waitlist_notify_email
      }

      # The deployment's identity (server/branding): store listings, legal
      # entity, policy URLs. entrypoint.sh maps each to its flag.
      dynamic "env" {
        for_each = var.branding
        content {
          name  = env.key
          value = env.value
        }
      }

      # Google OAuth Client ID for OIDC authentication.
      env {
        name  = "GOOGLE_CLIENT_ID"
        value = var.google_client_id
      }

      # Hostname for invitation short links
      env {
        name  = "INVITE_LINK_HOSTNAME"
        value = var.invite_link_hostname
      }

      # Allowed CORS origins (comma-separated); required in production
      env {
        name  = "CORS_ALLOWED_ORIGINS"
        value = var.cors_allowed_origins
      }

      # Active location provider (#2188). The server picks between
      # MapboxClient and GoogleMapsClient at startup; both providers'
      # API keys can be present in Secret Manager during a cutover, but
      # only the one named here receives traffic.
      env {
        name  = "MAP_PROVIDER"
        value = var.map_provider
      }

      # Platform SMS (#2492). Gates the text-message notification channel.
      # Kept false until A2P 10DLC registration and counsel sign-off; the
      # Twilio credentials are supplied separately via Secret Manager.
      env {
        name  = "PLATFORM_SMS_ENABLED"
        value = var.platform_sms_enabled ? "true" : "false"
      }

      # Activity digest configuration (#1924). Two independent cadences
      # (daily + weekly) share recipient and timezone. Both default off.
      env {
        name  = "ACTIVITY_DIGEST_ENABLED"
        value = var.activity_digest_enabled ? "true" : "false"
      }

      env {
        name  = "ACTIVITY_DIGEST_WEEKLY_ENABLED"
        value = var.activity_digest_weekly_enabled ? "true" : "false"
      }

      env {
        name  = "ACTIVITY_DIGEST_NOTIFY_EMAIL"
        value = var.activity_digest_notify_email
      }

      env {
        name  = "ACTIVITY_DIGEST_TIMEZONE"
        value = var.activity_digest_timezone
      }

      env {
        name  = "ACTIVITY_DIGEST_SEND_HOUR"
        value = tostring(var.activity_digest_send_hour)
      }

      env {
        name  = "ACTIVITY_DIGEST_WEEKLY_WEEKDAY"
        value = var.activity_digest_weekly_weekday
      }

      env {
        name  = "ACTIVITY_DIGEST_WEEKLY_SEND_HOUR"
        value = tostring(var.activity_digest_weekly_send_hour)
      }

      # The following server-side credentials are NOT plumbed through
      # Terraform any more. /app/entrypoint.sh in the container fetches
      # them from Secret Manager at startup using the Cloud Run runtime
      # service account's metadata-server token: openai-api-key,
      # anthropic-api-key, mailgun-api-key, unsplash-access-key,
      # pexels-api-key, pixabay-api-key, mapbox-access-token,
      # jwt-signing-secret, github-app-private-key-base64.
      # See server/entrypoint.sh and the
      # secretmanager.secretAccessor IAM grants in
      # terraform/environments/{dev,prod}/main.tf.

      # Resource limits for cost control and performance. See var.cpu_idle
      # for the billing-mode tradeoff (always-allocated CPU for background
      # work vs. request-only CPU for scale-to-zero environments).
      resources {
        cpu_idle = var.cpu_idle
        limits = {
          cpu    = var.cpu_limit
          memory = var.memory_limit
        }
      }
    }


    # Auto-scaling configuration
    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    # Direct VPC egress for private database connection (#2809). Unlike the
    # former Serverless VPC Access connector, this attaches instances to the
    # VPC directly with no always-on relay VMs.
    vpc_access {
      network_interfaces {
        network    = var.vpc_network
        subnetwork = var.vpc_egress_subnet
      }
      egress = "PRIVATE_RANGES_ONLY"
    }
  }

  # Traffic routing (100% to latest revision)
  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }

  # The container image is owned by the CD pipeline (.github/workflows/build_and_deploy_container.yaml),
  # which deploys specific git-SHA tags via `gcloud run services update --image=...`. Terraform owns
  # the env/secrets/scaling config; ignoring image avoids drift on every plan.
  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }
}

# =============================================================================
# SERVICE ACCOUNT & IAM PERMISSIONS
# =============================================================================
# Get the project number for constructing service account email
data "google_project" "project" {
  project_id = var.project_id
}

# Grant Firebase Cloud Messaging access to the default Compute Engine service account
# This allows Cloud Run to send push notifications via FCM using Application Default Credentials
resource "google_project_iam_member" "firebase_admin" {
  project = var.project_id
  role    = "roles/firebase.admin"
  member  = "serviceAccount:${data.google_project.project.number}-compute@developer.gserviceaccount.com"
}

# =============================================================================
# ACCESS CONTROL
# =============================================================================
# Configure public access to the container service
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  count    = var.allow_public_access ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.server.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

