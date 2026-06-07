##
## ack-event-publisher e2e test harness
##
## Deployment is split into three layers:
##
##   Layer 1 (test-infra):       CloudFormation — VPC, EKS Auto Mode cluster,
##                               Pod Identity Agent add-on, ACK IAM role.
##                               No Kubernetes prereqs required.
##
##   Layer 2 (test-setup):       Helm — ACK S3 controller + ack-event-publisher.
##                               Requires cluster from Layer 1.
##
##   Layer 3 (test-run):         kubectl — test S3 Bucket resource + event
##                               verification. Requires Layer 2.
##
## Prerequisites:
##   aws CLI (configured, sufficient permissions)
##   helm >= 3.8
##   kubectl
##   envsubst (part of gettext)
##

STACK_NAME       ?= ack-event-publisher-test
AWS_REGION       ?= us-east-1
CFN_TEMPLATE     := cfn/cluster.yaml

.PHONY: test-infra test-infra-wait test-infra-destroy test-setup test-run test-clean

## Layer 1 ─────────────────────────────────────────────────────────────────

# Deploy the CloudFormation stack (non-blocking; use test-infra-wait to block).
test-infra:
	aws cloudformation deploy \
		--stack-name $(STACK_NAME) \
		--template-file $(CFN_TEMPLATE) \
		--parameter-overrides ClusterName=$(STACK_NAME) \
		--capabilities CAPABILITY_NAMED_IAM \
		--region $(AWS_REGION)
	@echo ""
	@echo "Stack deployment initiated. Run 'make test-infra-wait' to block until ACTIVE."

# Block until the stack reaches CREATE_COMPLETE / UPDATE_COMPLETE.
test-infra-wait:
	aws cloudformation wait stack-create-complete \
		--stack-name $(STACK_NAME) \
		--region $(AWS_REGION) || \
	aws cloudformation wait stack-update-complete \
		--stack-name $(STACK_NAME) \
		--region $(AWS_REGION)
	@echo "Stack is ready."

# Destroy the CloudFormation stack and all AWS resources it created.
test-infra-destroy:
	@echo "WARNING: This will delete the EKS cluster, VPC, and IAM roles."
	@read -p "Continue? [y/N] " confirm && [ "$${confirm}" = "y" ]
	aws cloudformation delete-stack \
		--stack-name $(STACK_NAME) \
		--region $(AWS_REGION)
	aws cloudformation wait stack-delete-complete \
		--stack-name $(STACK_NAME) \
		--region $(AWS_REGION)
	@echo "Stack deleted."

## Layer 2 ─────────────────────────────────────────────────────────────────

# Install Helm charts onto the cluster.
test-setup:
	STACK_NAME=$(STACK_NAME) AWS_REGION=$(AWS_REGION) \
		bash scripts/setup.sh

## Layer 3 ─────────────────────────────────────────────────────────────────

# Deploy test ACK resources and verify events are published.
test-run:
	AWS_REGION=$(AWS_REGION) bash scripts/verify.sh

# Remove test Kubernetes resources and Helm releases (keeps cluster).
test-clean:
	STACK_NAME=$(STACK_NAME) AWS_REGION=$(AWS_REGION) \
		bash scripts/teardown.sh

## Convenience targets ──────────────────────────────────────────────────────

# Full end-to-end: infra + wait + setup + run. Useful for CI pipelines.
test-all: test-infra test-infra-wait test-setup test-run

help:
	@grep -E '^[a-zA-Z_-]+:' Makefile | \
		awk -F: '{printf "  %-25s\n", $$1}' | sort
