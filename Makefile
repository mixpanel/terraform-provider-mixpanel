.PHONY: help test test-unit test-acceptance test-acc-cohort test-acc-dashboard test-acc-all build install clean docs

help:
	@echo "Mixpanel Terraform Provider - Available targets:"
	@echo ""
	@echo "  make test              - Run all tests (unit + mock)"
	@echo "  make test-unit         - Run unit tests only"
	@echo "  make test-acceptance   - Run acceptance tests (requires TF_ACC=1 and credentials)"
	@echo "  make test-acc-cohort   - Run cohort acceptance tests"
	@echo "  make test-acc-dashboard - Run dashboard acceptance tests"
	@echo "  make test-acc-all      - Run all GREEN-10 acceptance tests"
	@echo "  make build             - Build the provider binary"
	@echo "  make install           - Install provider locally"
	@echo "  make clean             - Clean build artifacts"
	@echo ""
	@echo "Acceptance testing requires:"
	@echo "  export TF_ACC=1"
	@echo "  export MIXPANEL_SERVICE_ACCOUNT=your-sa-name"
	@echo "  export MIXPANEL_SERVICE_ACCOUNT_SECRET=your-secret"
	@echo "  export MIXPANEL_PROJECT_ID=your-project-id"
	@echo ""
	@echo "See internal/provider/ACCEPTANCE_TESTING.md for full documentation"

test:
	go test -v ./...

test-unit:
	go test -v ./... -short

test-acceptance:
	@if [ -z "$(TF_ACC)" ]; then \
		echo "Error: TF_ACC must be set to 1"; \
		echo "Run: make test-acceptance TF_ACC=1"; \
		exit 1; \
	fi
	@if [ -z "$(MIXPANEL_SERVICE_ACCOUNT)" ]; then \
		echo "Error: MIXPANEL_SERVICE_ACCOUNT must be set"; \
		exit 1; \
	fi
	@if [ -z "$(MIXPANEL_SERVICE_ACCOUNT_SECRET)" ]; then \
		echo "Error: MIXPANEL_SERVICE_ACCOUNT_SECRET must be set"; \
		exit 1; \
	fi
	@if [ -z "$(MIXPANEL_PROJECT_ID)" ] && [ -z "$(MIXPANEL_TEST_PROJECT_POOL)" ]; then \
		echo "Error: Either MIXPANEL_PROJECT_ID or MIXPANEL_TEST_PROJECT_POOL must be set"; \
		exit 1; \
	fi
	TF_ACC=1 go test -v ./internal/provider -run TestAcc -timeout 30m

test-acc-cohort:
	TF_ACC=1 go test -v ./internal/provider -run TestAccCohort -timeout 10m

test-acc-dashboard:
	TF_ACC=1 go test -v ./internal/provider -run TestAccDashboard -timeout 10m

test-acc-bookmark:
	TF_ACC=1 go test -v ./internal/provider -run TestAccBookmark -timeout 10m

test-acc-metric:
	TF_ACC=1 go test -v ./internal/provider -run TestAccMetric -timeout 10m

test-acc-formula:
	TF_ACC=1 go test -v ./internal/provider -run TestAccFormula -timeout 10m

test-acc-feature-flag:
	TF_ACC=1 go test -v ./internal/provider -run TestAccFeatureFlag -timeout 10m

test-acc-behavior:
	TF_ACC=1 go test -v ./internal/provider -run TestAccBehavior -timeout 10m

test-acc-custom-event:
	TF_ACC=1 go test -v ./internal/provider -run TestAccCustomEvent -timeout 10m

test-acc-custom-property:
	TF_ACC=1 go test -v ./internal/provider -run TestAccCustomProperty -timeout 10m

test-acc-project:
	TF_ACC=1 go test -v ./internal/provider -run TestAccProject -timeout 10m

# Run all GREEN-10 acceptance tests
test-acc-all: test-acc-cohort test-acc-dashboard test-acc-bookmark test-acc-metric test-acc-formula test-acc-feature-flag test-acc-behavior test-acc-custom-event test-acc-custom-property test-acc-project

# Run acceptance tests in parallel (requires project pool)
test-acc-parallel:
	@if [ -z "$(MIXPANEL_TEST_PROJECT_POOL)" ]; then \
		echo "Error: MIXPANEL_TEST_PROJECT_POOL must be set for parallel tests"; \
		exit 1; \
	fi
	TF_ACC=1 go test -v -parallel 4 ./internal/provider -run TestAcc -timeout 45m

build:
	go build -o terraform-provider-mixpanel .

install: build
	mkdir -p ~/.terraform.d/plugins/mixpanel/mixpanel/0.1.0/linux_amd64/
	cp terraform-provider-mixpanel ~/.terraform.d/plugins/mixpanel/mixpanel/0.1.0/linux_amd64/

clean:
	rm -f terraform-provider-mixpanel
	go clean -testcache

# Development helpers
fmt:
	go fmt ./...

lint:
	golangci-lint run

# Docs under docs/ are HAND-CURATED (see gen/README.md step 6 and CONTRIBUTING.md).
# Do not wire tfplugindocs here: only templates/index.md.tmpl exists, so
# regeneration would overwrite every hand-written page.
docs:
	@echo "docs/ is hand-curated. Edit the Markdown directly; do not run tfplugindocs."
	@echo "See CONTRIBUTING.md and gen/README.md (step 6)."

.DEFAULT_GOAL := help
