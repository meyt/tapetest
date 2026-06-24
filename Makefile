DEMO_DIR := demoapp

.PHONY: install
install:
	go mod tidy


.PHONY: demo
demo:
	cd $(DEMO_DIR) && go run .


.PHONY: test
test:
	cd $(DEMO_DIR) && go test


.PHONY: dev
dev: test demo
