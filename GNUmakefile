default: testacc

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 TF_LOG_PROVIDER_BOSK=DEBUG go test ./... -v $(TESTARGS) -timeout 120m
