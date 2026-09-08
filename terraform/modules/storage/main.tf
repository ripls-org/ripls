# =============================================================================
# STORAGE MODULE - CLOUD SQL POSTGRESQL
# =============================================================================
# This module provisions managed PostgreSQL database for persistent storage.
# Currently implemented for Google Cloud SQL, but designed to be portable
# to other providers (AWS RDS, Azure Database, etc.)

# =============================================================================
# NETWORKING
# =============================================================================
# VPC network for secure database access
resource "google_compute_network" "vpc" {
  name                    = "${var.instance_name}-vpc"
  auto_create_subnetworks = false
  project                 = var.project_id
}

# Subnet for the database
resource "google_compute_subnetwork" "db_subnet" {
  name          = "${var.instance_name}-db-subnet"
  ip_cidr_range = "10.0.1.0/24"
  region        = var.region
  network       = google_compute_network.vpc.id
  project       = var.project_id
}

# Private service connection for Cloud SQL
resource "google_compute_global_address" "private_ip_address" {
  name          = "${var.instance_name}-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
  project       = var.project_id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_address.name]
}

# =============================================================================
# DATABASE INSTANCE
# =============================================================================
# Main PostgreSQL database instance
resource "google_sql_database_instance" "gear_db" {
  name             = var.instance_name
  database_version = var.postgres_version
  region           = var.region
  project          = var.project_id

  # Must wait for VPC peering to complete before creating instance
  depends_on = [google_service_networking_connection.private_vpc_connection]

  settings {
    tier = var.tier

    # Instance deletion protection (Cloud SQL API setting)
    deletion_protection_enabled = var.deletion_protection

    # Backup configuration
    backup_configuration {
      enabled                        = true
      start_time                     = "03:00"
      point_in_time_recovery_enabled = true
      backup_retention_settings {
        retained_backups = var.backup_retention_days
        retention_unit   = "COUNT"
      }
      # Transaction log retention for point-in-time recovery
      transaction_log_retention_days = var.transaction_log_retention_days
    }

    # High availability for production
    availability_type = var.high_availability ? "REGIONAL" : "ZONAL"

    # IP configuration - private network only
    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.vpc.id
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    # Storage configuration
    disk_type = "PD_SSD"
    disk_size = var.storage_size_gb

    # Performance settings
    insights_config {
      query_insights_enabled  = true
      record_application_tags = true
    }
  }

  # Prevent accidental deletion
  deletion_protection = var.deletion_protection
}

# =============================================================================
# DATABASE AND USER
# =============================================================================
# Application database
resource "google_sql_database" "gear_database" {
  name     = var.database_name
  instance = google_sql_database_instance.gear_db.name
  project  = var.project_id
}

# Application user with limited privileges
resource "google_sql_user" "gear_user" {
  name     = var.database_user
  instance = google_sql_database_instance.gear_db.name
  password = var.database_password
  project  = var.project_id
}

# =============================================================================
# CLOUD RUN VPC EGRESS
# =============================================================================
# Subnet for Cloud Run Direct VPC egress (#2809). Replaces the Serverless
# VPC Access connector, whose 2-3 always-on e2-micro instances were pure
# standing cost; direct egress attaches Cloud Run instances to the VPC
# with no per-instance charge. A /26 (64 addresses) covers the small
# max-instance counts here with headroom.
resource "google_compute_subnetwork" "cloud_run_egress" {
  name          = "${var.instance_name}-egress-subnet"
  ip_cidr_range = "10.0.2.0/26"
  region        = var.region
  network       = google_compute_network.vpc.id
  project       = var.project_id
}

# =============================================================================
# CONNECTION DETAILS
# =============================================================================
# Generate connection details for standard PostgreSQL connection
locals {
  # Standard PostgreSQL connection using private IP
  database_host = google_sql_database_instance.gear_db.private_ip_address
  database_port = 5432
}

# =============================================================================
# MEDIA STORAGE BUCKET
# =============================================================================
# GCS bucket for storing user-uploaded media files (images, videos)
resource "google_storage_bucket" "media_bucket" {
  name     = var.media_bucket_name
  location = var.region
  project  = var.project_id

  # Storage class for cost optimization
  storage_class = var.media_storage_class

  # Lifecycle rules for automatic deletion of old media (optional)
  dynamic "lifecycle_rule" {
    for_each = var.media_lifecycle_days > 0 ? [1] : []
    content {
      condition {
        age = var.media_lifecycle_days
      }
      action {
        type = "Delete"
      }
    }
  }

  # Enable versioning for data protection (production only)
  versioning {
    enabled = var.media_versioning_enabled
  }

  # CORS configuration for direct browser uploads (if needed in future)
  cors {
    origin          = var.media_cors_origins
    method          = ["GET", "HEAD"]
    response_header = ["Content-Type"]
    max_age_seconds = 3600
  }

  # Public access prevention - uniform bucket-level access enforces IAM only
  uniform_bucket_level_access = true

  # Force destroy (for dev environments)
  force_destroy = !var.deletion_protection
}

# Get default Compute Engine service account
data "google_compute_default_service_account" "default" {
  project = var.project_id
}

# Grant default Compute Engine service account access to media bucket
# This allows Cloud Run (which uses this service account by default) to access the bucket
resource "google_storage_bucket_iam_member" "media_bucket_access" {
  bucket = google_storage_bucket.media_bucket.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}

# Grant default Compute Engine service account access to Vertex AI
# This allows Cloud Run to use Vertex AI for Gemini models
resource "google_project_iam_member" "vertex_ai_user" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}

# Grant default Compute Engine service account permission to sign blobs
# This allows the service account to generate signed URLs for GCS objects
resource "google_service_account_iam_member" "signblob_permission" {
  service_account_id = data.google_compute_default_service_account.default.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${data.google_compute_default_service_account.default.email}"
}