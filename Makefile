BIN ?= $(HOME)/bin/tf

build:
	go build -o tf .

install:
	go build -o $(BIN) .
	@echo "installed $(BIN)"

demo.gif: demo.tape
	cd demo && terraform init -input=false >/dev/null && \
	  terraform destroy -auto-approve -input=false >/dev/null
	vhs demo.tape

.PHONY: build install
