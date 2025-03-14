#####
#
# Example usage:
# 	$ make deploy-dev-backend
#
#####

# Grab branch + commit to tag the version.
# GCP only allows letters, numbers, and hyphens (e.g. kaito/feature => kaito-feature)
CURRENT_BRANCH       := $(shell git rev-parse --abbrev-ref HEAD | sed 's/[^a-zA-Z0-9]/-/g')
CURRENT_COMMIT_HASH  := $(shell git rev-parse --short=10 HEAD)
DEPLOY               := deploy
PROMOTE              := promote
STOP                 := stop
RELEASE              := release
DOCKER               := docker
CRON                 := cron
JOB                  := job
DEPLOY_VERSION       := $(CURRENT_BRANCH)-$(CURRENT_COMMIT_HASH)
SET_GCP_PROJECT      = gcloud config set project $(GCP_PROJECT)
CLOUD_RUN_DEPLOY     = sops exec-file $(CONFIG_DIR)/$(SERVICE_FILE) 'gcloud run deploy $(DEPLOY_FLAGS) $(SERVICE) --env-vars-file {} --quiet'
CLOUD_JOB_DEPLOY     = sops exec-file $(CONFIG_DIR)/$(SERVICE_FILE) 'gcloud run jobs update $(JOB_NAME) --image $(IMAGE_TAG) --set-cloudsql-instances $(SQL_INSTANCES) --region $(REGION) $(JOB_OPTIONS) --env-vars-file {} --quiet'
SCHEDULER_DEPLOY     = gcloud scheduler jobs create http $(CRON_NAME) --location $(CRON_LOCATION) --schedule $(CRON_SCHEDULE) --uri $(CRON_URI) --http-method $(CRON_METHOD)
CRON_NAME            = $(CRON_PREFIX)-$(DEPLOY_VERSION)
BASE_DEPLOY_FLAGS    = --image $(IMAGE_TAG) $(RUN_PROMOTE_FLAGS) --concurrency $(CONCURRENCY) --cpu $(CPU) --memory $(MEMORY) --port $(PORT) --timeout $(TIMEOUT) --labels service=$(SERVICE) --platform managed --revision-suffix $(CURRENT_COMMIT_HASH) --vpc-connector $(VPC_CONNECTOR) --vpc-egress private-ranges-only --set-cloudsql-instances $(SQL_INSTANCES) --region $(REGION) --allow-unauthenticated
DEPLOY_FLAGS         = $(BASE_DEPLOY_FLAGS) --cpu-throttling
DEPLOY_REGION        = us-east1
SENTRY_RELEASE       = sentry-cli releases -o $(SENTRY_ORG) -p $(SENTRY_PROJECT)
IMAGE_TAG            = $(DOCKER_REGISTRY)/$(GCP_PROJECT)/$(REPO)/$(CURRENT_BRANCH):$(CURRENT_COMMIT_HASH)
DOCKER_BUILD         = docker build --file $(DOCKER_FILE) --platform linux/amd64 -t $(IMAGE_TAG) --build-arg VERSION=$(DEPLOY_VERSION) --build-arg FFMPEG_VERSION=$(FFMPEG_VERSION) $(DOCKER_CONTEXT)
DOCKER_PUSH          = docker push $(IMAGE_TAG)
DOCKER_DIR           := ./docker
DOCKER_CONTEXT       := .
DOCKER_REGISTRY      := us-east1-docker.pkg.dev
FFMPEG_VERSION       = 7:4.3.8-0+deb11u3

# Environments
DEV     := dev
PROD    := prod
SANDBOX := sandbox

# sops secrets
# These should be used to set REQUIRED_SOPS_SECRETS, which can take one or more
# space-separated secrets files
SOPS_SECRETS_FILENAME := makevars.yaml
SOPS_SECRETS_DIR      := secrets
SOPS_DEV_SECRETS      := $(SOPS_SECRETS_DIR)/$(DEV)/$(SOPS_SECRETS_FILENAME)
SOPS_PROD_SECRETS     := $(SOPS_SECRETS_DIR)/$(PROD)/$(SOPS_SECRETS_FILENAME)

# Per-target secrets
start-dev-sql-proxy    : REQUIRED_SOPS_SECRETS := $(SOPS_DEV_SECRETS)
start-prod-sql-proxy   : REQUIRED_SOPS_SECRETS := $(SOPS_PROD_SECRETS)
migrate-dev-coredb     : REQUIRED_SOPS_SECRETS := $(SOPS_DEV_SECRETS)
migrate-prod-coredb    : REQUIRED_SOPS_SECRETS := $(SOPS_PROD_SECRETS)

# Environment-specific settings
$(DEPLOY)-$(DEV)-%                : ENV                    := $(DEV)
$(DEPLOY)-$(SANDBOX)-%            : ENV                    := $(SANDBOX)
$(DEPLOY)-$(PROD)-%               : ENV                    := $(PROD)
$(DEPLOY)-$(DEV)-%                : REQUIRED_SOPS_SECRETS  := $(SOPS_DEV_SECRETS)
$(DEPLOY)-$(SANDBOX)-%            : REQUIRED_SOPS_SECRETS  := $(SOPS_DEV_SECRETS)
$(DEPLOY)-$(PROD)-%               : REQUIRED_SOPS_SECRETS  := $(SOPS_PROD_SECRETS)
$(DEPLOY)-$(DEV)-%                : RUN_PROMOTE_FLAGS      :=
$(DEPLOY)-$(PROD)-%               : RUN_PROMOTE_FLAGS      := --no-traffic
$(DEPLOY)-$(DEV)-%                : CONFIG_DIR             := ./$(SOPS_SECRETS_DIR)/$(DEV)
$(DEPLOY)-$(SANDBOX)-%            : CONFIG_DIR             := ./$(SOPS_SECRETS_DIR)/$(DEV)
$(DEPLOY)-$(PROD)-%               : CONFIG_DIR             := ./$(SOPS_SECRETS_DIR)/$(PROD)
$(PROMOTE)-$(PROD)-%              : ENV                    := $(PROD)
$(PROMOTE)-$(PROD)-%              : REQUIRED_SOPS_SECRETS  := $(SOPS_PROD_SECRETS)

# Service files, add a line for each service and environment you want to deploy.
$(DEPLOY)-$(DEV)-backend            : SERVICE_FILE := backend-env.yaml
$(DEPLOY)-$(DEV)-admin              : SERVICE_FILE := app-dev-admin.yaml
$(DEPLOY)-$(DEV)-feed               : SERVICE_FILE := feed-env.yaml
$(DEPLOY)-$(DEV)-tokenprocessing    : SERVICE_FILE := tokenprocessing-env.yaml
$(DEPLOY)-$(DEV)-autosocial         : SERVICE_FILE := autosocial-env.yaml
$(DEPLOY)-$(DEV)-activitystats      : SERVICE_FILE := activitystats-env.yaml
$(DEPLOY)-$(DEV)-autosocial-orch    : SERVICE_FILE := autosocial-env.yaml
$(DEPLOY)-$(DEV)-pushnotifications  : SERVICE_FILE := pushnotifications-env.yaml
$(DEPLOY)-$(DEV)-emails             : SERVICE_FILE := emails-server-env.yaml
$(DEPLOY)-$(DEV)-opensea-streamer   : SERVICE_FILE := opensea-streamer-env.yaml
$(DEPLOY)-$(DEV)-feedbot            : SERVICE_FILE := feedbot-env.yaml
$(DEPLOY)-$(DEV)-routing-rules      : SERVICE_FILE := dispatch.yaml
$(DEPLOY)-$(DEV)-graphql-gateway    : SERVICE_FILE := graphql-gateway.yml
$(DEPLOY)-$(DEV)-userpref-upload    : SERVICE_FILE := userpref-upload.yaml
$(DEPLOY)-$(SANDBOX)-backend        : SERVICE_FILE := backend-sandbox-env.yaml
$(DEPLOY)-$(PROD)-backend           : SERVICE_FILE := backend-env.yaml
$(DEPLOY)-$(PROD)-admin             : SERVICE_FILE := app-prod-admin.yaml
$(DEPLOY)-$(PROD)-feed              : SERVICE_FILE := feed-env.yaml
$(DEPLOY)-$(PROD)-feedbot           : SERVICE_FILE := feedbot-env.yaml
$(DEPLOY)-$(PROD)-autosocial        : SERVICE_FILE := autosocial-env.yaml
$(DEPLOY)-$(PROD)-activitystats     : SERVICE_FILE := activitystats-env.yaml
$(DEPLOY)-$(PROD)-autosocial-orch   : SERVICE_FILE := autosocial-env.yaml
$(DEPLOY)-$(PROD)-tokenprocessing   : SERVICE_FILE := tokenprocessing-env.yaml
$(DEPLOY)-$(PROD)-pushnotifications : SERVICE_FILE := pushnotifications-env.yaml
$(DEPLOY)-$(PROD)-opensea-streamer  : SERVICE_FILE := opensea-streamer-env.yaml
$(DEPLOY)-$(PROD)-dummymetadata     : SERVICE_FILE := dummymetadata-env.yaml
$(DEPLOY)-$(PROD)-emails            : SERVICE_FILE := emails-server-env.yaml
$(DEPLOY)-$(PROD)-routing-rules     : SERVICE_FILE := dispatch.yaml
$(DEPLOY)-$(PROD)-graphql-gateway   : SERVICE_FILE := graphql-gateway.yml
$(DEPLOY)-$(PROD)-userpref-upload   : SERVICE_FILE := userpref-upload.yaml
$(DEPLOY)-$(PROD)-rasterizer        : SERVICE_FILE := rasterizer.yaml

# Service to Sentry project mapping
$(DEPLOY)-%-backend               : SENTRY_PROJECT := gallery-backend
$(DEPLOY)-%-tokenprocessing       : SENTRY_PROJECT := tokenprocessing
$(DEPLOY)-%-pushnotifications     : SENTRY_PROJECT := pushnotifications
$(DEPLOY)-%-dummymetadata         : SENTRY_PROJECT := dummymetadata
$(DEPLOY)-%-feed                  : SENTRY_PROJECT := feed
$(DEPLOY)-%-feedbot               : SENTRY_PROJECT := feedbot
$(DEPLOY)-%-emails                : SENTRY_PROJECT := emails
$(DEPLOY)-%-userpref-upload       : SENTRY_PROJECT := userpref
$(DEPLOY)-%-activitystats         : SENTRY_PROJECT := activitystats
$(DEPLOY)-%-opensea-streamer      : SENTRY_PROJECT := opensea-streamer
$(DEPLOY)-%-autosocial            : SENTRY_PROJECT := autosocial
$(DEPLOY)-%-autosocial-orch       : SENTRY_PROJECT := autosocial

# Docker builds
$(DEPLOY)-%-tokenprocessing            : REPO           := tokenprocessing-v3
$(DEPLOY)-%-tokenprocessing            : DOCKER_FILE    := $(DOCKER_DIR)/tokenprocessing/Dockerfile
$(DEPLOY)-%-tokenprocessing            : PORT           := 6500
$(DEPLOY)-%-tokenprocessing            : TIMEOUT        := $(TOKENPROCESSING_TIMEOUT)
$(DEPLOY)-%-tokenprocessing            : CPU            := $(TOKENPROCESSING_CPU)
$(DEPLOY)-%-tokenprocessing            : MEMORY         := $(TOKENPROCESSING_MEMORY)
$(DEPLOY)-%-tokenprocessing            : CONCURRENCY    := $(TOKENPROCESSING_CONCURRENCY)
$(DEPLOY)-$(DEV)-tokenprocessing       : SERVICE        := tokenprocessing-v3
$(DEPLOY)-$(PROD)-tokenprocessing      : SERVICE        := tokenprocessing-v3
$(DEPLOY)-%-autosocial            	   : REPO           := autosocial
$(DEPLOY)-%-autosocial            	   : DOCKER_FILE    := $(DOCKER_DIR)/autosocial/Dockerfile
$(DEPLOY)-%-autosocial            	   : PORT           := 6700
$(DEPLOY)-%-autosocial            	   : TIMEOUT        := $(AUTOSOCIAL_TIMEOUT)
$(DEPLOY)-%-autosocial            	   : CPU            := $(AUTOSOCIAL_CPU)
$(DEPLOY)-%-autosocial            	   : MEMORY         := $(AUTOSOCIAL_MEMORY)
$(DEPLOY)-%-autosocial            	   : CONCURRENCY    := $(AUTOSOCIAL_CONCURRENCY)
$(DEPLOY)-$(DEV)-autosocial      	   : SERVICE        := autosocial
$(DEPLOY)-$(PROD)-autosocial      	   : SERVICE        := autosocial
$(DEPLOY)-%-activitystats              : REPO           := activitystats
$(DEPLOY)-%-activitystats              : DOCKER_FILE    := $(DOCKER_DIR)/activitystats/Dockerfile
$(DEPLOY)-%-activitystats              : PORT           := 6750
$(DEPLOY)-%-activitystats              : TIMEOUT        := $(ACTIVITYSTATS_TIMEOUT) 
$(DEPLOY)-%-activitystats              : CPU            := $(ACTIVITYSTATS_CPU)
$(DEPLOY)-%-activitystats              : MEMORY         := $(ACTIVITYSTATS_MEMORY)
$(DEPLOY)-%-activitystats              : CONCURRENCY    := $(ACTIVITYSTATS_CONCURRENCY)
$(DEPLOY)-$(DEV)-activitystats         : SERVICE        := activitystats
$(DEPLOY)-$(PROD)-activitystats        : SERVICE        := activitystats
$(DEPLOY)-%-autosocial-orch            : REPO           := autosocial-orchestrator
$(DEPLOY)-%-autosocial-orch            : DOCKER_FILE    := $(DOCKER_DIR)/autosocial/orchestrator/Dockerfile
$(DEPLOY)-%-autosocial-orch            : PORT           := 6800
$(DEPLOY)-%-autosocial-orch            : TIMEOUT        := $(AUTOSOCIAL_ORCH_TIMEOUT)
$(DEPLOY)-%-autosocial-orch            : CPU            := $(AUTOSOCIAL_ORCH_CPU)
$(DEPLOY)-%-autosocial-orch            : MEMORY         := $(AUTOSOCIAL_ORCH_MEMORY)
$(DEPLOY)-%-autosocial-orch            : CONCURRENCY    := $(AUTOSOCIAL_ORCH_CONCURRENCY)
$(DEPLOY)-$(DEV)-autosocial-orch       : SERVICE        := autosocial-orchestrator
$(DEPLOY)-$(PROD)-autosocial-orch      : SERVICE        := autosocial-orchestrator
$(DEPLOY)-%-pushnotifications          : REPO           := pushnotifications
$(DEPLOY)-%-pushnotifications          : REPO           := pushnotifications
$(DEPLOY)-%-pushnotifications          : DOCKER_FILE    := $(DOCKER_DIR)/pushnotifications/Dockerfile
$(DEPLOY)-%-pushnotifications          : PORT           := 6600
$(DEPLOY)-%-pushnotifications          : TIMEOUT        := $(PUSH_NOTIFICATIONS_TIMEOUT)
$(DEPLOY)-%-pushnotifications          : CPU            := $(PUSH_NOTIFICATIONS_CPU)
$(DEPLOY)-%-pushnotifications          : MEMORY         := $(PUSH_NOTIFICATIONS_MEMORY)
$(DEPLOY)-%-pushnotifications          : CONCURRENCY    := $(PUSH_NOTIFICATIONS_CONCURRENCY)
$(DEPLOY)-$(DEV)-pushnotifications     : SERVICE        := pushnotifications-dev
$(DEPLOY)-$(PROD)-pushnotifications    : SERVICE        := pushnotifications
$(DEPLOY)-$(PROD)-pushnotifications    : SQL_INSTANCES  := $(SQL_INSTANCES),$(SQL_CONNECTION_NAME_MOSHI_PROD_DB)
$(DEPLOY)-%-dummymetadata              : REPO           := dummymetadata
$(DEPLOY)-%-dummymetadata              : DOCKER_FILE    := $(DOCKER_DIR)/dummymetadata/Dockerfile
$(DEPLOY)-%-dummymetadata              : PORT           := 8500
$(DEPLOY)-%-dummymetadata              : TIMEOUT        := $(DUMMYMETADATA_TIMEOUT)
$(DEPLOY)-%-dummymetadata              : CPU            := $(DUMMYMETADATA_CPU)
$(DEPLOY)-%-dummymetadata              : MEMORY         := $(DUMMYMETADATA_MEMORY)
$(DEPLOY)-%-dummymetadata              : CONCURRENCY    := $(DUMMYMETADATA_CONCURRENCY)
$(DEPLOY)-%-dummymetadata              : SERVICE        := dummymetadata
$(DEPLOY)-%-emails                     : REPO           := emails
$(DEPLOY)-%-emails                     : DOCKER_FILE    := $(DOCKER_DIR)/emails/Dockerfile
$(DEPLOY)-%-emails                     : PORT           := 5500
$(DEPLOY)-%-emails                     : TIMEOUT        := $(EMAILS_TIMEOUT)
$(DEPLOY)-%-emails                     : CPU            := $(EMAILS_CPU)
$(DEPLOY)-%-emails                     : MEMORY         := $(EMAILS_MEMORY)
$(DEPLOY)-%-emails                     : CONCURRENCY    := $(EMAILS_CONCURRENCY)
$(DEPLOY)-$(DEV)-emails                : SERVICE        := emails-dev
$(DEPLOY)-$(PROD)-emails               : SERVICE        := emails-v2
$(DEPLOY)-%-backend                    : REPO           := backend
$(DEPLOY)-$(DEV)-backend               : DOCKER_FILE    := $(DOCKER_DIR)/backend/dev/Dockerfile
$(DEPLOY)-$(SANDBOX)-backend           : DOCKER_FILE    := $(DOCKER_DIR)/backend/dev/Dockerfile
$(DEPLOY)-$(PROD)-backend              : DOCKER_FILE    := $(DOCKER_DIR)/backend/prod/Dockerfile
$(DEPLOY)-%-backend                    : PORT           := 4000
$(DEPLOY)-%-backend                    : TIMEOUT        := $(BACKEND_TIMEOUT)
$(DEPLOY)-%-backend                    : CPU            := $(BACKEND_CPU)
$(DEPLOY)-%-backend                    : MEMORY         := $(BACKEND_MEMORY)
$(DEPLOY)-%-backend                    : CONCURRENCY    := $(BACKEND_CONCURRENCY)
$(DEPLOY)-$(DEV)-backend               : SERVICE        := backend-dev
$(DEPLOY)-$(SANDBOX)-backend           : SERVICE        := backend-sandbox
$(DEPLOY)-$(PROD)-backend              : SERVICE        := backend
$(DEPLOY)-%-feed                       : REPO           := feed
$(DEPLOY)-%-feed                       : DOCKER_FILE    := $(DOCKER_DIR)/feed/Dockerfile
$(DEPLOY)-%-feed                       : PORT           := 4100
$(DEPLOY)-%-feed                       : TIMEOUT        := $(FEED_TIMEOUT)
$(DEPLOY)-%-feed                       : CPU            := $(FEED_CPU)
$(DEPLOY)-%-feed                       : MEMORY         := $(FEED_MEMORY)
$(DEPLOY)-%-feed                       : CONCURRENCY    := $(FEED_CONCURRENCY)
$(DEPLOY)-$(DEV)-feed                  : SERVICE        := feed-dev
$(DEPLOY)-$(PROD)-feed                 : SERVICE        := feed
$(DEPLOY)-%-feedbot                    : REPO           := feedbot
$(DEPLOY)-%-feedbot                    : DOCKER_FILE    := $(DOCKER_DIR)/feedbot/Dockerfile
$(DEPLOY)-%-feedbot                    : PORT           := 4123
$(DEPLOY)-%-feedbot                    : TIMEOUT        := $(FEEDBOT_TIMEOUT)
$(DEPLOY)-%-feedbot                    : CPU            := $(FEEDBOT_CPU)
$(DEPLOY)-%-feedbot                    : MEMORY         := $(FEEDBOT_MEMORY)
$(DEPLOY)-%-feedbot                    : CONCURRENCY    := $(FEEDBOT_CONCURRENCY)
$(DEPLOY)-$(DEV)-feedbot               : SERVICE        := feedbot-dev
$(DEPLOY)-$(PROD)-feedbot              : SERVICE        := feedbot
$(DEPLOY)-%-graphql-gateway            : REPO           := graphql-gateway
$(DEPLOY)-$(DEV)-graphql-gateway       : DOCKER_FILE    := $(DOCKER_DIR)/graphql-gateway/$(DEV)/Dockerfile
$(DEPLOY)-$(PROD)-graphql-gateway      : DOCKER_FILE    := $(DOCKER_DIR)/graphql-gateway/$(PROD)/Dockerfile
$(DEPLOY)-%-graphql-gateway            : PORT           := 8000
$(DEPLOY)-%-graphql-gateway            : TIMEOUT        := $(GRAPHQL_GATEWAY_TIMEOUT)
$(DEPLOY)-%-graphql-gateway            : CPU            := $(GRAPHQL_GATEWAY_CPU)
$(DEPLOY)-%-graphql-gateway            : MEMORY         := $(GRAPHQL_GATEWAY_MEMORY)
$(DEPLOY)-%-graphql-gateway            : CONCURRENCY    := $(GRAPHQL_GATEWAY_CONCURRENCY)
$(DEPLOY)-$(DEV)-graphql-gateway       : SERVICE        := graphql-gateway-dev
$(DEPLOY)-$(PROD)-graphql-gateway      : SERVICE        := graphql-gateway
$(DEPLOY)-%-opensea-streamer           : REPO           := opensea-streamer
$(DEPLOY)-%-opensea-streamer           : DOCKER_FILE    := $(DOCKER_DIR)/opensea-streamer/Dockerfile
$(DEPLOY)-%-opensea-streamer           : PORT           := 3000
$(DEPLOY)-%-opensea-streamer           : TIMEOUT        := $(OPENSEA_STREAMER_TIMEOUT)
$(DEPLOY)-%-opensea-streamer           : CPU            := $(OPENSEA_STREAMER_CPU)
$(DEPLOY)-%-opensea-streamer           : MEMORY         := $(OPENSEA_STREAMER_MEMORY)
$(DEPLOY)-%-opensea-streamer           : CONCURRENCY    := $(OPENSEA_STREAMER_CONCURRENCY)
$(DEPLOY)-%-opensea-streamer           : DEPLOY_FLAGS   = $(BASE_DEPLOY_FLAGS) --no-cpu-throttling
$(DEPLOY)-$(DEV)-opensea-streamer      : SERVICE        := opensea-streamer
$(DEPLOY)-$(PROD)-opensea-streamer     : SERVICE        := opensea-streamer
$(DEPLOY)-%-rasterizer                 : SERVICE        := rasterizer
$(DEPLOY)-%-rasterizer                 : REPO           := rasterizer
$(DEPLOY)-%-rasterizer                 : DOCKER_FILE    := $(DOCKER_DIR)/rasterizer/Dockerfile
$(DEPLOY)-%-rasterizer                 : CPU            := $(RASTERIZER_CPU)
$(DEPLOY)-%-rasterizer                 : MEMORY         := $(RASTERIZER_MEMORY)
$(DEPLOY)-%-rasterizer                 : CONCURRENCY    := $(RASTERIZER_CONCURRENCY)
$(DEPLOY)-%-rasterizer                 : TIMEOUT        := $(RASTERIZER_TIMEOUT)
$(DEPLOY)-%-rasterizer                 : PORT           := 3000

# Cloud Scheduler Jobs
$(DEPLOY)-%-alchemy-spam                     : CRON_PREFIX    := alchemy-spam
$(DEPLOY)-%-alchemy-spam                     : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-alchemy-spam                     : CRON_SCHEDULE  := '0 0 * * *'
$(DEPLOY)-%-alchemy-spam                     : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/contracts/detect-spam
$(DEPLOY)-%-alchemy-spam                     : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-alchemy-spam                : URI_NAME       := tokenprocessing-dev
$(DEPLOY)-$(PROD)-alchemy-spam               : URI_NAME       := tokenprocessing-v3
$(DEPLOY)-%-check-push-tickets               : CRON_PREFIX    := check-push-tickets
$(DEPLOY)-%-check-push-tickets               : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-check-push-tickets               : CRON_SCHEDULE  := '*/5 * * * *'
$(DEPLOY)-%-check-push-tickets               : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/jobs/check-push-tickets
$(DEPLOY)-%-check-push-tickets               : CRON_METHOD    := POST
$(DEPLOY)-%-check-push-tickets               : CRON_FLAGS     = --headers='Authorization=Basic $(shell printf ":$(PUSH_NOTIFICATIONS_SECRET)" | base64)' --attempt-deadline=10m
$(DEPLOY)-$(DEV)-check-push-tickets          : URI_NAME       := pushnotifications-dev
$(DEPLOY)-$(PROD)-check-push-tickets         : URI_NAME       := pushnotifications
$(DEPLOY)-%-autosocial-process-users         : CRON_PREFIX    := autosocial-process-users
$(DEPLOY)-%-autosocial-process-users         : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-autosocial-process-users         : CRON_SCHEDULE  := '0 0 * * *'
$(DEPLOY)-%-autosocial-process-users         : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/process/users
$(DEPLOY)-%-autosocial-process-users         : CRON_FLAGS     = --oidc-service-account-email $(GCP_PROJECT_NUMBER)-compute@developer.gserviceaccount.com --oidc-token-audience $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')
$(DEPLOY)-%-autosocial-process-users         : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-autosocial-process-users    : URI_NAME       := autosocial-orchestrator
$(DEPLOY)-$(PROD)-autosocial-process-users   : URI_NAME       := autosocial-orchestrator
$(DEPLOY)-%-activity-stats-top               : CRON_PREFIX    := activitystats_top
$(DEPLOY)-%-activity-stats-top               : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-activity-stats-top               : CRON_SCHEDULE  := '0 14 * * 1'
$(DEPLOY)-%-activity-stats-top               : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/calculate_activity_badges
$(DEPLOY)-%-activity-stats-top               : CRON_FLAGS     = --oidc-service-account-email $(GCP_PROJECT_NUMBER)-compute@developer.gserviceaccount.com --oidc-token-audience $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')
$(DEPLOY)-%-activity-stats-top               : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-activity-stats-top          : URI_NAME       := activitystats 
$(DEPLOY)-$(PROD)-activity-stats-top         : URI_NAME       := activitystats
$(DEPLOY)-%-emails-notifications             : CRON_PREFIX    := emails_notifications
$(DEPLOY)-%-emails-notifications             : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-emails-notifications             : CRON_SCHEDULE  := '0 14 * * 5'
$(DEPLOY)-%-emails-notifications             : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/notifications/send
$(DEPLOY)-%-emails-notifications             : CRON_FLAGS     = --oidc-service-account-email $(GCP_PROJECT_NUMBER)-compute@developer.gserviceaccount.com --oidc-token-audience $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')
$(DEPLOY)-%-emails-notifications             : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-emails-notifications        : URI_NAME       := emails-dev
$(DEPLOY)-$(PROD)-emails-notifications       : URI_NAME       := emails-v2
$(DEPLOY)-%-emails-digest                    : CRON_PREFIX    := emails_digest
$(DEPLOY)-%-emails-digest                    : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-emails-digest                    : CRON_SCHEDULE  := '0 16 * * 1'
$(DEPLOY)-%-emails-digest                    : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/digest/send
$(DEPLOY)-%-emails-digest                    : CRON_FLAGS     = --oidc-service-account-email $(GCP_PROJECT_NUMBER)-compute@developer.gserviceaccount.com --oidc-token-audience $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')
$(DEPLOY)-%-emails-digest                    : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-emails-digest               : URI_NAME       := emails-dev
$(DEPLOY)-$(PROD)-emails-digest              : URI_NAME       := emails-v2

# Cloud Jobs
$(DEPLOY)-%-userpref-upload            : JOB_NAME       := userpref-upload
$(DEPLOY)-%-userpref-upload            : BASE_OPTIONS   := --tasks 1 --task-timeout 10m --parallelism 1 --cpu 1 --memory 4G
$(DEPLOY)-%-userpref-upload            : REGION         := $(DEPLOY_REGION)
$(DEPLOY)-%-userpref-upload            : REPO           := userpref
$(DEPLOY)-%-userpref-upload            : CRON_PREFIX    := userpref-schedule
$(DEPLOY)-%-userpref-upload            : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-userpref-upload            : CRON_SCHEDULE  := '0 * * * *'
$(DEPLOY)-%-userpref-upload            : CRON_URI       = https://$(DEPLOY_REGION)-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/$(GCP_PROJECT)/jobs/$(JOB_NAME):run
$(DEPLOY)-%-userpref-upload            : CRON_METHOD    := POST
$(DEPLOY)-%-userpref-upload            : CRON_FLAGS     = --oauth-service-account-email $(GCP_PROJECT_NUMBER)-compute@developer.gserviceaccount.com
$(DEPLOY)-$(DEV)-userpref-upload       : JOB_OPTIONS    = $(BASE_OPTIONS) --args dev-user-pref,personalization_matrices.bin.gz
$(DEPLOY)-$(PROD)-userpref-upload      : JOB_OPTIONS    = $(BASE_OPTIONS) --args prod-user-pref,personalization_matrices.bin.gz
$(DEPLOY)-%-userpref-upload            : DOCKER_FILE    := $(DOCKER_DIR)/userpref/Dockerfile

# Service name mappings
$(PROMOTE)-%-backend                   : SERVICE := default
$(PROMOTE)-%-emails                    : SERVICE := emails-v2
$(PROMOTE)-%-tokenprocessing           : SERVICE := tokenprocessing
$(PROMOTE)-%-tokenprocessing           : SERVICE := tokenprocessing-v3
$(PROMOTE)-%-autosocial                : SERVICE := autosocial
$(PROMOTE)-%-autosocial-orch           : SERVICE := autosocial-orchestrator
$(PROMOTE)-%-activitystats             : SERVICE := activitystats
$(PROMOTE)-%-pushnotifications         : SERVICE := pushnotifications
$(PROMOTE)-%-dummymetadata             : SERVICE := dummymetadata
$(PROMOTE)-%-feed                      : SERVICE := feed
$(PROMOTE)-%-feedbot                   : SERVICE := feedbot
$(PROMOTE)-%-admin                     : SERVICE := admin

#----------------------------------------------------------------
# SOPS handling
#----------------------------------------------------------------
# Uses recursive $(MAKE) calls to import sops files and add their
# variables to the environment
#----------------------------------------------------------------
export _SOPS_EXPORTED_REQUIRED_FILES
export _SOPS_REQUIRED_FILES
export _SOPS_REMAINING_FILES
export _SOPS_PROCESSING_STARTED
export _SOPS_PROCESSING_FINISHED

ifdef _SOPS_REQUIRED_FILES
	ifndef _SOPS_PROCESSING_FINISHED
		_PROCESS_SOPS_FILES := 1
		ifndef _SOPS_PROCESSING_STARTED
			_SOPS_REMAINING_FILES = $(REQUIRED_SOPS_SECRETS)
			_SOPS_PROCESSING_STARTED := 1
		endif
	else
		_SOPS_REQUIRED_FILES :=
        _SOPS_REMAINING_FILES :=
        _SOPS_PROCESSING_STARTED :=
        _SOPS_PROCESSING_FINISHED :=
	endif
endif

# First pass: conditional statements like "ifdef _SOPS_REQUIRED_FILES" above are evaluated
# before target-specific variables are defined. We want the ability to import sops files on
# a per-target basis, though, so the first thing every target does is export its $(REQUIRED_SOPS_SECRETS)
# variable and re-run make.
ifndef _SOPS_EXPORTED_REQUIRED_FILES
%:
	$(eval _SOPS_EXPORTED_REQUIRED_FILES := 1)
	$(eval _SOPS_REQUIRED_FILES := $(REQUIRED_SOPS_SECRETS))
	@$(MAKE) $@

# Second pass: with the target's REQUIRED_SOPS_SECRETS (if any) available to the conditional
# statement above, we can now process each required secrets file in order. Each secrets file
# will use 'sops exec-env $(MAKE)' to spawn a new sub-make with the contents of the secrets file
# added to that sub-make's environment.
else ifdef _PROCESS_SOPS_FILES

%:
	$(eval _CURRENT_SOPS_FILE := $(firstword $(_SOPS_REMAINING_FILES)))
	$(eval _SOPS_REMAINING_FILES := $(wordlist 2,$(words $(_SOPS_REMAINING_FILES)),$(_SOPS_REMAINING_FILES)))
	$(if $(_SOPS_REMAINING_FILES),,$(eval _SOPS_PROCESSING_FINISHED := 1))
	@sops exec-env $(_CURRENT_SOPS_FILE) '$(MAKE) $@'

# Final pass: all REQUIRED_SOPS_SECRETS (if any) have been processed and their variables are
# now present in the environment. The final sub-make will build the actual target recipe. Clear
# out _SOPS_EXPORTED_REQUIRED_FILES so we can $(MAKE) sub-targets and let them import their own
# sops variables if necessary.
else
%: _SOPS_EXPORTED_REQUIRED_FILES :=

#----------------------------------------------------------------
# Targets go below here
#----------------------------------------------------------------

# Sets the GCP context to the appropriate project
_set-project-$(ENV):
	@echo "\n========== DEPLOYING TO '$(ENV)' ENVIRONMENT ==========\n"
	$(SET_GCP_PROJECT)

_$(DOCKER)-$(DEPLOY)-%:
	@$(DOCKER_BUILD)
	@$(DOCKER_PUSH)
	@$(CLOUD_RUN_DEPLOY)
	@if [ $$? -eq 0 ] && echo $(DEPLOY_FLAGS) | grep -e "--no-traffic" > /dev/null; then echo "\n\tVERSION: '$(CURRENT_COMMIT_HASH)' was deployed but is not currently receiving traffic.\n\tRun 'make promote-$(ENV)-$* version=$(CURRENT_COMMIT_HASH)' to promote it!\n"; else echo "\n\tVERSION: '$(CURRENT_COMMIT_HASH)' was deployed!\n"; fi

_$(CRON)-$(DEPLOY)-%:
	@$(SCHEDULER_DEPLOY) $(CRON_FLAGS)
	@echo Deployed job $(CRON_NAME) to $(ENV)

# Pauses jobs besides the version being deployed
_$(CRON)-$(PAUSE)-%:
	@echo Pausing jobs besides $(CRON_NAME)...
	@gcloud scheduler jobs list \
		--location $(DEPLOY_REGION) \
		--filter 'ID:$(CRON_PREFIX) AND NOT ID:$(CRON_NAME) AND STATE:enabled' \
		--format 'value(ID)' \
		| xargs -I {} gcloud scheduler jobs pause --location $(DEPLOY_REGION) --quiet {}

_$(JOB)-$(DEPLOY)-%:
	@echo Deploying job $(JOB_NAME)
	@$(DOCKER_BUILD)
	@$(DOCKER_PUSH)
	@$(CLOUD_JOB_DEPLOY)

# Immediately migrates traffic to the input version
_$(PROMOTE)-%:
	@version='$(version)'; \
	if [ -z "$$version" ]; then echo "Please add 'version=...' to the command!" ; exit 1; fi; \
	gcloud beta app services set-traffic $(SERVICE) --splits $$version=1

_$(DOCKER)-$(PROMOTE)-%:
	@version='$(version)'; \
	if [ -z "$$version" ]; then echo "Please add 'version=...' to the command!" ; exit 1; fi; \
	gcloud run services update-traffic $(SERVICE) --to-revisions=$(SERVICE)-$$version=100

# Creates a new release in Sentry. Requires sentry-cli, install it by running:
#
#  $ curl -sL https://sentry.io/get-cli/ | bash
#
# You'll also need to authenticate your client by running:
#
#  $ sentry-cli login
#
_$(RELEASE)-%:
	@$(SENTRY_RELEASE) new $(DEPLOY_VERSION)
	@$(SENTRY_RELEASE) set-commits $(DEPLOY_VERSION) --auto --ignore-missing
	@$(SENTRY_RELEASE) finalize $(DEPLOY_VERSION)
	@$(SENTRY_RELEASE) deploys $(DEPLOY_VERSION) new -n $(DEPLOY_VERSION) -e $(ENV)

# DEV deployments
$(DEPLOY)-$(DEV)-backend            : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend
$(DEPLOY)-$(DEV)-tokenprocessing    : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-tokenprocessing _$(RELEASE)-tokenprocessing
$(DEPLOY)-$(DEV)-autosocial         : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-autosocial _$(RELEASE)-autosocial
$(DEPLOY)-$(DEV)-autosocial-orch    : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-autosocial-orch _$(RELEASE)-autosocial-orc
$(DEPLOY)-$(DEV)-activitystats      : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-activitystats _$(RELEASE)-activitystats
$(DEPLOY)-$(DEV)-pushnotifications  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-pushnotifications _$(RELEASE)-pushnotifications
$(DEPLOY)-$(DEV)-emails             : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-emails _$(RELEASE)-emails
$(DEPLOY)-$(DEV)-admin              : _set-project-$(ENV) _$(DEPLOY)-admin
$(DEPLOY)-$(DEV)-feed               : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feed _$(RELEASE)-feed
$(DEPLOY)-$(DEV)-feedbot            : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feedbot _$(RELEASE)-feedbot
$(DEPLOY)-$(DEV)-opensea-streamer   : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-opensea-streamer _$(RELEASE)-opensea-streamer
$(DEPLOY)-$(DEV)-routing-rules      : _set-project-$(ENV) _$(DEPLOY)-routing-rules
$(DEPLOY)-$(DEV)-graphql-gateway    : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-graphql-gateway
$(DEPLOY)-$(DEV)-alchemy-spam       : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-alchemy-spam _$(CRON)-$(PAUSE)-alchemy-spam
$(DEPLOY)-$(DEV)-check-push-tickets : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-check-push-tickets _$(CRON)-$(PAUSE)-check-push-tickets
$(DEPLOY)-$(DEV)-userpref-upload    : _set-project-$(ENV) _$(JOB)-$(DEPLOY)-userpref-upload _$(CRON)-$(DEPLOY)-userpref-upload _$(CRON)-$(PAUSE)-userpref-upload
$(DEPLOY)-$(DEV)-autosocial-process-users : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-autosocial-process-users _$(CRON)-$(PAUSE)-autosocial-process-users
$(DEPLOY)-$(DEV)-activity-stats-top : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-activity-stats-top _$(CRON)-$(PAUSE)-activity-stats-top
$(DEPLOY)-$(DEV)-emails-notifications : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-emails-notifications _$(CRON)-$(PAUSE)-emails-notifications
$(DEPLOY)-$(DEV)-emails-digest : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-emails-digest _$(CRON)-$(PAUSE)-emails-digest

# SANDBOX deployments
$(DEPLOY)-$(SANDBOX)-backend      : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend # go server that uses dev upstream services

# PROD deployments
$(DEPLOY)-$(PROD)-backend                  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend
$(DEPLOY)-$(PROD)-tokenprocessing          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-tokenprocessing _$(RELEASE)-tokenprocessing
$(DEPLOY)-$(PROD)-autosocial               : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-autosocial _$(RELEASE)-autosocial
$(DEPLOY)-$(PROD)-autosocial-orch          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-autosocial-orch _$(RELEASE)-autosocial-orch
$(DEPLOY)-$(PROD)-activitystats            : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-activitystats _$(RELEASE)-activitystats
$(DEPLOY)-$(PROD)-pushnotifications        : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-pushnotifications _$(RELEASE)-pushnotifications
$(DEPLOY)-$(PROD)-dummymetadata            : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-dummymetadata _$(RELEASE)-dummymetadata
$(DEPLOY)-$(PROD)-emails                   : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-emails _$(RELEASE)-emails
$(DEPLOY)-$(PROD)-feed                     : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feed _$(RELEASE)-feed
$(DEPLOY)-$(PROD)-feedbot                  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feedbot _$(RELEASE)-feedbot
$(DEPLOY)-$(PROD)-opensea-streamer         : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-opensea-streamer _$(RELEASE)-opensea-streamer
$(DEPLOY)-$(PROD)-admin                    : _set-project-$(ENV) _$(DEPLOY)-admin
$(DEPLOY)-$(PROD)-routing-rules            : _set-project-$(ENV) _$(DEPLOY)-routing-rules
$(DEPLOY)-$(PROD)-graphql-gateway          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-graphql-gateway
$(DEPLOY)-$(PROD)-alchemy-spam             : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-alchemy-spam _$(CRON)-$(PAUSE)-alchemy-spam
$(DEPLOY)-$(PROD)-check-push-tickets       : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-check-push-tickets _$(CRON)-$(PAUSE)-check-push-tickets
$(DEPLOY)-$(PROD)-userpref-upload          : _set-project-$(ENV) _$(JOB)-$(DEPLOY)-userpref-upload _$(CRON)-$(DEPLOY)-userpref-upload _$(CRON)-$(PAUSE)-userpref-upload
$(DEPLOY)-$(PROD)-autosocial-process-users : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-autosocial-process-users _$(CRON)-$(PAUSE)-autosocial-process-users
$(DEPLOY)-$(PROD)-activity-stats-top       : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-activity-stats-top _$(CRON)-$(PAUSE)-activity-stats-top
$(DEPLOY)-$(PROD)-emails-notifications     : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-emails-notifications _$(CRON)-$(PAUSE)-emails-notifications
$(DEPLOY)-$(PROD)-emails-digest            : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-emails-digest _$(CRON)-$(PAUSE)-emails-digest
$(DEPLOY)-$(PROD)-rasterizer               : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-rasterizer


# PROD promotions. Running these targets will migrate traffic to the specified version.
# Example usage:
#
# $ make promote-prod-backend version=myVersion
#
$(PROMOTE)-$(PROD)-backend            : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-backend
$(PROMOTE)-$(PROD)-tokenprocessing    : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-tokenprocessing
$(PROMOTE)-$(PROD)-autosocial         : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-autosocial
$(PROMOTE)-$(PROD)-autosocial-orch    : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-autosocial-orch
$(PROMOTE)-$(PROD)-activitystats      : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-activitystats
$(PROMOTE)-$(PROD)-pushnotifications  : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-pushnotifications
$(PROMOTE)-$(PROD)-dummymetadata      : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-dummymetadata
$(PROMOTE)-$(PROD)-emails             : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-emails
$(PROMOTE)-$(PROD)-feed               : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-feed
$(PROMOTE)-$(PROD)-opensea-streamer   : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-opensea-streamer
$(PROMOTE)-$(PROD)-feedbot            : _set-project-$(ENV) _$(PROMOTE)-feedbot
$(PROMOTE)-$(PROD)-admin              : _set-project-$(ENV) _$(PROMOTE)-admin

# Contracts
contracts: solc abi-gen

solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.abi
	solc --abi ./contracts/sol/IERC20.sol > ./contracts/abi/IERC20.abi
	solc --abi ./contracts/sol/IERC721Metadata.sol > ./contracts/abi/IERC721Metadata.abi
	solc --abi ./contracts/sol/IERC1155.sol > ./contracts/abi/IERC1155.abi
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.abi
	solc --abi ./contracts/sol/IERC1155Metadata_URI.sol > ./contracts/abi/IERC1155Metadata_URI.abi
	solc --abi ./contracts/sol/ISignatureValidator.sol > ./contracts/abi/ISignatureValidator.abi
	solc --abi ./contracts/sol/CryptopunksData.sol > ./contracts/abi/CryptopunksData.abi
	solc --abi ./contracts/sol/Cryptopunks.sol > ./contracts/abi/Cryptopunks.abi
	solc --abi ./contracts/sol/Zora.sol > ./contracts/abi/Zora.abi
	solc --abi ./contracts/sol/Merch.sol > ./contracts/abi/Merch.abi
	solc --abi ./contracts/sol/PremiumCards.sol > ./contracts/abi/PremiumCards.abi
	solc --abi ./contracts/sol/Ownable.sol > ./contracts/abi/Ownable.abi
	tail -n +4 "./contracts/abi/IERC721.abi" > "./contracts/abi/IERC721.abi.tmp" && mv "./contracts/abi/IERC721.abi.tmp" "./contracts/abi/IERC721.abi"
	tail -n +4 "./contracts/abi/IERC20.abi" > "./contracts/abi/IERC20.abi.tmp" && mv "./contracts/abi/IERC20.abi.tmp" "./contracts/abi/IERC20.abi"
	tail -n +4 "./contracts/abi/IERC721Metadata.abi" > "./contracts/abi/IERC721Metadata.abi.tmp" && mv "./contracts/abi/IERC721Metadata.abi.tmp" "./contracts/abi/IERC721Metadata.abi"
	tail -n +4 "./contracts/abi/IERC1155.abi" > "./contracts/abi/IERC1155.abi.tmp" && mv "./contracts/abi/IERC1155.abi.tmp" "./contracts/abi/IERC1155.abi"
	tail -n +4 "./contracts/abi/IENS.abi" > "./contracts/abi/IENS.abi.tmp" && mv "./contracts/abi/IENS.abi.tmp" "./contracts/abi/IENS.abi"
	tail -n +4 "./contracts/abi/IERC1155Metadata_URI.abi" > "./contracts/abi/IERC1155Metadata_URI.abi.tmp" && mv "./contracts/abi/IERC1155Metadata_URI.abi.tmp" "./contracts/abi/IERC1155Metadata_URI.abi"
	tail -n +4 "./contracts/abi/ISignatureValidator.abi" > "./contracts/abi/ISignatureValidator.abi.tmp" && mv "./contracts/abi/ISignatureValidator.abi.tmp" "./contracts/abi/ISignatureValidator.abi"
	tail -n +4 "./contracts/abi/CryptopunksData.abi" > "./contracts/abi/CryptopunksData.abi.tmp" && mv "./contracts/abi/CryptopunksData.abi.tmp" "./contracts/abi/CryptopunksData.abi"
	tail -n +4 "./contracts/abi/Cryptopunks.abi" > "./contracts/abi/Cryptopunks.abi.tmp" && mv "./contracts/abi/Cryptopunks.abi.tmp" "./contracts/abi/Cryptopunks.abi"
	tail -n +4 "./contracts/abi/Zora.abi" > "./contracts/abi/Zora.abi.tmp" && mv "./contracts/abi/Zora.abi.tmp" "./contracts/abi/Zora.abi"
	tail -n +4 "./contracts/abi/Merch.abi" > "./contracts/abi/Merch.abi.tmp" && mv "./contracts/abi/Merch.abi.tmp" "./contracts/abi/Merch.abi"
	tail -n +4 "./contracts/abi/PremiumCards.abi" > "./contracts/abi/PremiumCards.abi.tmp" && mv "./contracts/abi/PremiumCards.abi.tmp" "./contracts/abi/PremiumCards.abi"
	tail -n +4 "./contracts/abi/Ownable.abi" > "./contracts/abi/Ownable.abi.tmp" && mv "./contracts/abi/Ownable.abi.tmp" "./contracts/abi/Ownable.abi"

abi-gen:
	abigen --abi=./contracts/abi/IERC721.abi --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IERC20.abi --pkg=contracts --type=IERC20 > ./contracts/IERC20.go
	abigen --abi=./contracts/abi/IERC721Metadata.abi --pkg=contracts --type=IERC721Metadata > ./contracts/IERC721Metadata.go
	abigen --abi=./contracts/abi/IERC1155.abi --pkg=contracts --type=IERC1155 > ./contracts/IERC1155.go
	abigen --abi=./contracts/abi/IENS.abi --pkg=contracts --type=IENS > ./contracts/IENS.go
	abigen --abi=./contracts/abi/IERC1155Metadata_URI.abi --pkg=contracts --type=IERC1155Metadata_URI > ./contracts/IERC1155Metadata_URI.go
	abigen --abi=./contracts/abi/ISignatureValidator.abi --pkg=contracts --type=ISignatureValidator > ./contracts/ISignatureValidator.go
	abigen --abi=./contracts/abi/CryptopunksData.abi --pkg=contracts --type=CryptopunksData > ./contracts/CryptopunksData.go
	abigen --abi=./contracts/abi/Cryptopunks.abi --pkg=contracts --type=Cryptopunks > ./contracts/Cryptopunks.go
	abigen --abi=./contracts/abi/Zora.abi --pkg=contracts --type=Zora > ./contracts/Zora.go
	abigen --abi=./contracts/abi/Merch.abi --pkg=contracts --type=Merch > ./contracts/Merch.go
	abigen --abi=./contracts/abi/PremiumCards.abi --pkg=contracts --type=PremiumCards > ./contracts/PremiumCards.go
	abigen --abi=./contracts/abi/Ownable.abi --pkg=contracts --type=Ownable > ./contracts/Ownable.go

# Miscellaneous stuff
docker-start-clean:	docker-build
	docker compose up -d

docker-build: docker-stop
	docker compose build

docker-start: docker-stop
	docker compose up -d

docker-stop:
	docker compose down

format-graphql:
	yarn install;
	yarn prettier --write graphql/schema/schema.graphql
	yarn prettier --write graphql/testdata/operations.graphql;

start-local-graphql-gateway:
	docker compose -f docker/graphql-gateway/docker-compose.yml up --build -d graphql-gateway-local

start-dev-graphql-gateway:
	docker compose -f docker/graphql-gateway/docker-compose.yml up --build -d graphql-gateway-dev

start-prod-graphql-gateway:
	docker compose -f docker/graphql-gateway/docker-compose.yml up --build -d graphql-gateway-prod

# Listing targets as dependencies doesn't pull in target-specific secrets, so we need to
# invoke $(MAKE) here to read appropriate secrets for each target.
start-sql-proxy:
	$(MAKE) start-dev-sql-proxy
	$(MAKE) start-prod-sql-proxy

start-dev-sql-proxy:
	docker compose -f docker/cloud_sql_proxy/docker-compose.yml up -d cloud-sql-proxy-dev

start-prod-sql-proxy:
	docker compose -f docker/cloud_sql_proxy/docker-compose.yml up -d cloud-sql-proxy-prod

stop-sql-proxy:
	docker compose -f docker/cloud_sql_proxy/docker-compose.yml down

migrate-local-coredb:
	go run cmd/migrate/main.go

confirm-dev-migrate:
	@prompt=$(shell bash -c 'read -p "Are you sure you want to apply migrations to the dev DB? Type \"development\" to confirm: " prompt; echo $$prompt'); \
	if [ "$$prompt" != "development" ]; then exit 1; fi

migrate-dev-coredb: start-dev-sql-proxy confirm-dev-migrate
	@POSTGRES_USER=$(POSTGRES_MIGRATION_USER) \
	POSTGRES_PASSWORD=$(POSTGRES_MIGRATION_PASSWORD) \
	POSTGRES_PORT=6643 \
	go run cmd/migrate/main.go

confirm-prod-migrate:
	@prompt=$(shell bash -c 'read -p "Are you sure you want to apply migrations to the production DB? Type \"production\" to confirm: " prompt; echo $$prompt'); \
	if [ "$$prompt" != "production" ]; then exit 1; fi

migrate-prod-coredb: start-prod-sql-proxy confirm-prod-migrate
	@POSTGRES_USER=$(POSTGRES_MIGRATION_USER) \
	POSTGRES_PASSWORD=$(POSTGRES_MIGRATION_PASSWORD) \
	POSTGRES_PORT=6543 \
	go run cmd/migrate/main.go

fix-sops-macs:
	@cd secrets; ../scripts/fix-sops-macs.sh

sqlc-generate:
	sqlc generate
	go run cmd/dataloaders/main.go

#----------------------------------------------------------------
# End of targets
#----------------------------------------------------------------

endif
