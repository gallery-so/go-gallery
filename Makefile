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
DEPLOY_VERSION       := $(CURRENT_BRANCH)-$(CURRENT_COMMIT_HASH)
SET_GCP_PROJECT      = gcloud config set project $(GCP_PROJECT)
CLOUD_RUN_DEPLOY     = sops exec-file $(CONFIG_DIR)/$(SERVICE_FILE) 'gcloud run deploy $(DEPLOY_FLAGS) $(SERVICE) --env-vars-file {} --quiet'
SCHEDULER_DEPLOY     = gcloud scheduler jobs create http $(CRON_NAME) --location $(CRON_LOCATION) --schedule $(CRON_SCHEDULE) --uri $(CRON_URI) --http-method $(CRON_METHOD)
CRON_NAME            = $(CRON_PREFIX)-$(DEPLOY_VERSION)
BASE_DEPLOY_FLAGS    = --image $(IMAGE_TAG) $(RUN_PROMOTE_FLAGS) --concurrency $(CONCURRENCY) --cpu $(CPU) --memory $(MEMORY) --port $(PORT) --timeout $(TIMEOUT) --platform managed --revision-suffix $(CURRENT_COMMIT_HASH) --vpc-connector $(VPC_CONNECTOR) --vpc-egress private-ranges-only --set-cloudsql-instances $(SQL_INSTANCES) --region $(REGION) --allow-unauthenticated
DEPLOY_FLAGS         = $(BASE_DEPLOY_FLAGS) --cpu-throttling
DEPLOY_REGION        = us-east1
SENTRY_RELEASE       = sentry-cli releases -o $(SENTRY_ORG) -p $(SENTRY_PROJECT)
IMAGE_TAG            = $(DOCKER_REGISTRY)/$(GCP_PROJECT)/$(REPO)/$(CURRENT_BRANCH):$(CURRENT_COMMIT_HASH)
DOCKER_BUILD         = docker build --file $(DOCKER_FILE) --platform linux/amd64 -t $(IMAGE_TAG) --build-arg VERSION=$(DEPLOY_VERSION) $(DOCKER_CONTEXT)
DOCKER_PUSH          = docker push $(IMAGE_TAG)
DOCKER_DIR           := ./docker
DOCKER_CONTEXT       := .
DOCKER_REGISTRY      := us-east1-docker.pkg.dev

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
start-dev-sql-proxy  : REQUIRED_SOPS_SECRETS := $(SOPS_DEV_SECRETS)
start-prod-sql-proxy : REQUIRED_SOPS_SECRETS := $(SOPS_PROD_SECRETS)
migrate-dev-coredb   : REQUIRED_SOPS_SECRETS := $(SOPS_DEV_SECRETS)
migrate-prod-coredb  : REQUIRED_SOPS_SECRETS := $(SOPS_PROD_SECRETS)
migrate-prod-indexerdb : REQUIRED_SOPS_SECRETS := $(SOPS_PROD_SECRETS)

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
$(DEPLOY)-$(DEV)-backend          : SERVICE_FILE := backend-env.yaml
$(DEPLOY)-$(DEV)-indexer-server   : SERVICE_FILE := indexer-server-env.yaml
$(DEPLOY)-$(DEV)-admin            : SERVICE_FILE := app-dev-admin.yaml
$(DEPLOY)-$(DEV)-feed             : SERVICE_FILE := feed-env.yaml
$(DEPLOY)-$(DEV)-tokenprocessing  : SERVICE_FILE := tokenprocessing-env.yaml
$(DEPLOY)-$(DEV)-emails           : SERVICE_FILE := emails-server-env.yaml
$(DEPLOY)-$(DEV)-feedbot          : SERVICE_FILE := feedbot-env.yaml
$(DEPLOY)-$(DEV)-routing-rules    : SERVICE_FILE := dispatch.yaml
$(DEPLOY)-$(SANDBOX)-backend      : SERVICE_FILE := backend-sandbox-env.yaml
$(DEPLOY)-$(PROD)-backend         : SERVICE_FILE := backend-env.yaml
$(DEPLOY)-$(PROD)-indexer         : SERVICE_FILE := indexer-env.yaml
$(DEPLOY)-$(PROD)-indexer-server  : SERVICE_FILE := indexer-server-env.yaml
$(DEPLOY)-$(PROD)-admin           : SERVICE_FILE := app-prod-admin.yaml
$(DEPLOY)-$(PROD)-feed            : SERVICE_FILE := feed-env.yaml
$(DEPLOY)-$(PROD)-feedbot         : SERVICE_FILE := feedbot-env.yaml
$(DEPLOY)-$(PROD)-tokenprocessing : SERVICE_FILE := tokenprocessing-env.yaml
$(DEPLOY)-$(PROD)-dummymetadata   : SERVICE_FILE := dummymetadata-env.yaml
$(DEPLOY)-$(PROD)-emails          : SERVICE_FILE := emails-server-env.yaml
$(DEPLOY)-$(PROD)-routing-rules   : SERVICE_FILE := dispatch.yaml
$(DEPLOY)-$(DEV)-graphql-gateway  : SERVICE_FILE := graphql-gateway.yml
$(DEPLOY)-$(PROD)-graphql-gateway : SERVICE_FILE := graphql-gateway.yml

# Service to Sentry project mapping
$(DEPLOY)-%-backend               : SENTRY_PROJECT := gallery-backend
$(DEPLOY)-%-indexer               : SENTRY_PROJECT := indexer
$(DEPLOY)-%-indexer-server        : SENTRY_PROJECT := indexer-api
$(DEPLOY)-%-tokenprocessing       : SENTRY_PROJECT := tokenprocessing
$(DEPLOY)-%-dummymetadata         : SENTRY_PROJECT := dummymetadata
$(DEPLOY)-%-feed                  : SENTRY_PROJECT := feed
$(DEPLOY)-%-feedbot               : SENTRY_PROJECT := feedbot
$(DEPLOY)-%-emails                : SENTRY_PROJECT := emails

# Docker builds
$(DEPLOY)-%-tokenprocessing            : REPO           := tokenprocessing
$(DEPLOY)-%-tokenprocessing            : DOCKER_FILE    := $(DOCKER_DIR)/tokenprocessing/Dockerfile
$(DEPLOY)-%-tokenprocessing            : PORT           := 6500
$(DEPLOY)-%-tokenprocessing            : TIMEOUT        := $(TOKENPROCESSING_TIMEOUT)
$(DEPLOY)-%-tokenprocessing            : CPU            := $(TOKENPROCESSING_CPU)
$(DEPLOY)-%-tokenprocessing            : MEMORY         := $(TOKENPROCESSING_MEMORY)
$(DEPLOY)-%-tokenprocessing            : CONCURRENCY    := $(TOKENPROCESSING_CONCURRENCY)
$(DEPLOY)-$(DEV)-tokenprocessing       : SERVICE        := tokenprocessing-dev
$(DEPLOY)-$(PROD)-tokenprocessing      : SERVICE        := tokenprocessing-v2
$(DEPLOY)-%-dummymetadata              : REPO           := dummymetadata
$(DEPLOY)-%-dummymetadata              : DOCKER_FILE    := $(DOCKER_DIR)/dummymetadata/Dockerfile
$(DEPLOY)-%-dummymetadata              : PORT           := 8500
$(DEPLOY)-%-dummymetadata              : TIMEOUT        := $(DUMMYMETADATA_TIMEOUT)
$(DEPLOY)-%-dummymetadata              : CPU            := $(DUMMYMETADATA_CPU)
$(DEPLOY)-%-dummymetadata              : MEMORY         := $(DUMMYMETADATA_MEMORY)
$(DEPLOY)-%-dummymetadata              : CONCURRENCY    := $(DUMMYMETADATA_CONCURRENCY)
$(DEPLOY)-%-dummymetadata              : SERVICE        := dummymetadata
$(DEPLOY)-%-indexer-server             : REPO           := indexer-api
$(DEPLOY)-%-indexer-server             : DOCKER_FILE    := $(DOCKER_DIR)/indexer_api/Dockerfile
$(DEPLOY)-%-indexer-server             : PORT           := 6000
$(DEPLOY)-%-indexer-server             : TIMEOUT        := $(INDEXER_SERVER_TIMEOUT)
$(DEPLOY)-%-indexer-server             : CPU            := $(INDEXER_SERVER_CPU)
$(DEPLOY)-%-indexer-server             : MEMORY         := $(INDEXER_SERVER_MEMORY)
$(DEPLOY)-%-indexer-server             : CONCURRENCY    := $(INDEXER_SERVER_CONCURRENCY)
$(DEPLOY)-$(DEV)-indexer-server        : SERVICE        := indexer-api-dev
$(DEPLOY)-$(PROD)-indexer-server       : SERVICE        := indexer-api
$(DEPLOY)-%-indexer                    : REPO           := indexer
$(DEPLOY)-%-indexer                    : DOCKER_FILE    := $(DOCKER_DIR)/indexer/Dockerfile
$(DEPLOY)-%-indexer                    : PORT           := 4000
$(DEPLOY)-%-indexer                    : TIMEOUT        := $(INDEXER_TIMEOUT)
$(DEPLOY)-%-indexer                    : CPU            := $(INDEXER_CPU)
$(DEPLOY)-%-indexer                    : MEMORY         := $(INDEXER_MEMORY)
$(DEPLOY)-%-indexer                    : CONCURRENCY    := $(INDEXER_CONCURRENCY)
$(DEPLOY)-%-indexer                    : DEPLOY_FLAGS   = $(BASE_DEPLOY_FLAGS) --no-cpu-throttling
$(DEPLOY)-$(PROD)-indexer              : SERVICE        := indexer
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

# Cloud Scheduler Jobs
$(DEPLOY)-%-alchemy-spam               : CRON_PREFIX    := alchemy-spam
$(DEPLOY)-%-alchemy-spam               : CRON_LOCATION  := $(DEPLOY_REGION)
$(DEPLOY)-%-alchemy-spam               : CRON_SCHEDULE  := '0 0 * * *'
$(DEPLOY)-%-alchemy-spam               : CRON_URI       = $(shell gcloud run services describe $(URI_NAME) --region $(DEPLOY_REGION) --format 'value(status.url)')/contracts/detect-spam
$(DEPLOY)-%-alchemy-spam               : CRON_METHOD    := POST
$(DEPLOY)-$(DEV)-alchemy-spam          : URI_NAME       := tokenprocessing-dev
$(DEPLOY)-$(PROD)-alchemy-spam         : URI_NAME       := tokenprocessing-v2

# Service name mappings
$(PROMOTE)-%-backend                   : SERVICE := default
$(PROMOTE)-%-indexer                   : SERVICE := indexer
$(PROMOTE)-%-indexer-server            : SERVICE := indexer-api
$(PROMOTE)-%-emails                    : SERVICE := emails
$(PROMOTE)-%-tokenprocessing           : SERVICE := tokenprocessing
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
	@$(SCHEDULER_DEPLOY)
	@echo Deployed job $(CRON_NAME) to $(ENV)

# Pauses jobs besides the version being deployed
_$(CRON)-$(PAUSE)-%:
	@echo Pausing jobs besides $(CRON_NAME)...
	@gcloud scheduler jobs list \
		--location $(DEPLOY_REGION) \
		--filter 'ID:$(CRON_PREFIX) AND NOT ID:$(CRON_NAME) AND STATE:enabled' \
		--format 'value(ID)' \
		| xargs -I {} gcloud scheduler jobs pause --location $(DEPLOY_REGION) --quiet {}

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
$(DEPLOY)-$(DEV)-backend          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend
$(DEPLOY)-$(DEV)-indexer-server   : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-indexer-server _$(RELEASE)-indexer-server
$(DEPLOY)-$(DEV)-tokenprocessing  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-tokenprocessing _$(RELEASE)-tokenprocessing
$(DEPLOY)-$(DEV)-emails           : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-emails _$(RELEASE)-emails
$(DEPLOY)-$(DEV)-admin            : _set-project-$(ENV) _$(DEPLOY)-admin
$(DEPLOY)-$(DEV)-feed             : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feed _$(RELEASE)-feed
$(DEPLOY)-$(DEV)-feedbot          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feedbot _$(RELEASE)-feedbot
$(DEPLOY)-$(DEV)-routing-rules    : _set-project-$(ENV) _$(DEPLOY)-routing-rules
$(DEPLOY)-$(DEV)-graphql-gateway  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-graphql-gateway
$(DEPLOY)-$(DEV)-alchemy-spam     : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-alchemy-spam _$(CRON)-$(PAUSE)-alchemy-spam

# SANDBOX deployments
$(DEPLOY)-$(SANDBOX)-backend      : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend # go server that uses dev upstream services

# PROD deployments
$(DEPLOY)-$(PROD)-backend         : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-backend _$(RELEASE)-backend
$(DEPLOY)-$(PROD)-indexer         : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-indexer _$(RELEASE)-indexer
$(DEPLOY)-$(PROD)-indexer-server  : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-indexer-server _$(RELEASE)-indexer-server
$(DEPLOY)-$(PROD)-tokenprocessing : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-tokenprocessing _$(RELEASE)-tokenprocessing
$(DEPLOY)-$(PROD)-dummymetadata   : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-dummymetadata _$(RELEASE)-dummymetadata
$(DEPLOY)-$(PROD)-emails          : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-emails _$(RELEASE)-emails
$(DEPLOY)-$(PROD)-feed            : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feed _$(RELEASE)-feed
$(DEPLOY)-$(PROD)-feedbot         : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-feedbot _$(RELEASE)-feedbot
$(DEPLOY)-$(PROD)-admin           : _set-project-$(ENV) _$(DEPLOY)-admin
$(DEPLOY)-$(PROD)-routing-rules   : _set-project-$(ENV) _$(DEPLOY)-routing-rules
$(DEPLOY)-$(PROD)-graphql-gateway : _set-project-$(ENV) _$(DOCKER)-$(DEPLOY)-graphql-gateway
$(DEPLOY)-$(PROD)-alchemy-spam    : _set-project-$(ENV) _$(CRON)-$(DEPLOY)-alchemy-spam _$(CRON)-$(PAUSE)-alchemy-spam

# PROD promotions. Running these targets will migrate traffic to the specified version.
# Example usage:
#
# $ make promote-prod-backend version=myVersion
#
$(PROMOTE)-$(PROD)-backend          : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-backend
$(PROMOTE)-$(PROD)-indexer          : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-indexer
$(PROMOTE)-$(PROD)-indexer-server   : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-indexer-server
$(PROMOTE)-$(PROD)-tokenprocessing  : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-tokenprocessing
$(PROMOTE)-$(PROD)-dummymetadata    : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-dummymetadata
$(PROMOTE)-$(PROD)-emails           : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-emails
$(PROMOTE)-$(PROD)-feed             : _set-project-$(ENV) _$(DOCKER)-$(PROMOTE)-feed
$(PROMOTE)-$(PROD)-feedbot          : _set-project-$(ENV) _$(PROMOTE)-feedbot
$(PROMOTE)-$(PROD)-admin            : _set-project-$(ENV) _$(PROMOTE)-admin

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
	docker-compose up -d

docker-build: docker-stop
	docker-compose build

docker-start: docker-stop
	docker-compose up -d

docker-stop:
	docker-compose down

format-graphql:
	yarn install;
	yarn prettier --write graphql/schema/schema.graphql
	yarn prettier --write graphql/testdata/operations.graphql;

start-local-graphql-gateway:
	docker-compose -f docker/graphql-gateway/docker-compose.yml up -d graphql-gateway-local

start-dev-graphql-gateway:
	docker-compose -f docker/graphql-gateway/docker-compose.yml up -d graphql-gateway-dev

start-prod-graphql-gateway:
	docker-compose -f docker/graphql-gateway/docker-compose.yml up -d graphql-gateway-prod

# Listing targets as dependencies doesn't pull in target-specific secrets, so we need to
# invoke $(MAKE) here to read appropriate secrets for each target.
start-sql-proxy:
	$(MAKE) start-dev-sql-proxy
	$(MAKE) start-prod-sql-proxy

start-dev-sql-proxy:
	docker-compose -f docker/cloud_sql_proxy/docker-compose.yml up -d cloud-sql-proxy-dev

start-prod-sql-proxy:
	docker-compose -f docker/cloud_sql_proxy/docker-compose.yml up -d cloud-sql-proxy-prod

stop-sql-proxy:
	docker-compose -f docker/cloud_sql_proxy/docker-compose.yml down

# Migrations
migrate-local-indexerdb:
	migrate -path ./db/migrations/indexer -database "postgresql://postgres@localhost:5433/postgres?sslmode=disable" up

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

migrate-prod-indexerdb: start-prod-sql-proxy confirm-prod-migrate
	migrate -path ./db/migrations/indexer -database "postgresql://postgres:$(POSTGRES_INDEXER_PASSWORD)@localhost:6545/postgres?sslmode=disable" up

fix-sops-macs:
	@cd secrets; ../scripts/fix-sops-macs.sh

#----------------------------------------------------------------
# End of targets
#----------------------------------------------------------------

endif
