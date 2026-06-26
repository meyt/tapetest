DEMO_DIR := demoapp

.PHONY: install
install:
	go mod tidy


test:
	go test ./tests


.PHONY: demo
demo:
	cd $(DEMO_DIR) && go run .


.PHONY: test-demo
test-demo:
	cd $(DEMO_DIR) && go test


.PHONY: dev
dev: test-demo demo
