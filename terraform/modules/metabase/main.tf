# =============================================================================
# METABASE MODULE - ANALYTICS DASHBOARD
# =============================================================================
# Deploys Metabase as a Cloud Run service connected to the application's
# Cloud SQL database via a read-only user. Metabase's internal metadata
# (dashboards, saved questions, users) is stored in a separate database
# on the same Cloud SQL instance.

# =============================================================================
# METABASE METADATA DATABASE AND USER
# =============================================================================
# Separate database on the shared Cloud SQL instance for Metabase internals.
resource "google_sql_database" "metabase" {
  name     = var.metabase_db_name
  instance = var.cloud_sql_instance_name
  project  = var.project_id
}

# User for Metabase to manage its own metadata database.
resource "google_sql_user" "metabase_admin" {
  name     = var.metabase_db_user
  instance = var.cloud_sql_instance_name
  password = var.metabase_db_password
  project  = var.project_id
}

# =============================================================================
# READ-ONLY USER FOR APPLICATION DATABASE
# =============================================================================
# Metabase connects to the application database with this user.
# SELECT-only grants are applied after creation via the grant script.
resource "google_sql_user" "metabase_reader" {
  name     = var.metabase_reader_user
  instance = var.cloud_sql_instance_name
  password = var.metabase_reader_password
  project  = var.project_id
}

# NOTE: After terraform apply creates the metabase_reader user, you must
# manually run the grant script to give it SELECT-only access. Connect to
# Cloud SQL as the application user and run:
#
#   GRANT CONNECT ON DATABASE <db> TO metabase_reader;
#   GRANT USAGE ON SCHEMA public TO metabase_reader;
#   GRANT SELECT ON ALL TABLES IN SCHEMA public TO metabase_reader;
#   GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO metabase_reader;
#   ALTER DEFAULT PRIVILEGES FOR ROLE <app_user> IN SCHEMA public
#     GRANT SELECT ON TABLES TO metabase_reader;
#
# See scripts/metabase-readonly-user.sql for the full script.
# This is a one-time step after initial deployment.

# =============================================================================
# METABASE CLOUD RUN SERVICE
# =============================================================================
resource "google_cloud_run_v2_service" "metabase" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  template {
    containers {
      image = var.container_image

      ports {
        container_port = 3000
      }

      # Metabase metadata database configuration
      env {
        name  = "MB_DB_TYPE"
        value = "postgres"
      }

      env {
        name  = "MB_DB_HOST"
        value = var.database_host
      }

      env {
        name  = "MB_DB_PORT"
        value = tostring(var.database_port)
      }

      env {
        name  = "MB_DB_DBNAME"
        value = var.metabase_db_name
      }

      env {
        name  = "MB_DB_USER"
        value = var.metabase_db_user
      }

      env {
        name = "MB_DB_PASS"
        value_source {
          secret_key_ref {
            secret  = var.metabase_db_password_secret_id
            version = "latest"
          }
        }
      }

      env {
        name  = "MB_JETTY_PORT"
        value = "3000"
      }

      env {
        name  = "JAVA_OPTS"
        value = "-Xmx512m -Xms256m"
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = var.cpu_limit
          memory = var.memory_limit
        }
      }

      # Metabase takes 2-5 minutes on first boot (hundreds of DB migrations).
      startup_probe {
        http_get {
          path = "/api/health"
          port = 3000
        }
        initial_delay_seconds = 60
        period_seconds        = 15
        failure_threshold     = 20
        timeout_seconds       = 5
      }
    }

    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instances
    }

    # Direct VPC egress for private database connection (#2809) — no
    # always-on connector VMs.
    vpc_access {
      network_interfaces {
        network    = var.vpc_network
        subnetwork = var.vpc_egress_subnet
      }
      egress = "PRIVATE_RANGES_ONLY"
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

# =============================================================================
# ACCESS CONTROL
# =============================================================================
# When using a custom domain, Cloud Run must allow public access — Metabase's
# built-in authentication handles user access control.
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  count    = var.allow_public_access ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.metabase.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Per-member IAM access (used when public access is disabled).
resource "google_cloud_run_v2_service_iam_member" "metabase_access" {
  count    = var.allow_public_access ? 0 : length(var.allowed_members)
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.metabase.name
  role     = "roles/run.invoker"
  member   = var.allowed_members[count.index]
}
